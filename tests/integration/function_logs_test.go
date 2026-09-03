//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFunctionLogs(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("SUPABASE_MANAGER_E2E_URL"), "/")
	username, password := os.Getenv("SUPABASE_MANAGER_E2E_USERNAME"), os.Getenv("SUPABASE_MANAGER_E2E_PASSWORD")
	if baseURL == "" || username == "" || password == "" {
		t.Skip("set SUPABASE_MANAGER_E2E_URL, SUPABASE_MANAGER_E2E_USERNAME, and SUPABASE_MANAGER_E2E_PASSWORD for function-log acceptance")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	ensureSetup(t, client, baseURL, username, password)
	csrf, cookie := loginAcceptance(t, client, baseURL, username, password)
	created := createCustomSMTPFunctionsProject(t, client, baseURL, cookie, csrf)
	projectID := created["projectId"].(string)
	defer cleanupRuntimeProject(t, client, baseURL, projectID, cookie, csrf)
	waitOperation(t, client, baseURL, created["operationId"].(string), cookie, csrf)

	project := getJSON(t, client, baseURL+"/api/projects/"+projectID, cookie, csrf)
	slug := project["slug"].(string)
	recordRuntimeProject(t, "supabase-manager-"+slug)
	configuration := getJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration", cookie, csrf)
	canonical := configuration["configuration"].(map[string]any)
	services := canonical["services"].(map[string]any)
	if enabled, _ := services["logs"].(bool); enabled {
		t.Fatal("standard function-log acceptance project unexpectedly enabled optional Logs/Logflare")
	}
	network := canonical["network"].(map[string]any)
	supabaseURL := fmt.Sprintf("http://127.0.0.1:%d", int(network["apiPort"].(float64)))
	anon := requestJSON(t, client, http.MethodPost, baseURL+"/api/projects/"+projectID+"/secrets/anonKey/reveal", map[string]any{"password": password}, cookie, csrf)["value"].(string)

	apiMarker := fmt.Sprintf("API_LOG_%d", time.Now().UnixNano())
	pushMarker := fmt.Sprintf("PUSH_LOG_%d", time.Now().UnixNano())
	deployFunctionLogFixture(t, client, baseURL, projectID, "api", apiMarker, true, cookie, csrf)
	deployFunctionLogFixture(t, client, baseURL, projectID, "deliver-push", pushMarker, false, cookie, csrf)

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, invocation := range []struct{ name, marker string }{{"api", apiMarker}, {"deliver-push", pushMarker}} {
		invocation := invocation
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- invokeFunction(client, supabaseURL, anon, invocation.name, invocation.marker)
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}

	apiPage := pollFunctionLogs(t, client, baseURL, projectID, "api", apiMarker, cookie, csrf, 60*time.Second)
	pushPage := pollFunctionLogs(t, client, baseURL, projectID, "deliver-push", pushMarker, cookie, csrf, 60*time.Second)
	assertFunctionLogIsolation(t, apiPage, "api", apiMarker, pushMarker, true)
	assertFunctionLogIsolation(t, pushPage, "deliver-push", pushMarker, apiMarker, false)
	assertNewestFirstAndCursors(t, client, baseURL, projectID, "api", cookie, csrf, apiPage)

	t.Run("collector failure does not fail functions", func(t *testing.T) {
		compose := runtimeComposeCommand(t, slug)
		restart := func() error { return runCompose(compose, "up", "-d", "--wait", "function-log-collector") }
		if err := runCompose(compose, "stop", "function-log-collector"); err != nil {
			t.Fatalf("stop function-log-collector: %v", err)
		}
		restarted := false
		defer func() {
			if !restarted {
				if err := restart(); err != nil {
					t.Errorf("restart function-log-collector during cleanup: %v", err)
				}
			}
		}()
		failureMarker := fmt.Sprintf("PUSH_WHILE_COLLECTOR_OFFLINE_%d", time.Now().UnixNano())
		if err := invokeFunction(client, supabaseURL, anon, "deliver-push", failureMarker); err != nil {
			t.Fatalf("healthy function failed while collector was stopped: %v", err)
		}
		page := pollFunctionLogHealth(t, client, baseURL, projectID, "deliver-push", cookie, csrf, 60*time.Second,
			func(status string) bool {
				return status == "offline" || status == "dropped" || status == "storage_error"
			})
		assertNoMessage(t, page, failureMarker)
		assertNoMessage(t, page, apiMarker)
		if err := restart(); err != nil {
			t.Fatalf("restart function-log-collector: %v", err)
		}
		restarted = true
		pollFunctionLogHealth(t, client, baseURL, projectID, "deliver-push", cookie, csrf, 60*time.Second,
			func(status string) bool { return status == "healthy" })
	})

	t.Run("optional Logs enabled does not duplicate or cross managed logs", func(t *testing.T) {
		if os.Getenv("SUPABASE_MANAGER_E2E_LOGS_ENABLED") != "1" {
			t.Skip("set SUPABASE_MANAGER_E2E_LOGS_ENABLED=1 to pay the additional Logflare/Vector acceptance cost")
		}
		current := getJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration", cookie, csrf)
		value := current["configuration"].(map[string]any)["services"].(map[string]any)
		value["logs"] = true
		operation := patchJSON(t, client, baseURL+"/api/projects/"+projectID+"/configuration/services", map[string]any{
			"expectedRevision": int64(current["revision"].(float64)),
			"value":            value,
		}, cookie, csrf)
		waitOperation(t, client, baseURL, operation["operationId"].(string), cookie, csrf)

		enabledAPI := fmt.Sprintf("API_WITH_LOGFLARE_%d", time.Now().UnixNano())
		enabledPush := fmt.Sprintf("PUSH_WITH_LOGFLARE_%d", time.Now().UnixNano())
		if err := invokeFunction(client, supabaseURL, anon, "api", enabledAPI); err != nil {
			t.Fatal(err)
		}
		if err := invokeFunction(client, supabaseURL, anon, "deliver-push", enabledPush); err != nil {
			t.Fatal(err)
		}
		apiEnabledPage := pollFunctionLogs(t, client, baseURL, projectID, "api", enabledAPI, cookie, csrf, 60*time.Second)
		pushEnabledPage := pollFunctionLogs(t, client, baseURL, projectID, "deliver-push", enabledPush, cookie, csrf, 60*time.Second)
		assertMarkerOnce(t, apiEnabledPage, enabledAPI)
		assertMarkerOnce(t, pushEnabledPage, enabledPush)
		assertNoMessage(t, apiEnabledPage, enabledPush)
		assertNoMessage(t, pushEnabledPage, enabledAPI)
	})
}

