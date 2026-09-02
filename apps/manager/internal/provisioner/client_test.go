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

func TestClientDeployFunctionPreservesProvisionerDiagnostic(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{
			Code:    "FUNCTION_DEPLOY_FAILED",
			Message: "function archive requires root index.ts",
		}})
		return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)
	_, err := client.DeployFunction(context.Background(), "bee", "demo", "op-1", strings.NewReader("zip-body"))
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || clientErr.Code != "FUNCTION_DEPLOY_FAILED" || clientErr.Message != "function archive requires root index.ts" {
		t.Fatalf("DeployFunction() error = %#v, want typed provisioner diagnostic", err)
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
	for candidate, want := range map[string]string{
		"STALE_CONFIG_REVISION":   "Server configuration revision is stale",
		"INVALID_CONFIG_REVISION": "Server configuration revision is invalid",
		"RECONCILE_FAILED":        "Server runtime reconciliation failed",
		"LIFECYCLE_FAILED":        "Server lifecycle operation failed",
		"INSPECT_FAILED":          "Server inspection failed",
	} {
		code = candidate
		_, err := client.Reconcile(context.Background(), contracts.ReconcileProjectRequest{})
		var clientErr *ClientError
		if !errors.As(err, &clientErr) || clientErr.Message != want {
			t.Fatalf("Reconcile(%s) error = %#v, want message %q", candidate, err, want)
		}
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

func TestClientPreservesRedactedReconcileDiagnostic(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.ReconcileProjectResponse{RuntimeChanged: true, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "runtime health is unhealthy; services: auth (restarting, unhealthy)"}})
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
		return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(`{"rolledBack":true,"runtimeChanged":true,"error":{"code":"ROTATE_DATABASE_PASSWORD_FAILED","message":"Database password rotation failed"},"diagnostic":"` + diagnostic + `"}`)), Header: make(http.Header)}, nil
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
