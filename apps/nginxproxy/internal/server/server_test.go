package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"supabase-manager/apps/nginxproxy/internal/site"
	"supabase-manager/internal/contracts"
)

func TestStageCertificateRequiresBearerTokenAndReturnsOnlySafeMetadata(t *testing.T) {
	staged := &recordingCertificateStore{}
	handler := New("agent-token", site.NewRenderer(site.TLSPaths{CertificateFile: "/etc/nginx/ssl/cloudflare-origin.pem", CertificateKeyFile: "/etc/nginx/ssl/cloudflare-origin.key"}), &recordingStore{}, staged)
	payload, err := json.Marshal(site.CertificateInput{
		Name: "cloudflare-origin", BaseDomain: "beegame.studio", CertificatePEM: []byte("certificate"), PrivateKeyPEM: []byte("private-key"),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/certificates/stage", bytes.NewReader(payload))
	request.Header.Set("Authorization", "Bearer agent-token")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if staged.input.Name != "cloudflare-origin" || staged.input.BaseDomain != "beegame.studio" {
		t.Fatalf("staged input = %#v", staged.input)
	}
	if bytes.Contains(response.Body.Bytes(), []byte("private-key")) || bytes.Contains(response.Body.Bytes(), []byte("-----BEGIN")) {
		t.Fatalf("response leaked PEM material: %s", response.Body.String())
	}
}

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

func TestOperationalFailuresReturnTypedRedactedDiagnostics(t *testing.T) {
	const bearerToken = "proxy-token-sentinel"
	const studioPassword = "studio-password-sentinel"
	const certificate = "certificate-sentinel"
	const privateKey = "private-key-sentinel"

	tests := []struct {
		name    string
		path    string
		payload string
		store   *recordingStore
		stager  *recordingCertificateStore
		status  int
		code    string
		message string
		cause   string
		secrets []string
	}{
		{
			name:    "apply",
			path:    "/v1/sites/apply",
			payload: `{"slug":"bee","domain":"bee.example.com","apiPort":18001,"studioPort":18002,"studioEnabled":true,"studioUsername":"operator","studioPassword":"studio-password-sentinel"}`,
			store:   &recordingStore{applyErr: errors.New("nginx reload failed Authorization: Bearer proxy-token-sentinel for studio-password-sentinel")},
			status:  http.StatusInternalServerError,
			code:    "PROXY_APPLY_FAILED",
			message: "Unable to apply managed Nginx site",
			cause:   "nginx reload failed",
			secrets: []string{bearerToken, studioPassword},
		},
		{
			name:    "remove",
			path:    "/v1/sites/remove",
			payload: `{"slug":"bee"}`,
			store:   &recordingStore{removeErr: errors.New("nginx remove failed Authorization: Bearer proxy-token-sentinel")},
			status:  http.StatusInternalServerError,
			code:    "PROXY_REMOVE_FAILED",
			message: "Unable to remove managed Nginx site",
			cause:   "nginx remove failed",
			secrets: []string{bearerToken},
		},
		{
			name:    "stage certificate",
			path:    "/v1/certificates/stage",
			payload: `{"certificateName":"cloudflare-origin","baseDomain":"bee.example.com","certificatePem":"Y2VydGlmaWNhdGUtc2VudGluZWw=","privateKeyPem":"cHJpdmF0ZS1rZXktc2VudGluZWw="}`,
			store:   &recordingStore{},
			stager:  &recordingCertificateStore{stageErr: errors.New("certificate staging failed: certificate-sentinel; private-key-sentinel")},
			status:  http.StatusUnprocessableEntity,
			code:    "PROXY_TLS_STAGE_FAILED",
			message: "Unable to stage managed TLS certificate",
			cause:   "certificate staging failed",
			secrets: []string{certificate, privateKey},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := New("agent-token", site.NewRenderer(site.TLSPaths{CertificateFile: "/etc/nginx/cert.pem", CertificateKeyFile: "/etc/nginx/key.pem"}), test.store, test.stager)
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.payload))
			request.Header.Set("Authorization", "Bearer agent-token")
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			var body contracts.ErrorEnvelope
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode failure envelope: %v; body=%s", err, response.Body.String())
			}
			if response.Code != test.status || body.Error.Code != test.code || body.Error.Message != test.message || !strings.Contains(body.Diagnostic, test.cause) {
				t.Fatalf("status=%d body=%+v", response.Code, body)
			}
			for _, secret := range test.secrets {
				if strings.Contains(response.Body.String(), secret) {
					t.Fatalf("response leaked %q: %s", secret, response.Body.String())
				}
			}
		})
	}
}

type recordingStore struct {
	applied   site.RenderedSite
	applyErr  error
	removeErr error
}

type recordingCertificateStore struct {
	input    site.CertificateInput
	stageErr error
}

func (s *recordingCertificateStore) Stage(_ context.Context, input site.CertificateInput) (site.CertificateResult, error) {
	s.input = input
	if s.stageErr != nil {
		return site.CertificateResult{}, s.stageErr
	}
	return site.CertificateResult{CertificateName: input.Name, CertificateFile: "/etc/nginx/ssl/cloudflare-origin-beegame.pem", PrivateKeyFile: "/etc/nginx/ssl/cloudflare-origin-beegame.key", Created: true}, nil
}

func (s *recordingStore) Apply(_ context.Context, rendered site.RenderedSite) error {
	s.applied = rendered
	return s.applyErr
}

func (s *recordingStore) Remove(context.Context, string) error { return s.removeErr }