func recordRuntimeProject(t *testing.T, project string) {
	t.Helper()
	os.Setenv("SUPABASE_MANAGER_E2E_RUNTIME_PROJECT", project)
	if marker := os.Getenv("SUPABASE_MANAGER_E2E_RUNTIME_PROJECT_FILE"); marker != "" {
		if err := os.WriteFile(marker, []byte(project), 0o600); err != nil {
			t.Fatalf("record runtime Compose project: %v", err)
		}
	}
}

func deployFunctionLogFixture(t *testing.T, client *http.Client, baseURL, projectID, name, marker string, throw bool, cookie, csrf string) {
	t.Helper()
	throwLine := ""
	if throw {
		throwLine = `queueMicrotask(() => { throw new Error("UNCAUGHT_` + marker + `") })`
	}
	source := `let emitted = false
Deno.serve(async (request) => {
  const body = await request.text()
  console.log("` + marker + `", body)
  if (!emitted) { emitted = true; ` + throwLine + ` }
  return new Response("ok:" + body, { status: 200 })
})
`
	operation := multipartFunctionRequest(t, baseURL+"/api/projects/"+projectID+"/functions/"+name+"/deploy", name+".zip", zipSource(t, source), cookie, csrf)
	waitOperation(t, client, baseURL, operation, cookie, csrf)
}

