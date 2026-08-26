package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	provisionerruntime "supabase-manager/apps/provisioner/internal/runtime"
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

func TestPrepareRuntimePublicationCompletes(t *testing.T) {
	handler := newTestServer(t)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- authenticatedJSON(t, handler, "/internal/v1/projects/prepare", prepareRequest("op-timeout", "key-timeout", 0, 1))
	}()
	select {
	case response := <-done:
		if response.Code != http.StatusCreated {
			t.Fatalf("prepare status = %d, body = %s", response.Code, response.Body.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prepare did not complete within timeout")
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
	compose, err := os.ReadFile(filepath.Join(base, "bee", ".manager-runtime", "current", "docker-compose.yml"))
	if err != nil || strings.Contains(string(compose), "realtime:") {
		t.Fatalf("generated Compose error = %v, content includes disabled realtime", err)
	}
	env, err := os.ReadFile(filepath.Join(base, "bee", ".manager-runtime", "current", ".env"))
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

func TestReconcileEndpointReturnsTypedRedactedRollbackOutcome(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	stub := &reconcileStub{err: &contracts.ReconcileFailure{RollbackSucceeded: true, Response: contracts.ReconcileProjectResponse{OperationID: "op", ProjectID: "project", Revision: 1, RolledBack: true, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "Project runtime reconciliation failed"}}}}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: stub})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", contracts.ReconcileProjectRequest{OperationID: "op", IdempotencyKey: "key", ProjectID: "project", Slug: "bee"})
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"rolledBack":true`) || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestReconcileEndpointInvokesProductionBackend(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	executor := &serverCaptureExecutor{}
	source := &serverSequenceSource{}
	backend := provisionerruntime.NewBackend(root, compose.NewRunner(executor), health.NewInspector(source))
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend})
	request := contracts.ReconcileProjectRequest{
		OperationID: "op-real", IdempotencyKey: "key-real", ProjectID: "project-1", ProjectName: "Bee", Slug: "bee",
		ExpectedRevision: 0, NextRevision: 1, APIPort: 18001,
		Configuration: contracts.ProjectConfiguration{
			Revision: 1,
			General:  contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://bee.example.com", SupabaseVersion: "self-hosted/v0.8.0"},
			Services: contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true},
			Auth:     contracts.AuthConfig{Enabled: true, Email: contracts.EmailAuthConfig{Enabled: true, AllowSignup: true}},
			Database: contracts.DatabaseConfig{Version: "15", MaxConnections: 100},
			Network:  contracts.NetworkConfig{Gateway: contracts.GatewayEnvoy, HTTPSMode: contracts.HTTPSModeExternal, APIPort: 18001},
		},
		Secrets: contracts.ProjectSecrets{DatabasePassword: "database-secret", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-secret", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"},
	}
	response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(executor.calls) < 3 || !strings.Contains(strings.Join(executor.calls[0], " "), "config --quiet") {
		t.Fatalf("production backend compose calls = %#v", executor.calls)
	}
	stale := request
	stale.OperationID, stale.IdempotencyKey = "op-stale", "key-stale"
	stale.ExpectedRevision, stale.NextRevision, stale.Configuration.Revision = 0, 2, 2
	if response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", stale); response.Code != http.StatusConflict {
		t.Fatalf("stale status = %d, body = %s", response.Code, response.Body.String())
	}
	invalid := request
	invalid.OperationID, invalid.IdempotencyKey = "op-invalid", "key-invalid"
	invalid.ExpectedRevision, invalid.NextRevision, invalid.Configuration.Revision = 1, 2, 1
	if response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", invalid); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid revision status = %d, body = %s", response.Code, response.Body.String())
	}
	failed := request
	failed.OperationID, failed.IdempotencyKey = "op-failed", "key-failed"
	failed.ExpectedRevision, failed.NextRevision, failed.Configuration.Revision = 1, 2, 2
	failed.Configuration.General.SiteURL = "https://failed.example.com"
	failure := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", failed)
	if failure.Code != http.StatusUnprocessableEntity || !strings.Contains(failure.Body.String(), `"rolledBack":true`) {
		t.Fatalf("failure status = %d, body = %s", failure.Code, failure.Body.String())
	}
	callCount := len(executor.calls)
	replay := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", failed)
	if replay.Code != http.StatusUnprocessableEntity || replay.Body.String() != failure.Body.String() || len(executor.calls) != callCount {
		t.Fatalf("failure replay status/body/calls = %d/%s/%d, want %d/%s/%d", replay.Code, replay.Body.String(), len(executor.calls), failure.Code, failure.Body.String(), callCount)
	}
}

type serverCaptureExecutor struct{ calls [][]string }

func (e *serverCaptureExecutor) Run(_ context.Context, _ string, args, _ []string) ([]byte, error) {
	e.calls = append(e.calls, append([]string(nil), args...))
	return nil, nil
}

type serverSequenceSource struct{ calls int }

func (source *serverSequenceSource) Containers(context.Context, string) ([]health.Container, error) {
	source.calls++
	services := []string{"db", "api-gw", "auth", "meta", "rest", "studio"}
	containers := make([]health.Container, 0, len(services))
	state, healthState := "running", "healthy"
	if source.calls == 2 {
		state, healthState = "running", "unhealthy"
	}
	for _, service := range services {
		containers = append(containers, health.Container{Service: service, State: state, Health: healthState})
	}
	return containers, nil
}

type reconcileStub struct{ err error }

func (s *reconcileStub) Lifecycle(context.Context, contracts.LifecycleRequest) error { return nil }
func (s *reconcileStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (s *reconcileStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, s.err
}
