package provisioner

import (
	"context"
	"encoding/json"
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
