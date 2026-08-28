package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"supabase-manager/apps/nginxproxy/internal/site"
)

func TestApplyRequiresBearerTokenAndForwardsTypedRequest(t *testing.T) {
	store := &recordingStore{}
	handler := New("agent-token", site.NewRenderer(site.TLSPaths{
		CertificateFile: "/etc/nginx/cert.pem", CertificateKeyFile: "/etc/nginx/key.pem",
	}), store)

	request := httptest.NewRequest(http.MethodPost, "/v1/sites/apply", bytes.NewBufferString(`{
		"slug":"bee","domain":"bee.example.com","apiPort":18001,"studioPort":18002,"studioEnabled":true,"studioUsername":"operator","studioPassword":"studio-password"
	}`))
	request.Header.Set("Authorization", "Bearer agent-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got, want := response.Code, http.StatusNoContent; got != want {
		t.Fatalf("status = %d, want %d: %s", got, want, response.Body.String())
	}
	if got, want := store.applied.AvailableName, "supabase-manager-bee.conf"; got != want {
		t.Fatalf("available name = %q, want %q", got, want)
	}
}

func TestApplyRejectsUnauthenticatedAndUnknownFields(t *testing.T) {
	handler := New("agent-token", site.NewRenderer(site.TLSPaths{
		CertificateFile: "/etc/nginx/cert.pem", CertificateKeyFile: "/etc/nginx/key.pem",
	}), &recordingStore{})

	unauthenticated := httptest.NewRequest(http.MethodPost, "/v1/sites/apply", bytes.NewBufferString(`{}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, unauthenticated)
	if got, want := response.Code, http.StatusUnauthorized; got != want {
		t.Fatalf("unauthenticated status = %d, want %d", got, want)
	}

	unknownField := httptest.NewRequest(http.MethodPost, "/v1/sites/apply", bytes.NewBufferString(`{
		"slug":"bee","domain":"bee.example.com","apiPort":18001,"rawNginx":"server {}"
	}`))
	unknownField.Header.Set("Authorization", "Bearer agent-token")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, unknownField)
	if got, want := response.Code, http.StatusBadRequest; got != want {
		t.Fatalf("unknown-field status = %d, want %d", got, want)
	}
}

type recordingStore struct {
	applied site.RenderedSite
}

func (s *recordingStore) Apply(_ context.Context, rendered site.RenderedSite) error {
	s.applied = rendered
	return nil
}

func (*recordingStore) Remove(context.Context, string) error { return nil }
