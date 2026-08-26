package provisioner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"supabase-manager/internal/contracts"
)

func TestClientSendsManagerTokenAndDecodesPrepareResponse(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 32) {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		if request.URL.Path != "/internal/v1/projects/prepare" {
			t.Errorf("path = %q", request.URL.Path)
		}
		body, _ := json.Marshal(contracts.PrepareProjectResponse{ProjectID: "project-1", Slug: "bee", Revision: 1})
		return &http.Response{StatusCode: http.StatusCreated, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)

	result, err := client.Prepare(context.Background(), contracts.PrepareProjectRequest{ProjectID: "project-1", Slug: "bee"})
	if err != nil || result.Revision != 1 {
		t.Fatalf("Prepare() = %#v, %v", result, err)
	}
}

func TestClientReturnsProvisionerErrorCode(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.ErrorEnvelope{Error: contracts.APIError{Code: "STALE_CONFIG_REVISION", Message: "stale"}})
		return &http.Response{StatusCode: http.StatusConflict, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)

	_, err := client.Prepare(context.Background(), contracts.PrepareProjectRequest{})
	if err == nil || !strings.Contains(err.Error(), "STALE_CONFIG_REVISION") {
		t.Fatalf("Prepare() error = %v, want provisioner error code", err)
	}
}

func TestClientRedactsRotationFailureAndPreservesRollbackState(t *testing.T) {
	const sentinel = "new-password-sentinel"
	httpClient := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		body, _ := json.Marshal(contracts.RotateDatabasePasswordResponse{RolledBack: true, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: sentinel}})
		return &http.Response{StatusCode: http.StatusUnprocessableEntity, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	client := NewClient("http://provisioner:9090", strings.Repeat("a", 32), httpClient)

	_, err := client.RotateDatabasePassword(context.Background(), contracts.RotateDatabasePasswordRequest{})
	if err == nil || !strings.Contains(err.Error(), "Database password rotation failed") || strings.Contains(err.Error(), sentinel) {
		t.Fatalf("RotateDatabasePassword() error = %v, want redacted typed failure", err)
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || !clientErr.RollbackSucceeded() {
		t.Fatalf("RotateDatabasePassword() error = %#v, want rollback state", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
