//go:build integration

package integration

// Executable black-box acceptance against the public Manager URL. It requires
// a disposable project selected by the Docker harness and never logs secrets.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"supabase-manager/internal/contracts"
)

func TestConfigurationReconcile(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("SUPABASE_MANAGER_E2E_URL"), "/")
	if baseURL == "" {
		t.Fatal("SUPABASE_MANAGER_E2E_URL is required")
	}
	username, password, projectID := os.Getenv("SUPABASE_MANAGER_E2E_USERNAME"), os.Getenv("SUPABASE_MANAGER_E2E_PASSWORD"), os.Getenv("SUPABASE_MANAGER_E2E_PROJECT_ID")
	client := &http.Client{Timeout: 30 * time.Second}
	csrf, cookie := "", ""
	if username != "" && password != "" {
		ensureSetup(t, client, baseURL, username, password)
		csrf, cookie = loginAcceptance(t, client, baseURL, username, password)
	} else {
		cookie = os.Getenv("SUPABASE_MANAGER_E2E_COOKIE")
		csrf = os.Getenv("SUPABASE_MANAGER_E2E_CSRF")
		if cookie == "" || csrf == "" {
			t.Fatal("set credentials or SUPABASE_MANAGER_E2E_COOKIE and SUPABASE_MANAGER_E2E_CSRF")
		}
	}
	if projectID == "" {
		if username == "" || password == "" {
			t.Fatal("SUPABASE_MANAGER_E2E_PROJECT_ID or administrator credentials are required")
		}
		created := createCustomSMTPFunctionsProject(t, client, baseURL, cookie, csrf)
		projectID = created["projectId"].(string)
		if runtimeProject, ok := created["_runtimeProject"].(string); ok {
			os.Setenv("SUPABASE_MANAGER_E2E_RUNTIME_PROJECT", runtimeProject)
			if marker := os.Getenv("SUPABASE_MANAGER_E2E_RUNTIME_PROJECT_FILE"); marker != "" {
				if err := os.WriteFile(marker, []byte(runtimeProject), 0o600); err != nil {
					t.Fatalf("record runtime Compose project: %v", err)
				}
			}
		}
		defer cleanupRuntimeProject(t, client, baseURL, projectID, cookie, csrf)
		waitOperation(t, client, baseURL, created["operationId"].(string), cookie, csrf)
	}
	project := getJSON(t, client, baseURL+"/api/projects/"+projectID, cookie, csrf)
	if projectSlug, _ := project["slug"].(string); projectSlug == "" {
		t.Fatal("project response omitted slug")
	} else {
		runtimeProject := "supabase-manager-" + projectSlug
		os.Setenv("SUPABASE_MANAGER_E2E_RUNTIME_PROJECT", runtimeProject)
		if marker := os.Getenv("SUPABASE_MANAGER_E2E_RUNTIME_PROJECT_FILE"); marker != "" {
			if err := os.WriteFile(marker, []byte(runtimeProject), 0o600); err != nil {
				t.Fatalf("record runtime Compose project: %v", err)
			}
		}
	}
	config := getJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration", cookie, csrf)
	revision := int64(config["revision"].(float64))
	composeProject := os.Getenv("SUPABASE_MANAGER_E2E_RUNTIME_PROJECT")
	if composeProject == "" {
		composeProject = os.Getenv("SUPABASE_MANAGER_E2E_COMPOSE_PROJECT")
	}
	before := inspectContainerIDs(t, composeProject)
	google := map[string]any{"enabled": true, "clientId": "acceptance-client", "secretSet": true, "secret": map[string]any{"action": "replace", "value": acceptanceValue()}, "fields": map[string]any{}}
	op := patchJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration/oauth/google", map[string]any{"expectedRevision": revision, "value": google}, cookie, csrf)
	waitOperation(t, client, baseURL, op["operationId"].(string), cookie, csrf)
	afterOAuth := inspectContainerIDs(t, composeProject)
	assertServiceChange(t, before, afterOAuth, "auth", true)
	assertServiceChange(t, before, afterOAuth, "functions", false)
	assertOnlyServicesChanged(t, before, afterOAuth, "auth")
	config = getJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration", cookie, csrf)
	canonical, ok := config["configuration"].(map[string]any)
	if !ok {
		t.Fatal("configuration response omitted canonical configuration")
	}
	functions, ok := canonical["functions"].(map[string]any)
	if !ok {
		t.Fatal("configuration response omitted functions section")
	}
	functions["variables"] = []any{map[string]any{"name": "TASK10_ACCEPTANCE_SECRET", "valueSet": true, "value": map[string]any{"action": "replace", "value": acceptanceValue()}}}
	op = patchJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration/functions", map[string]any{"expectedRevision": int64(config["revision"].(float64)), "value": functions}, cookie, csrf)
	waitOperation(t, client, baseURL, op["operationId"].(string), cookie, csrf)
	afterFunctions := inspectContainerIDs(t, composeProject)
	assertServiceChange(t, afterOAuth, afterFunctions, "auth", false)
	assertServiceChange(t, afterOAuth, afterFunctions, "functions", true)
	assertOnlyServicesChanged(t, afterOAuth, afterFunctions, "functions")
	supabaseURL := strings.TrimRight(os.Getenv("SUPABASE_MANAGER_E2E_SUPABASE_URL"), "/")
	if supabaseURL == "" {
		network, networkOK := canonical["network"].(map[string]any)
		apiPort, portOK := network["apiPort"].(float64)
		if !networkOK || !portOK || apiPort < 1 {
			t.Fatal("SUPABASE_MANAGER_E2E_SUPABASE_URL or canonical network.apiPort is required")
		}
		supabaseURL = fmt.Sprintf("http://127.0.0.1:%d", int(apiPort))
	}
	request, err := http.NewRequest(http.MethodGet, supabaseURL+"/auth/v1/settings", nil)
	if err != nil {
		t.Fatal(err)
	}
	anonKey := os.Getenv("SUPABASE_MANAGER_E2E_ANON_KEY")
	if anonKey == "" && password != "" {
		anonKeyResponse := requestJSON(t, client, http.MethodPost, baseURL+"/api/projects/"+projectID+"/secrets/anonKey/reveal", map[string]any{"password": password}, cookie, csrf)
		anonKey, _ = anonKeyResponse["value"].(string)
	}
	if anonKey == "" {
		t.Fatal("SUPABASE_MANAGER_E2E_ANON_KEY is required for authenticated settings acceptance")
	}
	request.Header.Set("apikey", anonKey)
	request.Header.Set("Authorization", "Bearer "+anonKey)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET /auth/v1/settings: %v", err)
	}
	var settings map[string]any
	if err := json.NewDecoder(response.Body).Decode(&settings); err != nil {
		response.Body.Close()
		t.Fatalf("decode /auth/v1/settings: %v", err)
	}
	response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("GET /auth/v1/settings status=%d", response.StatusCode)
	}
	external, _ := settings["external"].(map[string]any)
	googleEnabled, _ := external["google"].(bool)
	if !googleEnabled {
		t.Fatalf("GET /auth/v1/settings reports Google disabled: %v", settings["external"])
	}
}

