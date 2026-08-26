package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// TestConfigurationReconcile is the repeatable black-box entry point for the
// real Docker acceptance. It is opt-in because pulling the pinned Supabase
// images is intentionally not part of the default unit/integration suite.
// Set SUPABASE_MANAGER_E2E_URL to the single public Manager URL after running
// `docker compose up -d --build --wait`.
func TestConfigurationReconcile(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("SUPABASE_MANAGER_E2E_URL"), "/")
	if baseURL == "" {
		t.Skip("set SUPABASE_MANAGER_E2E_URL to run the Docker/browser acceptance")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Get(baseURL + "/health/ready")
	if err != nil {
		t.Fatalf("Manager health request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusOK {
		t.Fatalf("Manager health status = %d, want ready", response.StatusCode)
	}

	// The opt-in probe deliberately asserts only redacted public metadata. Full
	// create/edit/rollback verification is performed through the browser using
	// the same URL; this test must never require or print administrator secrets.
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/setup/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	setupResponse, err := client.Do(request)
	if err != nil {
		t.Fatalf("setup status request: %v", err)
	}
	defer setupResponse.Body.Close()
	if setupResponse.StatusCode != http.StatusOK {
		t.Fatalf("setup status = %d, want 200", setupResponse.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(setupResponse.Body).Decode(&payload); err != nil {
		t.Fatalf("decode setup status: %v", err)
	}
	if _, ok := payload["error"]; ok {
		t.Fatalf("setup status unexpectedly returned an error envelope")
	}
}
