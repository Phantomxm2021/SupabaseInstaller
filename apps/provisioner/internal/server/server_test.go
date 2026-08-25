package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

func TestProvisionerRejectsMissingServiceToken(t *testing.T) {
	handler := newTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/projects/prepare", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestProvisionerRejectsStaleConfigRevision(t *testing.T) {
	handler := newTestServer(t)
	first := prepareRequest("op-1", "key-1", 0, 4)
	response := authenticatedJSON(t, handler, "/internal/v1/projects/prepare", first)
	if response.Code != http.StatusCreated {
		t.Fatalf("first prepare status = %d, body = %s", response.Code, response.Body.String())
	}
	stale := prepareRequest("op-2", "key-2", 3, 5)
	response = authenticatedJSON(t, handler, "/internal/v1/projects/prepare", stale)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale prepare status = %d, want 409", response.Code)
	}
}

func TestProvisionerReturnsStoredResponseForIdempotencyKey(t *testing.T) {
	handler := newTestServer(t)
	request := prepareRequest("op-1", "same-key", 0, 1)
	first := authenticatedJSON(t, handler, "/internal/v1/projects/prepare", request)
	second := authenticatedJSON(t, handler, "/internal/v1/projects/prepare", request)
	if first.Body.String() != second.Body.String() || second.Code != http.StatusCreated {
		t.Fatalf("idempotent responses differ: first=%s second=%s", first.Body.String(), second.Body.String())
	}
}

func TestPrepareWritesGeneratedComposeAndSecretEnv(t *testing.T) {
	base := t.TempDir()
	root, _ := projectfs.New(base)
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/prepare", prepareRequest("op-1", "key-1", 0, 1))
	if response.Code != http.StatusCreated {
		t.Fatalf("prepare status = %d, body = %s", response.Code, response.Body.String())
	}
	compose, err := os.ReadFile(filepath.Join(base, "bee", "docker-compose.yml"))
	if err != nil || strings.Contains(string(compose), "realtime:") {
		t.Fatalf("generated Compose error = %v, content includes disabled realtime", err)
	}
	env, err := os.ReadFile(filepath.Join(base, "bee", ".env"))
	if err != nil || !strings.Contains(string(env), "POSTGRES_PASSWORD=database-secret") {
		t.Fatalf("generated env error = %v, content = %s", err, env)
	}
}

func prepareRequest(operationID, key string, expected, next int64) contracts.PrepareProjectRequest {
	return contracts.PrepareProjectRequest{
		OperationID: operationID, IdempotencyKey: key, ProjectID: "project-1", Slug: "bee",
		ExpectedRevision: expected, NextRevision: next, Domain: "bee.example.com", SiteURL: "https://example.com", APIPort: 18001,
		Secrets: contracts.ProjectSecrets{DatabasePassword: "database-secret", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-secret", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"},
	}
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatalf("projectfs.New() error = %v", err)
	}
	return New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root})
}

func authenticatedJSON(t *testing.T, handler http.Handler, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