func cleanupRuntimeProject(t *testing.T, client *http.Client, baseURL, projectID, cookie, csrf string) {
	t.Helper()
	result, err := requestJSONNoFatal(client, http.MethodDelete, baseURL+"/api/projects/"+projectID, map[string]any{"mode": "runtime"}, cookie, csrf)
	if err != nil {
		t.Logf("cleanup runtime request failed: %v", err)
		return
	}
	if operationID, ok := result["operationId"].(string); ok {
		waitOperation(t, client, baseURL, operationID, cookie, csrf)
	}
}

func ensureSetup(t *testing.T, client *http.Client, baseURL, username, password string) {
	t.Helper()
	status := requestJSON(t, client, http.MethodGet, baseURL+"/api/setup/status", nil, "", "")
	required, _ := status["required"].(bool)
	if !required {
		return
	}
	requestJSON(t, client, http.MethodPost, baseURL+"/api/setup", map[string]any{"username": username, "password": password}, "", "")
}

func createCustomSMTPFunctionsProject(t *testing.T, client *http.Client, baseURL, cookie, csrf string) map[string]any {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	cfg := contracts.ProjectConfiguration{
		General:   contracts.GeneralConfig{SupabaseVersion: "self-hosted/v0.8.0"},
		Services:  contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true, Functions: true},
		Auth:      contracts.AuthConfig{Enabled: true, Email: contracts.EmailAuthConfig{Enabled: true, AllowSignup: true}, SMTP: contracts.SMTPConfig{Port: 587}},
		Storage:   contracts.StorageConfig{Backend: contracts.StorageBackendLocal},
		Realtime:  contracts.RealtimeConfig{MaxConnections: 100, DatabasePoolSize: 5, LogLevel: contracts.LogLevelInfo},
		Functions: contracts.FunctionsConfig{DefaultJWTVerification: true, Directory: "./functions"},
		Database:  contracts.DatabaseConfig{Version: "15", MaxConnections: 100},
		Pooler:    contracts.PoolerConfig{PoolSize: 20, MaxClientConnections: 100},
		Network:   contracts.NetworkConfig{Gateway: contracts.GatewayEnvoy, HTTPSMode: contracts.HTTPSModeExternal},
	}
	cfg.General.Domain = "task10-" + suffix + ".example.test"
	cfg.General.SiteURL = "https://" + cfg.General.Domain
	cfg.Services.Functions = true
	cfg.Auth.SMTP.Enabled = true
	cfg.Auth.SMTP.Host = "smtp.example.test"
	cfg.Auth.SMTP.Username = "acceptance"
	cfg.Auth.SMTP.Password = contracts.SecretInput{Action: "replace", Value: acceptanceValue()}
	cfg.Auth.SMTP.SenderEmail = "acceptance@example.test"
	cfg.Auth.SMTP.SenderName = "Task10 Acceptance"
	draft := contracts.ProjectDraft{Name: "Task10 Acceptance " + suffix, Slug: "task10-" + suffix, Preset: contracts.PresetCustom, Configuration: cfg}
	raw, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	result := requestJSON(t, client, http.MethodPost, baseURL+"/api/projects", payload, cookie, csrf)
	result["_runtimeProject"] = "supabase-manager-" + cfg.General.Domain[:strings.Index(cfg.General.Domain, ".")]
	return result
}

