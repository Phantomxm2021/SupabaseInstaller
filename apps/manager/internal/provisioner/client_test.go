package provisioner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"supabase-manager/internal/contracts"
)

func TestClientDefaultRequestTimeoutAllowsDurableRuntimeRecovery(t *testing.T) {
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), nil)
	if client.http.Timeout != DefaultRequestTimeout || client.http.Timeout < 5*time.Minute {
		t.Fatalf("default provisioner timeout=%s, want at least five minutes", client.http.Timeout)
	}
}

func TestClientDeployFunctionStreamsArchiveToTypedProvisionerRoute(t *testing.T) {
	var archive string
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.Path != "/internal/v1/projects/bee/functions/demo/deploy" || request.Header.Get("X-Operation-ID") != "op-1" || request.Header.Get("Authorization") == "" {
			t.Fatalf("request = %s %s headers=%v", request.Method, request.URL.Path, request.Header)
		}
		data, _ := io.ReadAll(request.Body)
		archive = string(data)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"current":{"sha256":"abc"}}`)), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
	result, err := client.DeployFunction(context.Background(), "bee", "demo", "op-1", strings.NewReader("zip-body"))
	if err != nil || archive != "zip-body" || result.Current == nil || result.Current.SHA256 != "abc" {
		t.Fatalf("result/archive/error = %#v/%q/%v", result, archive, err)
	}
}

func TestClientDeployFunctionPreservesAllowListedProvisionerDiagnostic(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{
			Code:    "FUNCTION_DEPLOY_FAILED",
			Message: "untrusted error message",
		}, Diagnostic: "function archive requires root index.ts"})
		return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
	_, err := client.DeployFunction(context.Background(), "bee", "demo", "op-1", strings.NewReader("zip-body"))
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || clientErr.Code != "FUNCTION_DEPLOY_FAILED" || clientErr.Message != "function archive requires root index.ts" {
		t.Fatalf("DeployFunction() error = %#v, want typed provisioner diagnostic", err)
	}
}

func TestClientFunctionActionsPreserveTypedProvisionerDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		name       string
		call       func(*Client) error
		code       string
		diagnostic string
	}{
		{"rollback", func(c *Client) error {
			_, err := c.RollbackFunction(context.Background(), "bee", "demo", "op-1")
			return err
		}, "FUNCTION_ROLLBACK_FAILED", "previous function release is unavailable"},
		{"delete", func(c *Client) error {
			_, err := c.DeleteFunction(context.Background(), "bee", "demo", "op-1")
			return err
		}, "FUNCTION_DELETE_FAILED", "function release cleanup is incomplete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				body, _ := json.Marshal(contracts.FunctionDeploymentResult{Error: &contracts.APIError{Code: tc.code, Message: "untrusted error message"}, Diagnostic: tc.diagnostic})
				return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
			})}
			client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
			var clientErr *ClientError
			if err := tc.call(client); !errors.As(err, &clientErr) || clientErr.Code != tc.code || clientErr.Message != tc.diagnostic {
				t.Fatalf("function action error = %#v, want %s diagnostic %q", err, tc.code, tc.diagnostic)
			}
		})
	}
}

func TestReconcileWireRequestOmitsRetiredRevisionProtocol(t *testing.T) {
	var body []byte
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ = io.ReadAll(request.Body)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"operationId":"op","projectId":"project","enabledServices":[]}`)), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
	_, err := client.Reconcile(context.Background(), contracts.ReconcileProjectRequest{OperationID: "op", ProjectID: "project", Slug: "bee", IdempotencyKey: "legacy", ExpectedRevision: 4, NextRevision: 5, Fence: 9})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"idempotencyKey", "expectedRevision", "nextRevision", "fence"} {
		if strings.Contains(string(body), field) {
			t.Fatalf("wire request contains retired field %q: %s", field, body)
		}
	}
}

