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

func TestClientReturnsProvisionerErrorCode(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{Code: "STALE_CONFIG_REVISION", Message: "stale"}})
		return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)

	_, err := client.Reconcile(context.Background(), contracts.ReconcileProjectRequest{})
	if err == nil || !strings.Contains(err.Error(), "STALE_CONFIG_REVISION") {
		t.Fatalf("Reconcile() error = %v, want provisioner error code", err)
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
