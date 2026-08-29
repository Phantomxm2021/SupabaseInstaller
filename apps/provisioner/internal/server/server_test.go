package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	provisionerruntime "supabase-manager/apps/provisioner/internal/runtime"
	"supabase-manager/internal/contracts"
)

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
	stub := &reconcileStub{err: &contracts.ReconcileFailure{RollbackSucceeded: true, Response: contracts.ReconcileProjectResponse{OperationID: "op", ProjectID: "project", Revision: 1, RolledBack: true, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "Server runtime reconciliation failed"}}}}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: stub})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", contracts.ReconcileProjectRequest{OperationID: "op", IdempotencyKey: "key", ProjectID: "project", Slug: "bee"})
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"rolledBack":true`) || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestReconcileEndpointUsesServerTerminologyForGenericFailures(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: &reconcileStub{err: errors.New("runtime failure")}})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", contracts.ReconcileProjectRequest{OperationID: "op", IdempotencyKey: "key", ProjectID: "project", Slug: "bee"})
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"message":"Server runtime reconciliation failed"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHostPortEndpointReturnsDockerBindingAvailability(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: &hostResourcesStub{portAvailable: map[int]bool{8001: false, 8002: true}}})
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/host/ports/8001", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":false`) {
		t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/internal/v1/host/ports/nope", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid port status = %d, body = %s", response.Code, response.Body.String())
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
			Database: contracts.DatabaseConfig{Version: "17", MaxConnections: 100},
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

func TestLifecycleEndpointLogsSafeFailureDetails(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	backend := &lifecycleFailureStub{err: errors.New("compose action failed: env file POSTGRES_PASSWORD=secret-value missing")}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend, Logger: logger})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/lifecycle", contracts.LifecycleRequest{ProjectID: "project-1", Slug: "bee", Action: contracts.LifecycleDeleteData})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(logs.String(), "project lifecycle failed") || !strings.Contains(logs.String(), "project-1") || strings.Contains(logs.String(), "secret-value") {
		t.Fatalf("unsafe or missing lifecycle log: %s", logs.String())
	}
}

func TestReconcileEndpointLogsSafeFailureDetails(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	backend := &reconcileStub{err: &contracts.ReconcileFailure{Cause: errors.New("compose action failed: POSTGRES_PASSWORD=secret-value missing env file")}}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend, Logger: logger})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", contracts.ReconcileProjectRequest{OperationID: "op-1", IdempotencyKey: "key-1", ProjectID: "project-1", Slug: "bee"})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(logs.String(), "project runtime reconciliation started") || !strings.Contains(logs.String(), "project runtime reconciliation failed") || !strings.Contains(logs.String(), "project-1") || strings.Contains(logs.String(), "secret-value") {
		t.Fatalf("unsafe or missing reconcile log: %s", logs.String())
	}
}

func TestReconcileEndpointReturnsRedactedFailureDiagnostic(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	backend := &reconcileStub{err: &contracts.ReconcileFailure{Cause: errors.New("compose action failed: POSTGRES_PASSWORD=secret-value missing env file")}}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", contracts.ReconcileProjectRequest{OperationID: "op-1", IdempotencyKey: "key-1", ProjectID: "project-1", Slug: "bee"})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "compose action failed") || strings.Contains(response.Body.String(), "secret-value") {
		t.Fatalf("response must include a redacted diagnostic: %s", response.Body.String())
	}
}

func TestHostResourcesEndpointReturnsReadOnlySnapshot(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend := &hostResourcesStub{resources: contracts.HostResources{CPUPercent: 31, CPUCores: 10, MemoryTotalBytes: 1024, DiskTotalBytes: 2048}}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend})
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/host/resources", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"cpuPercent":31`) || !strings.Contains(response.Body.String(), `"cpuCores":10`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type serverCaptureExecutor struct{ calls [][]string }

func (e *serverCaptureExecutor) Run(_ context.Context, _ string, args, _ []string) ([]byte, error) {
	e.calls = append(e.calls, append([]string(nil), args...))
	if strings.Contains(strings.Join(args, " "), "exec -T db psql") {
		return []byte("schema:auth:supabase_admin\nschema:graphql_public:supabase_admin\nfunction:auth.email:supabase_auth_admin\nfunction:auth.role:supabase_auth_admin\nfunction:auth.uid:supabase_auth_admin\n"), nil
	}
	return nil, nil
}

func (e *serverCaptureExecutor) RunInput(_ context.Context, _ string, args, _ []string, _ []byte) ([]byte, error) {
	e.calls = append(e.calls, append([]string(nil), args...))
	return nil, nil
}

type serverSequenceSource struct{ calls int }

func (source *serverSequenceSource) Containers(context.Context, string) ([]health.Container, error) {
	source.calls++
	services := []string{"db", "api-gw", "auth", "auth-templates", "meta", "rest", "studio"}
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

type lifecycleFailureStub struct{ err error }

func (stub *lifecycleFailureStub) Lifecycle(context.Context, contracts.LifecycleRequest) error {
	return stub.err
}
func (*lifecycleFailureStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (*lifecycleFailureStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, nil
}

type hostResourcesStub struct {
	resources     contracts.HostResources
	portAvailable map[int]bool
}

func (*hostResourcesStub) Lifecycle(context.Context, contracts.LifecycleRequest) error { return nil }
func (*hostResourcesStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (*hostResourcesStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, nil
}
func (stub *hostResourcesStub) HostResources(context.Context) (contracts.HostResources, error) {
	return stub.resources, nil
}
func (stub *hostResourcesStub) HostPortAvailable(_ context.Context, port int) (bool, error) {
	return stub.portAvailable[port], nil
}

func (s *reconcileStub) Lifecycle(context.Context, contracts.LifecycleRequest) error { return nil }
func (s *reconcileStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (s *reconcileStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, s.err
}
