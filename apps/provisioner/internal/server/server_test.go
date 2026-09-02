package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	provisionerruntime "supabase-manager/apps/provisioner/internal/runtime"
	"supabase-manager/internal/contracts"
)

type functionDeployStub struct {
	archive     string
	err         error
	listErr     error
	rollbackErr error
	deleteErr   error
}

type projectfsFunctionBackend struct{ root *projectfs.Root }

func (*projectfsFunctionBackend) Lifecycle(context.Context, contracts.LifecycleRequest) error {
	return nil
}
func (*projectfsFunctionBackend) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (*projectfsFunctionBackend) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, nil
}
func (backend *projectfsFunctionBackend) DeployFunction(_ context.Context, request contracts.DeployFunctionRequest) (contracts.FunctionDeploymentResult, error) {
	_, err := backend.root.StageFunctionRelease(request.Slug, request.Name, request.OperationID, request.Archive)
	return contracts.FunctionDeploymentResult{}, err
}

func (s *functionDeployStub) Lifecycle(context.Context, contracts.LifecycleRequest) error { return nil }
func (s *functionDeployStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (s *functionDeployStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, nil
}
func (s *functionDeployStub) DeployFunction(_ context.Context, request contracts.DeployFunctionRequest) (contracts.FunctionDeploymentResult, error) {
	data, _ := io.ReadAll(request.Archive)
	s.archive = string(data)
	return contracts.FunctionDeploymentResult{}, s.err
}
func (s *functionDeployStub) ListFunctions(context.Context, contracts.FunctionOperationRequest) ([]contracts.FunctionSummary, error) {
	return []contracts.FunctionSummary{{Name: "demo"}}, s.listErr
}
func (s *functionDeployStub) RollbackFunction(context.Context, contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error) {
	return contracts.FunctionDeploymentResult{}, s.rollbackErr
}
func (s *functionDeployStub) DeleteFunction(context.Context, contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error) {
	return contracts.FunctionDeploymentResult{}, s.deleteErr
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
	stub := &reconcileStub{err: &contracts.ReconcileFailure{RollbackSucceeded: true, Response: contracts.ReconcileProjectResponse{OperationID: "op", ProjectID: "project", Revision: 1, RolledBack: true, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "Server runtime reconciliation failed"}}}}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: stub})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", contracts.ReconcileProjectRequest{OperationID: "op", IdempotencyKey: "key", ProjectID: "project", Slug: "bee"})
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"rolledBack":true`) || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRotateDatabasePasswordEndpointReturnsRedactedFailureDiagnostic(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	const password = "new-password-sentinel"
	backend := &rotationFailureStub{err: &contracts.ReconcileFailure{Cause: errors.New("runtime health is UNHEALTHY; services: auth (restarting, UNHEALTHY); POSTGRES_PASSWORD=" + password), RollbackSucceeded: true, RuntimeChanged: true}}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/rotate-database-password", contracts.RotateDatabasePasswordRequest{OperationID: "op", IdempotencyKey: "key", ProjectID: "project", Slug: "bee", OldPassword: "old-password", NewPassword: password})
	var body contracts.RotateDatabasePasswordResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body.Error == nil || body.Error.Code != "ROTATE_DATABASE_PASSWORD_FAILED" || body.Error.Message != "Database password rotation failed" || !strings.Contains(body.Diagnostic, "services: auth") || strings.Contains(response.Body.String(), password) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestRotateDatabasePasswordEndpointPreservesSanitizedReplayDiagnostic(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	const password = "new-password-sentinel"
	backend := &rotationFailureStub{
		err:    &contracts.ReconcileFailure{Cause: errors.New("retry failed")},
		result: contracts.RotateDatabasePasswordResponse{RolledBack: true, RuntimeChanged: true, Diagnostic: "first attempt failed: POSTGRES_PASSWORD=" + password},
	}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/rotate-database-password", contracts.RotateDatabasePasswordRequest{OperationID: "op", IdempotencyKey: "key", ProjectID: "project", Slug: "bee", OldPassword: "old-password", NewPassword: password})
	var body contracts.RotateDatabasePasswordResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error == nil || body.Error.Code != "ROTATE_DATABASE_PASSWORD_FAILED" || !strings.Contains(body.Diagnostic, "first attempt failed") || strings.Contains(body.Diagnostic, password) {
		t.Fatalf("response = %d %#v", response.Code, body)
	}
}

func TestRotationRollbackAndConfirmationFailuresKeepCanonicalErrorsSeparate(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	backend := &rotationOperationsStub{rollbackErr: errors.New("rollback script failed: POSTGRES_PASSWORD=new-password"), confirmErr: errors.New("journal update failed: token=secret-value")}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend})
	rollback := authenticatedJSON(t, handler, "/internal/v1/projects/rollback-database-password", contracts.RotateDatabasePasswordRequest{OperationKind: "ROLLBACK_DATABASE_PASSWORD", OperationID: "rollback-op", IdempotencyKey: "key", ProjectID: "project", Slug: "bee", OldPassword: "old-password", NewPassword: "new-password"})
	var rollbackBody contracts.RotateDatabasePasswordResponse
	if err := json.Unmarshal(rollback.Body.Bytes(), &rollbackBody); err != nil {
		t.Fatal(err)
	}
	if rollback.Code != http.StatusUnprocessableEntity || rollbackBody.Error == nil || rollbackBody.Error.Code != "ROTATE_DATABASE_PASSWORD_FAILED" || rollbackBody.Error.Message != "Database password rollback failed" || !strings.Contains(rollbackBody.Diagnostic, "rollback script failed") || strings.Contains(rollback.Body.String(), "new-password") {
		t.Fatalf("rollback = %d %#v", rollback.Code, rollbackBody)
	}
	confirmation := authenticatedJSON(t, handler, "/internal/v1/projects/confirm-database-password-rotation", contracts.ConfirmDatabasePasswordRotationRequest{OperationID: "confirm-op", IdempotencyKey: "key", ProjectID: "project", Slug: "bee", ExpectedRevision: 1, NextRevision: 2})
	var confirmationBody contracts.ErrorEnvelope
	if err := json.Unmarshal(confirmation.Body.Bytes(), &confirmationBody); err != nil {
		t.Fatal(err)
	}
	if confirmation.Code != http.StatusUnprocessableEntity || confirmationBody.Error.Code != "ROTATE_DATABASE_PASSWORD_FAILED" || confirmationBody.Error.Message != "Database password rotation confirmation failed" || !strings.Contains(confirmationBody.Diagnostic, "journal update failed") || strings.Contains(confirmation.Body.String(), "secret-value") {
		t.Fatalf("confirmation = %d %#v", confirmation.Code, confirmationBody)
	}
}

func TestStageCertificateForwardsPEMWithoutReturningIt(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	stager := &certificateStagerStub{}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, CertificateStager: stager})
	response := authenticatedJSON(t, handler, "/internal/v1/nginx/certificates/stage", contracts.StageManagedTLSRequest{
		CertificateName: "cloudflare-origin", BaseDomain: "example.com", CertificatePEM: []byte("-----BEGIN CERTIFICATE-----"), PrivateKeyPEM: []byte("-----BEGIN PRIVATE KEY-----"),
	})
	if response.Code != http.StatusOK {
		t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
	}
	if string(stager.input.PrivateKeyPEM) != "-----BEGIN PRIVATE KEY-----" || strings.Contains(response.Body.String(), "PRIVATE KEY") {
		t.Fatalf("PEM forwarding/redaction failed: input=%q body=%s", stager.input.PrivateKeyPEM, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "/etc/nginx/ssl/cloudflare-origin-example.pem") {
		t.Fatalf("response missing safe TLS paths: %s", response.Body.String())
	}
}

func TestStageCertificateFailureReturnsRedactedDiagnostic(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	const privateKey = "private-key-sentinel"
	stager := &certificateStagerStub{err: errors.New("nginx rejected certificate: private_key=" + privateKey + "; temporary file is unavailable")}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, CertificateStager: stager})
	response := authenticatedJSON(t, handler, "/internal/v1/nginx/certificates/stage", contracts.StageManagedTLSRequest{
		CertificateName: "cloudflare-origin", BaseDomain: "example.com", CertificatePEM: []byte("certificate-sentinel"), PrivateKeyPEM: []byte(privateKey),
	})
	var body contracts.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body.Error.Code != "TLS_STAGE_FAILED" || body.Error.Message != "Unable to stage managed TLS certificate" || !strings.Contains(body.Diagnostic, "temporary file is unavailable") || strings.Contains(response.Body.String(), privateKey) {
		t.Fatalf("response = %d %#v", response.Code, body)
	}
}

func TestFunctionDeployEndpointForwardsArchiveWithoutReturningIt(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	backend := &functionDeployStub{}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/projects/bee/functions/demo/deploy", strings.NewReader("zip-body"))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/zip")
	request.Header.Set("X-Operation-ID", "operation-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || backend.archive != "zip-body" || strings.Contains(response.Body.String(), "zip-body") {
		t.Fatalf("status/body/archive = %d/%s/%q", response.Code, response.Body.String(), backend.archive)
	}
}

func TestFunctionDeployEndpointDoesNotReturnArchiveControlledFailureDetail(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	const archiveSentinel = "archive-content-sentinel"
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	backend := &functionDeployStub{err: &projectfs.ArchiveIngestionError{Cause: errors.New("function archive entry " + archiveSentinel + " could not be extracted")}}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend, Logger: logger})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/projects/bee/functions/demo/deploy", strings.NewReader("zip-body"))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/zip")
	request.Header.Set("X-Operation-ID", "operation-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var body contracts.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body.Error.Code != "FUNCTION_DEPLOY_FAILED" || body.Error.Message != "Function deployment failed" || body.Diagnostic != "Function archive processing failed" || strings.Contains(response.Body.String(), archiveSentinel) || strings.Contains(logs.String(), archiveSentinel) {
		t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
	}
}

func TestFunctionDeployEndpointRedactsArchiveDerivedFilesystemPath(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	project, err := root.ProjectPath("bee")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	const entrySentinel = "archive-entry-sentinel"
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range []string{"index.ts", strings.Repeat(entrySentinel, 20) + ".ts"} {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte("export default {}")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: &projectfsFunctionBackend{root: root}, Logger: logger})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/projects/bee/functions/demo/deploy", bytes.NewReader(archive.Bytes()))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/zip")
	request.Header.Set("X-Operation-ID", "operation-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var body contracts.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body.Error.Code != "FUNCTION_DEPLOY_FAILED" || body.Error.Message != "Function deployment failed" || !strings.Contains(body.Diagnostic, "function staging filesystem") || !strings.Contains(body.Diagnostic, "file name too long") || strings.Contains(response.Body.String(), entrySentinel) || strings.Contains(logs.String(), entrySentinel) {
		t.Fatalf("response=%d body=%#v logs=%s", response.Code, body, logs.String())
	}
}

func TestFunctionDeployEndpointRedactsArchiveDerivedWritePath(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	project, err := root.ProjectPath("bee")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	const entrySentinel = "archive-write-sentinel"
	root.SetFunctionArchiveWriteHookForTest(func(destination string, _ []byte) error {
		if strings.Contains(destination, entrySentinel) {
			return &os.PathError{Op: "write", Path: destination, Err: errors.New("injected write failure")}
		}
		return nil
	})
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range []string{"index.ts", entrySentinel + ".ts"} {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte("export default {}")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: &projectfsFunctionBackend{root: root}, Logger: logger})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/projects/bee/functions/demo/deploy", bytes.NewReader(archive.Bytes()))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/zip")
	request.Header.Set("X-Operation-ID", "operation-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var body contracts.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body.Error.Code != "FUNCTION_DEPLOY_FAILED" || body.Error.Message != "Function deployment failed" || !strings.Contains(body.Diagnostic, "function staging filesystem") || !strings.Contains(body.Diagnostic, "injected write failure") || strings.Contains(response.Body.String(), entrySentinel) || strings.Contains(logs.String(), entrySentinel) {
		t.Fatalf("response=%d body=%#v logs=%s", response.Code, body, logs.String())
	}
}

func TestFunctionDeployEndpointReturnsSanitizedRuntimeFailureDiagnostic(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	backend := &functionDeployStub{err: errors.New("compose action failed: functions exited; POSTGRES_PASSWORD=secret-value")}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend, Logger: logger})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/projects/bee/functions/demo/deploy", strings.NewReader("zip-body"))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/zip")
	request.Header.Set("X-Operation-ID", "operation-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var body contracts.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body.Error.Code != "FUNCTION_DEPLOY_FAILED" || body.Error.Message != "Function deployment failed" || !strings.Contains(body.Diagnostic, "compose action failed: functions exited") || strings.Contains(body.Diagnostic, "secret-value") || !strings.Contains(logs.String(), body.Diagnostic) {
		t.Fatalf("response=%d body=%#v logs=%s", response.Code, body, logs.String())
	}
}

func TestFunctionDeployEndpointReturnsSanitizedStagingFilesystemDiagnostic(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	backend := &functionDeployStub{err: errors.New("staging filesystem sentinel: sync failed; token=secret-value")}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend, Logger: logger})
	request := httptest.NewRequest(http.MethodPost, "/internal/v1/projects/bee/functions/demo/deploy", strings.NewReader("zip-body"))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("Content-Type", "application/zip")
	request.Header.Set("X-Operation-ID", "operation-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var body contracts.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(body.Diagnostic, "staging filesystem sentinel: sync failed") || strings.Contains(body.Diagnostic, "secret-value") || !strings.Contains(logs.String(), body.Diagnostic) {
		t.Fatalf("response=%d body=%#v logs=%s", response.Code, body, logs.String())
	}
}

func TestFunctionRollbackAndDeleteFailuresReturnTypedCanonicalDiagnostics(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	backend := &functionDeployStub{rollbackErr: errors.New("rollback release failed: token=secret-value"), deleteErr: errors.New("delete release failed: token=secret-value")}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend})
	for _, endpoint := range []struct {
		method, path, code, message, detail string
	}{
		{http.MethodPost, "/internal/v1/projects/bee/functions/demo/rollback", "FUNCTION_ROLLBACK_FAILED", "Function rollback failed", "rollback release failed"},
		{http.MethodDelete, "/internal/v1/projects/bee/functions/demo", "FUNCTION_DELETE_FAILED", "Function deletion failed", "delete release failed"},
	} {
		t.Run(endpoint.code, func(t *testing.T) {
			request := httptest.NewRequest(endpoint.method, endpoint.path, nil)
			request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
			request.Header.Set("X-Operation-ID", "operation-1")
			if endpoint.method == http.MethodDelete {
				request.Header.Set("X-Confirm-Function", "demo")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var body contracts.FunctionDeploymentResult
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusUnprocessableEntity || body.Error == nil || body.Error.Code != endpoint.code || body.Error.Message != endpoint.message || !strings.Contains(body.Diagnostic, endpoint.detail) || strings.Contains(response.Body.String(), "secret-value") {
				t.Fatalf("response = %d %#v", response.Code, body)
			}
		})
	}
}

func TestFunctionManagementEndpointsRequireTypedConfirmationAndReturnMetadata(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	backend := &functionDeployStub{}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend})
	request := httptest.NewRequest(http.MethodGet, "/internal/v1/projects/bee/functions", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"demo"`) {
		t.Fatalf("list status/body = %d/%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodDelete, "/internal/v1/projects/bee/functions/demo", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
	request.Header.Set("X-Operation-ID", "op-1")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("delete without confirmation status = %d", response.Code)
	}
	request.Header.Set("X-Confirm-Function", "demo")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("delete status/body = %d/%s", response.Code, response.Body.String())
	}
}