func inspectContainerIDs(t *testing.T, composeProject string) map[string]string {
	t.Helper()
	ids := make(map[string]string)
	if composeProject == "" {
		return ids
	}
	for _, service := range []string{"db", "api-gw", "auth", "rest", "studio", "meta", "functions"} {
		command := exec.Command("docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+composeProject, "--filter", "label=com.docker.compose.service="+service)
		output, err := command.Output()
		if err != nil {
			t.Fatalf("inspect %s container: %v", service, err)
		}
		ids[service] = strings.TrimSpace(string(output))
		if ids[service] == "" {
			t.Fatalf("acceptance container %s has no ID in Compose project %s", service, composeProject)
		}
		t.Logf("acceptance container %s id=%s", service, ids[service])
	}
	return ids
}

func assertServiceChange(t *testing.T, before, after map[string]string, service string, changed bool) {
	t.Helper()
	if before[service] == "" || after[service] == "" {
		t.Fatalf("%s container ID missing (before=%q after=%q)", service, before[service], after[service])
	}
	if (before[service] != after[service]) != changed {
		t.Fatalf("%s container changed=%v, want %v (before=%q after=%q)", service, before[service] != after[service], changed, before[service], after[service])
	}
}

func assertOnlyServicesChanged(t *testing.T, before, after map[string]string, changed ...string) {
	t.Helper()
	allowed := make(map[string]bool, len(changed))
	for _, service := range changed {
		allowed[service] = true
	}
	for name, previous := range before {
		if !allowed[name] && previous != after[name] {
			t.Fatalf("unexpected %s container change (before=%q after=%q)", name, previous, after[name])
		}
	}
}

func acceptanceValue() string { return fmt.Sprintf("task10-acceptance-%d", time.Now().UnixNano()) }

func loginAcceptance(t *testing.T, client *http.Client, baseURL, username, password string) (string, string) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/api/session", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", baseURL)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("login status=%d", response.StatusCode)
	}
	var payload struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for _, c := range response.Cookies() {
		if c.Name == "supabase_manager_session" {
			return payload.CSRFToken, c.String()
		}
	}
	t.Fatal("login did not return session cookie")
	return "", ""
}

func getJSON(t *testing.T, client *http.Client, url, cookie, csrf string) map[string]any {
	return requestJSON(t, client, http.MethodGet, url, nil, cookie, csrf)
}
func patchJSON(t *testing.T, client *http.Client, url string, payload map[string]any, cookie, csrf string) map[string]any {
	return requestJSON(t, client, http.MethodPatch, url, payload, cookie, csrf)
}
func requestJSON(t *testing.T, client *http.Client, method, rawURL string, payload map[string]any, cookie, csrf string) map[string]any {
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	}
	request, _ := http.NewRequest(method, rawURL, body)
	if parsed, parseErr := urlpkg(rawURL); parseErr == nil {
		request.Header.Set("Origin", parsed)
	}
	request.Header.Set("Cookie", cookie)
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, rawURL, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("%s %s status=%d", method, rawURL, response.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func requestJSONNoFatal(client *http.Client, method, rawURL string, payload map[string]any, cookie, csrf string) (map[string]any, error) {
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, err
	}
	if parsed, parseErr := urlpkg(rawURL); parseErr == nil {
		request.Header.Set("Origin", parsed)
	}
	request.Header.Set("Cookie", cookie)
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var result map[string]any
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s status=%d", method, rawURL, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func urlpkg(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid URL")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func waitOperation(t *testing.T, client *http.Client, baseURL, operationID, cookie, csrf string) {
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		result := getJSON(t, client, baseURL+"/api/operations/"+operationID, cookie, csrf)
		switch result["status"] {
		case "SUCCEEDED":
			return
		case "FAILED", "ROLLED_BACK", "CANCELLED":
			t.Fatalf("operation ended %v", result["status"])
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatal("operation did not complete within acceptance timeout")
}
