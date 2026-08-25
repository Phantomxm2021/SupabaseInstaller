package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/auth"
	"supabase-manager/apps/manager/internal/store"
)

func TestSetupCreatesFirstAdminAndReturnsRecoveryCodes(t *testing.T) {
	handler, _ := newAuthHandler(t)
	body := bytes.NewBufferString(`{"username":"admin","password":"correct horse battery staple"}`)
	request := httptest.NewRequest(http.MethodPost, "https://manager.example.com/api/setup", body)
	request.Header.Set("Origin", "https://manager.example.com")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || len(payload.RecoveryCodes) != 10 {
		t.Fatalf("response = %s, error = %v, want 10 recovery codes", response.Body.String(), err)
	}
}

func TestLoginSetsSecureHttpOnlyStrictCookie(t *testing.T) {
	handler, service := newAuthHandler(t)
	_, _ = service.Bootstrap(context.Background(), "admin", "correct horse battery staple")
	body := bytes.NewBufferString(`{"username":"admin","password":"correct horse battery staple"}`)
	request := httptest.NewRequest(http.MethodPost, "https://manager.example.com/api/session", body)
	request.Header.Set("Origin", "https://manager.example.com")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != SessionCookieName || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("session cookie = %#v, want Secure HttpOnly SameSite=Strict", cookies)
	}
}

func TestMutationRejectsCrossOriginRequest(t *testing.T) {
	handler, _ := newAuthHandler(t)
	request := httptest.NewRequest(http.MethodPost, "https://manager.example.com/api/setup", bytes.NewBufferString(`{}`))
	request.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func TestProtectedMutationRejectsCrossOriginBeforeSessionProcessing(t *testing.T) {
	_, service := newAuthHandler(t)
	protected := ProtectAPI(AuthOptions{Service: service, PublicOrigin: "https://manager.example.com", SecureCookies: true}, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "https://manager.example.com/api/projects", bytes.NewBufferString(`{}`))
	request.Header.Set("Origin", "https://evil.example")
	response := httptest.NewRecorder()

	protected.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func newAuthHandler(t *testing.T) (http.Handler, *auth.Service) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) }
	service := auth.NewService(database, auth.NewPasswordHasher(auth.Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}), rand.Reader, now)
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthOptions{Service: service, PublicOrigin: "https://manager.example.com", SecureCookies: true})
	return mux, service
}