func TestReconcileEndpointUsesServerTerminologyForGenericFailures(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: &reconcileStub{err: errors.New("runtime failure")}})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", contracts.ReconcileProjectRequest{OperationID: "op", IdempotencyKey: "key", ProjectID: "project", Slug: "bee"})
	var body contracts.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body.Error.Code != "RECONCILE_FAILED" || body.Error.Message != "Server runtime reconciliation failed" || body.Diagnostic != "runtime failure" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestInspectEndpointReturnsCanonicalErrorWithDiagnostic(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: &inspectFailureStub{err: errors.New("docker inspect failed: password=secret-value")}})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/inspect", contracts.InspectProjectRequest{ProjectID: "project-1", Slug: "bee"})
	var body contracts.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body.Error.Code != "INSPECT_FAILED" || body.Error.Message != "Project inspection failed" || !strings.Contains(body.Diagnostic, "docker inspect failed") || strings.Contains(response.Body.String(), "secret-value") {
		t.Fatalf("response = %d %#v", response.Code, body)
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

func TestReconcileReplayDoesNotLogCachedConfigurationSecret(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	executor := &serverCaptureExecutor{}
	backend := provisionerruntime.NewBackend(root, compose.NewRunner(executor), health.NewInspector(&serverSequenceSource{}))
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend, Logger: logger})
	request := contracts.ReconcileProjectRequest{
		OperationID: "op-initial", IdempotencyKey: "key-initial", ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", ExpectedRevision: 0, NextRevision: 1, APIPort: 18001,
		Configuration: contracts.ProjectConfiguration{Revision: 1, General: contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://bee.example.com", SupabaseVersion: "self-hosted/v0.8.0"}, Services: contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true}, Auth: contracts.AuthConfig{Enabled: true, Email: contracts.EmailAuthConfig{Enabled: true, AllowSignup: true}}, Database: contracts.DatabaseConfig{Version: "17", MaxConnections: 100}, Network: contracts.NetworkConfig{Gateway: contracts.GatewayEnvoy, HTTPSMode: contracts.HTTPSModeExternal, APIPort: 18001}},
		Secrets:       contracts.ProjectSecrets{DatabasePassword: "database-secret", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-secret", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"},
	}
	if response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", request); response.Code != http.StatusOK {
		t.Fatalf("initial response = %d %s", response.Code, response.Body.String())
	}
	const oldSecret = "cached-server-config-secret"
	failed := request
	failed.OperationID, failed.IdempotencyKey = "op-failed", "key-failed"
	failed.ExpectedRevision, failed.NextRevision, failed.Configuration.Revision = 1, 2, 2
	failed.Configuration.General.SiteURL = "https://failed.example.com"
	failed.Configuration.General.StudioPassword = contracts.SecretInput{Value: oldSecret}
	executor.configErr = errors.New("compose validation failed with " + oldSecret)
	initialFailure := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", failed)
	if initialFailure.Code != http.StatusUnprocessableEntity || strings.Contains(initialFailure.Body.String(), oldSecret) || strings.Contains(logs.String(), oldSecret) {
		t.Fatalf("initial failure leaked secret: %d %s logs=%s", initialFailure.Code, initialFailure.Body.String(), logs.String())
	}
	logs.Reset()
	retry := failed
	retry.Configuration = request.Configuration
	retry.RuntimeSecrets = nil
	replayed := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", retry)
	if replayed.Code != http.StatusUnprocessableEntity || strings.Contains(replayed.Body.String(), oldSecret) || strings.Contains(logs.String(), oldSecret) {
		t.Fatalf("replay leaked cached secret: %d %s logs=%s", replayed.Code, replayed.Body.String(), logs.String())
	}
}

