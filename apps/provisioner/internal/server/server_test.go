package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	first := contracts.PrepareProjectRequest{OperationID: "op-1", IdempotencyKey: "key-1", ProjectID: "project-1", Slug: "bee", ExpectedRevision: 0, NextRevision: 4}
	response := authenticatedJSON(t, handler, "/internal/v1/projects/prepare", first)
	if response.Code != http.StatusCreated {
		t.Fatalf("first prepare status = %d, body = %s", response.Code, response.Body.String())
	}
	stale := contracts.PrepareProjectRequest{OperationID: "op-2", IdempotencyKey: "key-2", ProjectID: "project-1", Slug: "bee", ExpectedRevision: 3, NextRevision: 5}
	response = authenticatedJSON(t, handler, "/internal/v1/projects/prepare", stale)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale prepare status = %d, want 409", response.Code)
	}
}

func TestProvisionerReturnsStoredResponseForIdempotencyKey(t *testing.T) {
	handler := newTestServer(t)
	request := contracts.PrepareProjectRequest{OperationID: "op-1", IdempotencyKey: "same-key", ProjectID: "project-1", Slug: "bee", ExpectedRevision: 0, NextRevision: 1}
	first := authenticatedJSON(t, handler, "/internal/v1/projects/prepare", request)
	second := authenticatedJSON(t, handler, "/internal/v1/projects/prepare", request)
	if first.Body.String() != second.Body.String() || second.Code != http.StatusCreated {
		t.Fatalf("idempotent responses differ: first=%s second=%s", first.Body.String(), second.Body.String())
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