func zipSource(t *testing.T, source string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("index.ts")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, source); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func multipartFunctionRequest(t *testing.T, rawURL, filename string, archive []byte, cookie, csrf string) string {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("archive", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(archive); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, rawURL, &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", strings.SplitN(rawURL, "/api/", 2)[0])
	request.Header.Set("Cookie", cookie)
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("deploy %s status=%d", filename, response.StatusCode)
	}
	var result struct {
		OperationID string `json:"operationId"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.OperationID == "" {
		t.Fatal("deploy response omitted operationId")
	}
	return result.OperationID
}

func invokeFunction(client *http.Client, baseURL, anon, name, marker string) error {
	request, err := http.NewRequest(http.MethodPost, baseURL+"/functions/v1/"+url.PathEscape(name), strings.NewReader(marker))
	if err != nil {
		return err
	}
	request.Header.Set("apikey", anon)
	request.Header.Set("Authorization", "Bearer "+anon)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("invoke %s: %w", name, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), marker) {
		return fmt.Errorf("invoke %s status=%d returned unexpected response", name, response.StatusCode)
	}
	return nil
}

func pollFunctionLogs(t *testing.T, client *http.Client, baseURL, projectID, name, marker, cookie, csrf string, timeout time.Duration) map[string]any {
	t.Helper()
	return pollFunctionLogPage(t, client, baseURL, projectID, name, cookie, csrf, timeout, func(page map[string]any) bool {
		return pageContains(page, marker)
	})
}

func pollFunctionLogHealth(t *testing.T, client *http.Client, baseURL, projectID, name, cookie, csrf string, timeout time.Duration, accept func(string) bool) map[string]any {
	t.Helper()
	return pollFunctionLogPage(t, client, baseURL, projectID, name, cookie, csrf, timeout, func(page map[string]any) bool {
		health, _ := page["health"].(map[string]any)
		status, _ := health["status"].(string)
		return accept(status)
	})
}

func pollFunctionLogPage(t *testing.T, client *http.Client, baseURL, projectID, name, cookie, csrf string, timeout time.Duration, done func(map[string]any) bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	endpoint := baseURL + "/api/projects/" + projectID + "/functions/" + name + "/logs?limit=200"
	var last map[string]any
	for time.Now().Before(deadline) {
		last = getJSON(t, client, endpoint, cookie, csrf)
		if done(last) {
			return last
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("function %s logs did not reach expected state within %s (health=%v)", name, timeout, last["health"])
	return nil
}

func pageContains(page map[string]any, marker string) bool {
	for _, raw := range page["logs"].([]any) {
		if strings.Contains(raw.(map[string]any)["message"].(string), marker) {
			return true
		}
	}
	return false
}

func assertNoMessage(t *testing.T, page map[string]any, marker string) {
	t.Helper()
	if pageContains(page, marker) {
		t.Fatalf("log page unexpectedly contains marker %q", marker)
	}
}

func assertMarkerOnce(t *testing.T, page map[string]any, marker string) {
	t.Helper()
	count := 0
	for _, raw := range page["logs"].([]any) {
		if strings.Contains(raw.(map[string]any)["message"].(string), marker) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("marker %q occurred %d times, want exactly once", marker, count)
	}
}

func assertFunctionLogIsolation(t *testing.T, page map[string]any, owner, ownMarker, otherMarker string, wantException bool) {
	t.Helper()
	if !pageContains(page, ownMarker) {
		t.Fatalf("%s logs omit own marker", owner)
	}
	assertNoMessage(t, page, otherMarker)
	exceptions := 0
	for _, raw := range page["logs"].([]any) {
		record := raw.(map[string]any)
		if record["functionName"] != owner {
			t.Fatalf("%s page contains owner %v", owner, record["functionName"])
		}
		if record["eventType"] == "UncaughtException" {
			exceptions++
		}
		if record["eventType"] == "Boot" && record["functionName"] != owner {
			t.Fatalf("Boot event attributed to %v, want %s", record["functionName"], owner)
		}
	}
	if (exceptions > 0) != wantException {
		t.Fatalf("%s UncaughtException count=%d, wantException=%v", owner, exceptions, wantException)
	}
	health := page["health"].(map[string]any)
	if health["status"] != "healthy" {
		t.Fatalf("%s canonical health=%v", owner, health)
	}
}

func assertNewestFirstAndCursors(t *testing.T, client *http.Client, baseURL, projectID, name, cookie, csrf string, page map[string]any) {
	t.Helper()
	logs := page["logs"].([]any)
	for i := 1; i < len(logs); i++ {
		previous, _ := time.Parse(time.RFC3339Nano, logs[i-1].(map[string]any)["timestamp"].(string))
		current, _ := time.Parse(time.RFC3339Nano, logs[i].(map[string]any)["timestamp"].(string))
		if current.After(previous) {
			t.Fatalf("logs are not newest-first at index %d", i)
		}
	}
	first := getJSON(t, client, baseURL+"/api/projects/"+projectID+"/functions/"+name+"/logs?limit=1", cookie, csrf)
	older, _ := first["olderCursor"].(string)
	if older == "" {
		t.Fatal("limited function-log page omitted olderCursor")
	}
	second := getJSON(t, client, baseURL+"/api/projects/"+projectID+"/functions/"+name+"/logs?limit=1&before="+url.QueryEscape(older), cookie, csrf)
	if len(second["logs"].([]any)) == 0 {
		t.Fatal("olderCursor did not return an older page")
	}
}

func runtimeComposeCommand(t *testing.T, slug string) []string {
	t.Helper()
	root := os.Getenv("SUPABASE_MANAGER_E2E_PROJECT_ROOT")
	if root == "" {
		t.Fatal("SUPABASE_MANAGER_E2E_PROJECT_ROOT is required for collector acceptance")
	}
	dir := filepath.Join(root, slug, ".manager-runtime", "current")
	return []string{"compose", "--file", filepath.Join(dir, "docker-compose.yml"), "--project-directory", dir, "--project-name", "supabase-manager-" + slug}
}

func runCompose(prefix []string, args ...string) error {
	command := exec.Command("docker", append(prefix, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose action failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}