func TestClientCanonicalizesOperationErrorsWithServerTerminology(t *testing.T) {
	var code string
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{Code: code}})
		return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
	for _, tc := range []struct {
		code string
		want string
		call func(*Client) error
	}{
		{"STALE_CONFIG_REVISION", "Server configuration revision is stale", func(c *Client) error {
			_, err := c.Reconcile(context.Background(), contracts.ReconcileProjectRequest{})
			return err
		}},
		{"INVALID_CONFIG_REVISION", "Server configuration revision is invalid", func(c *Client) error {
			_, err := c.Reconcile(context.Background(), contracts.ReconcileProjectRequest{})
			return err
		}},
		{"RECONCILE_FAILED", "Server runtime reconciliation failed", func(c *Client) error {
			_, err := c.Reconcile(context.Background(), contracts.ReconcileProjectRequest{})
			return err
		}},
		{"LIFECYCLE_FAILED", "Server lifecycle operation failed", func(c *Client) error { return c.Lifecycle(context.Background(), contracts.LifecycleRequest{}) }},
		{"INSPECT_FAILED", "Server inspection failed", func(c *Client) error {
			_, err := c.Inspect(context.Background(), contracts.InspectProjectRequest{})
			return err
		}},
	} {
		code = tc.code
		err := tc.call(client)
		var clientErr *ClientError
		if !errors.As(err, &clientErr) || clientErr.Message != tc.want {
			t.Fatalf("operation(%s) error = %#v, want message %q", tc.code, err, tc.want)
		}
	}
}

func TestClientPreservesDiagnosticsOnlyForAllowListedEndpointAndCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		call func(*Client) error
		code string
		want string
	}{
		{"lifecycle", "/internal/v1/projects/lifecycle", func(c *Client) error { return c.Lifecycle(context.Background(), contracts.LifecycleRequest{}) }, "LIFECYCLE_FAILED", "compose action failed: api-gw exited"},
		{"inspect", "/internal/v1/projects/inspect", func(c *Client) error {
			_, err := c.Inspect(context.Background(), contracts.InspectProjectRequest{})
			return err
		}, "INSPECT_FAILED", "docker inspection timed out"},
		{"tls", "/internal/v1/nginx/certificates/stage", func(c *Client) error {
			_, err := c.StageManagedTLS(context.Background(), contracts.StageManagedTLSRequest{})
			return err
		}, "TLS_STAGE_FAILED", "certificate staging directory is unavailable"},
		{"invalid request", "/internal/v1/projects/lifecycle", func(c *Client) error { return c.Lifecycle(context.Background(), contracts.LifecycleRequest{}) }, "INVALID_REQUEST", "untrusted-invalid-request-diagnostic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				if request.URL.Path != tc.path {
					t.Fatalf("path = %s, want %s", request.URL.Path, tc.path)
				}
				body, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{Code: tc.code, Message: "untrusted error message"}, Diagnostic: tc.want})
				return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
			})}
			client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
			var clientErr *ClientError
			err := tc.call(client)
			if !errors.As(err, &clientErr) {
				t.Fatalf("error = %#v, want ClientError", err)
			}
			if tc.code == "INVALID_REQUEST" {
				if !strings.Contains(clientErr.Message, "Provisioner request is invalid") || strings.Contains(clientErr.Message, tc.want) {
					t.Fatalf("invalid request message = %q, want canonical message without %q", clientErr.Message, tc.want)
				}
			} else if clientErr.Message != tc.want {
				t.Fatalf("error = %#v, want message %q", err, tc.want)
			}
		})
	}
}