func TestLifecycleEndpointLogsSafeFailureDetails(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	backend := &lifecycleFailureStub{err: errors.New("compose action failed: env file POSTGRES_PASSWORD=secret-value missing")}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend, Logger: logger})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/lifecycle", contracts.LifecycleRequest{ProjectID: "project-1", Slug: "bee", Action: contracts.LifecycleDeleteData})
	var body contracts.ErrorEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusUnprocessableEntity || body.Error.Code != "LIFECYCLE_FAILED" || body.Error.Message != "Project lifecycle action failed" || !strings.Contains(body.Diagnostic, "compose action failed") || strings.Contains(response.Body.String(), "secret-value") {
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
	var body contracts.ReconcileProjectResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error == nil || body.Error.Code != "RECONCILE_FAILED" || body.Error.Message != "Server runtime reconciliation failed" || !strings.Contains(body.Diagnostic, "compose action failed") || strings.Contains(response.Body.String(), "secret-value") {
		t.Fatalf("response must include a redacted diagnostic: %s", response.Body.String())
	}
}

func TestReconcileEndpointRedactsNestedConfigurationSecretInputs(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	values := []string{"studio-sentinel", "smtp-sentinel", "phone-sentinel", "oauth-sentinel", "storage-sentinel", "function-sentinel"}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	backend := &reconcileStub{err: &contracts.ReconcileFailure{Cause: errors.New("runtime rejected nested config values: " + strings.Join(values, ", "))}}
	handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: backend, Logger: logger})
	response := authenticatedJSON(t, handler, "/internal/v1/projects/reconcile", contracts.ReconcileProjectRequest{
		OperationID: "op-1", IdempotencyKey: "key-1", ProjectID: "project-1", Slug: "bee",
		Configuration: contracts.ProjectConfiguration{
			General: contracts.GeneralConfig{StudioPassword: contracts.SecretInput{Value: values[0]}},
			Auth: contracts.AuthConfig{
				SMTP:  contracts.SMTPConfig{Password: contracts.SecretInput{Value: values[1]}},
				Phone: contracts.PhoneAuthConfig{Secret: contracts.SecretInput{Value: values[2]}},
				OAuth: map[string]contracts.OAuthProviderConfig{"google": {Secret: contracts.SecretInput{Value: values[3]}}},
			},
			Storage:   contracts.StorageConfig{SecretAccessKey: contracts.SecretInput{Value: values[4]}},
			Functions: contracts.FunctionsConfig{Variables: []contracts.FunctionVariable{{Name: "APP_SECRET", Value: contracts.SecretInput{Value: values[5]}}}},
		},
	})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	for _, value := range values {
		if strings.Contains(response.Body.String(), value) || strings.Contains(logs.String(), value) {
			t.Fatalf("nested configuration secret leaked %q: response=%s logs=%s", value, response.Body.String(), logs.String())
		}
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

func TestInspectionEndpointsReturnCanonicalRedactedDiagnostics(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	backend := &hostResourcesStub{
		resourcesErr: errors.New("docker resources failed: token=secret-value"),
		portErr:      errors.New("docker binding check failed: password=secret-value"),
	}
	functionBackend := &functionDeployStub{listErr: errors.New("functions listing failed: token=secret-value")}
	for _, endpoint := range []struct {
		name, path, code, message, detail string
		backend                           Backend
	}{
		{"functions", "/internal/v1/projects/bee/functions", "FUNCTIONS_LIST_FAILED", "Unable to list functions", "functions listing failed", functionBackend},
		{"resources", "/internal/v1/host/resources", "HOST_RESOURCES_UNAVAILABLE", "Host resource inspection failed", "docker resources failed", backend},
		{"port", "/internal/v1/host/ports/8001", "HOST_PORT_UNAVAILABLE", "Host port inspection failed", "docker binding check failed", backend},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			handler := New(Options{ManagerToken: strings.Repeat("a", 32), ProjectFS: root, Backend: endpoint.backend, Logger: logger})
			request := httptest.NewRequest(http.MethodGet, endpoint.path, nil)
			request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 32))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var body contracts.ErrorEnvelope
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Error.Code != endpoint.code || body.Error.Message != endpoint.message || !strings.Contains(body.Diagnostic, endpoint.detail) || strings.Contains(response.Body.String(), "secret-value") || !strings.Contains(logs.String(), body.Diagnostic) {
				t.Fatalf("response=%d body=%#v logs=%s", response.Code, body, logs.String())
			}
		})
	}
}

