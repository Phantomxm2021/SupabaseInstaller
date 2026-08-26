//go:build integration

package integration

// Executable black-box acceptance against the public Manager URL. It requires
// a disposable project selected by the Docker harness and never logs secrets.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestConfigurationReconcile(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("SUPABASE_MANAGER_E2E_URL"), "/")
	if baseURL == "" {
		t.Fatal("SUPABASE_MANAGER_E2E_URL is required")
	}
	username, password, projectID := os.Getenv("SUPABASE_MANAGER_E2E_USERNAME"), os.Getenv("SUPABASE_MANAGER_E2E_PASSWORD"), os.Getenv("SUPABASE_MANAGER_E2E_PROJECT_ID")
	if username == "" || password == "" || projectID == "" {
		t.Fatal("SUPABASE_MANAGER_E2E_USERNAME, SUPABASE_MANAGER_E2E_PASSWORD, and SUPABASE_MANAGER_E2E_PROJECT_ID are required")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	csrf, cookie := loginAcceptance(t, client, baseURL, username, password)
	config := getJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration", cookie, csrf)
	revision := int64(config["revision"].(float64))
	google := map[string]any{"enabled": true, "clientId": "acceptance-client", "secretSet": true, "secret": map[string]any{"action": "replace", "value": acceptanceValue()}, "fields": map[string]any{}}
	op := patchJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration/oauth/google", map[string]any{"expectedRevision": revision, "value": google}, cookie, csrf)
	waitOperation(t, client, baseURL, op["operationId"].(string), cookie, csrf)
	config = getJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration", cookie, csrf)
	functions := config["functions"].(map[string]any)
	functions["variables"] = []any{map[string]any{"name": "TASK10_ACCEPTANCE_SECRET", "valueSet": true, "value": map[string]any{"action": "replace", "value": acceptanceValue()}}}
	op = patchJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration/functions", map[string]any{"expectedRevision": int64(config["revision"].(float64)), "value": functions}, cookie, csrf)
	waitOperation(t, client, baseURL, op["operationId"].(string), cookie, csrf)
	supabaseURL := strings.TrimRight(os.Getenv("SUPABASE_MANAGER_E2E_SUPABASE_URL"), "/")
	if supabaseURL == "" {
		t.Fatal("SUPABASE_MANAGER_E2E_SUPABASE_URL is required")
	}
	response, err := client.Get(supabaseURL + "/auth/v1/settings")
	if err != nil {
		t.Fatalf("GET /auth/v1/settings: %v", err)
	}
	response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("GET /auth/v1/settings status=%d", response.StatusCode)
	}
}

func acceptanceValue() string { return fmt.Sprintf("task10-acceptance-%d", time.Now().UnixNano()) }

func loginAcceptance(t *testing.T, client *http.Client, baseURL, username, password string) (string, string) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	request, _ := http.NewRequest(http.MethodPost, baseURL+"/api/session", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
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
func requestJSON(t *testing.T, client *http.Client, method, url string, payload map[string]any, cookie, csrf string) map[string]any {
	var body *bytes.Reader
	if payload == nil {
		body = bytes.NewReader(nil)
	} else {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	}
	request, _ := http.NewRequest(method, url, body)
	request.Header.Set("Cookie", cookie)
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("%s %s status=%d", method, url, response.StatusCode)
	}
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
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