func TestClientChecksHostPortAvailability(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet || request.URL.Path != "/internal/v1/host/ports/8001" {
			t.Fatalf("request = %s %s, want host port check", request.Method, request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"port":8001,"available":false}`)), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
	available, err := client.HostPortAvailable(context.Background(), 8001)
	if err != nil {
		t.Fatalf("HostPortAvailable() error = %v", err)
	}
	if available {
		t.Fatal("HostPortAvailable() = true, want false")
	}
}

func TestClientDoesNotInferRuntimeOutcomeFromGenericErrorEnvelope(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{Code: "RECONCILE_FAILED", Message: "failed"}})
		return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
	_, err := client.Reconcile(context.Background(), contracts.ReconcileProjectRequest{})
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || clientErr.RuntimeOutcomeKnown() {
		t.Fatalf("generic reconcile error = %#v, want unknown runtime outcome", err)
	}
}

func TestClientPreservesVersionedReconcileDiagnostic(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.ReconcileProjectResponse{RuntimeChanged: true, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "untrusted error message"}, Diagnostic: "runtime health is unhealthy; services: auth (restarting, unhealthy)", DiagnosticVersion: contracts.DiagnosticVersionCompleteRedaction})
		return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)

	_, err := client.Reconcile(context.Background(), contracts.ReconcileProjectRequest{})
	if err == nil || !strings.Contains(err.Error(), "services: auth") {
		t.Fatalf("Reconcile() error = %v, want provisioner diagnostic", err)
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || !clientErr.RuntimeOutcomeKnown() || !clientErr.RuntimeChanged() {
		t.Fatalf("Reconcile() error = %#v, want known changed runtime outcome", err)
	}
}

func TestClientRejectsUnversionedReconcileDiagnostic(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.ReconcileProjectResponse{RuntimeChanged: true, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "untrusted error message"}, Diagnostic: "unsafe stale diagnostic"})
		return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
	_, err := client.Reconcile(context.Background(), contracts.ReconcileProjectRequest{})
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || clientErr.Message != "Server runtime reconciliation failed" {
		t.Fatalf("Reconcile() error = %#v, want canonical message", err)
	}
}

func TestClientRejectsGenericReconcileAndRotationEnvelopeDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name      string
		call      func(*Client) error
		code      string
		canonical string
	}{
		{"reconcile", func(c *Client) error {
			_, err := c.Reconcile(context.Background(), contracts.ReconcileProjectRequest{})
			return err
		}, "RECONCILE_FAILED", "Server runtime reconciliation failed"},
		{"rotation", func(c *Client) error {
			_, err := c.RotateDatabasePassword(context.Background(), contracts.RotateDatabasePasswordRequest{})
			return err
		}, "ROTATE_DATABASE_PASSWORD_FAILED", "Database password rotation failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const sentinel = "unversioned-generic-envelope-diagnostic"
			httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				body, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{Code: tc.code, Message: "untrusted error message"}, Diagnostic: sentinel})
				return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
			})}
			client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
			var clientErr *ClientError
			err := tc.call(client)
			if !errors.As(err, &clientErr) || clientErr.Message != tc.canonical || strings.Contains(clientErr.Message, sentinel) {
				t.Fatalf("error = %#v, want canonical message %q without %q", err, tc.canonical, sentinel)
			}
		})
	}
}

func TestClientDiagnosticRoutesRequireExactFunctionAndHostPaths(t *testing.T) {
	functionPayload, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{Code: "FUNCTION_DEPLOY_FAILED"}, Diagnostic: "function diagnostic"})
	hostPayload, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{Code: "HOST_PORT_UNAVAILABLE"}, Diagnostic: "host diagnostic"})
	for _, tc := range []struct {
		name      string
		path      string
		payload   []byte
		want      string
		preserved bool
	}{
		{"function deploy", "/internal/v1/projects/bee/functions/demo/deploy", functionPayload, "function diagnostic", true},
		{"function lookalike", "/evil/functions/demo/deploy", functionPayload, "function diagnostic", false},
		{"host port", "/internal/v1/host/ports/8001", hostPayload, "host diagnostic", true},
		{"host lookalike", "/internal/v1/host/ports/123/extra", hostPayload, "host diagnostic", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientErr := clientErrorForPayload(tc.path, http.StatusUnprocessableEntity, tc.payload)
			if tc.preserved && clientErr.Message != tc.want {
				t.Fatalf("message = %q, want diagnostic %q", clientErr.Message, tc.want)
			}
			if !tc.preserved && strings.Contains(clientErr.Message, tc.want) {
				t.Fatalf("message = %q, must not preserve diagnostic %q", clientErr.Message, tc.want)
			}
		})
	}
}

func TestClientRedactsRotationFailureAndPreservesRollbackState(t *testing.T) {
	const sentinel = "new-password-sentinel"
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.RotateDatabasePasswordResponse{RolledBack: true, RuntimeChanged: true, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: sentinel}})
		return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)

	_, err := client.RotateDatabasePassword(context.Background(), contracts.RotateDatabasePasswordRequest{})
	if err == nil || !strings.Contains(err.Error(), "Database password rotation failed") || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("RotateDatabasePassword() error = %v, want redacted typed failure", err)
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || !clientErr.RollbackSucceeded() || !clientErr.RuntimeOutcomeKnown() || !clientErr.RuntimeChanged() {
		t.Fatalf("RotateDatabasePassword() error = %#v, want rollback state", err)
	}
}

func TestClientPreservesProvisionerRotationDiagnostic(t *testing.T) {
	const diagnostic = "runtime health is UNHEALTHY; services: auth (restarting, UNHEALTHY)"
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(`{"rolledBack":true,"runtimeChanged":true,"error":{"code":"ROTATE_DATABASE_PASSWORD_FAILED","message":"Database password rotation failed"},"diagnostic":"` + diagnostic + `","diagnosticVersion":1}`)), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)

	_, err := client.RotateDatabasePassword(context.Background(), contracts.RotateDatabasePasswordRequest{})
	if err == nil || !strings.Contains(err.Error(), diagnostic) {
		t.Fatalf("RotateDatabasePassword() error = %v, want provisioner diagnostic", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