type serverCaptureExecutor struct {
	calls     [][]string
	configErr error
}

func (e *serverCaptureExecutor) Run(_ context.Context, _ string, args, _ []string) ([]byte, error) {
	e.calls = append(e.calls, append([]string(nil), args...))
	if e.configErr != nil && strings.Contains(strings.Join(args, " "), "config --quiet") {
		return nil, e.configErr
	}
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

type rotationFailureStub struct {
	err    error
	result contracts.RotateDatabasePasswordResponse
}

type rotationOperationsStub struct {
	rollbackErr error
	confirmErr  error
}

func (*rotationOperationsStub) Lifecycle(context.Context, contracts.LifecycleRequest) error {
	return nil
}
func (*rotationOperationsStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (*rotationOperationsStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, nil
}
func (s *rotationOperationsStub) RollbackDatabasePassword(context.Context, contracts.RotateDatabasePasswordRequest) error {
	return s.rollbackErr
}
func (s *rotationOperationsStub) ConfirmDatabasePasswordRotation(context.Context, contracts.ConfirmDatabasePasswordRotationRequest) error {
	return s.confirmErr
}

func (*rotationFailureStub) Lifecycle(context.Context, contracts.LifecycleRequest) error { return nil }
func (*rotationFailureStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (*rotationFailureStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, nil
}
func (s *rotationFailureStub) RotateDatabasePassword(context.Context, contracts.RotateDatabasePasswordRequest) (contracts.RotateDatabasePasswordResponse, error) {
	result := s.result
	if !result.RolledBack && !result.RuntimeChanged {
		result = contracts.RotateDatabasePasswordResponse{RolledBack: true, RuntimeChanged: true}
	}
	return result, s.err
}

type certificateStagerStub struct {
	input contracts.StageManagedTLSRequest
	err   error
}

func (s *certificateStagerStub) StageCertificate(_ context.Context, input contracts.StageManagedTLSRequest) (contracts.StageManagedTLSResponse, error) {
	s.input = input
	return contracts.StageManagedTLSResponse{ManagedTLSConfig: contracts.ManagedTLSConfig{CertificateName: input.CertificateName, CertificateFile: "/etc/nginx/ssl/cloudflare-origin-example.pem", PrivateKeyFile: "/etc/nginx/ssl/cloudflare-origin-example.key"}, Created: true}, s.err
}

type inspectFailureStub struct{ err error }

func (s *inspectFailureStub) Lifecycle(context.Context, contracts.LifecycleRequest) error { return nil }
func (s *inspectFailureStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, s.err
}
func (s *inspectFailureStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, nil
}

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
	resourcesErr  error
	portErr       error
}

func (*hostResourcesStub) Lifecycle(context.Context, contracts.LifecycleRequest) error { return nil }
func (*hostResourcesStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (*hostResourcesStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, nil
}
func (stub *hostResourcesStub) HostResources(context.Context) (contracts.HostResources, error) {
	return stub.resources, stub.resourcesErr
}
func (stub *hostResourcesStub) HostPortAvailable(_ context.Context, port int) (bool, error) {
	return stub.portAvailable[port], stub.portErr
}

func (s *reconcileStub) Lifecycle(context.Context, contracts.LifecycleRequest) error { return nil }
func (s *reconcileStub) Inspect(context.Context, contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{}, nil
}
func (s *reconcileStub) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, s.err
}
