package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/configuration"
	"supabase-manager/apps/manager/internal/operation"
	projectservice "supabase-manager/apps/manager/internal/project"
	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestConfigurationHTTPGetRedactsSecretLeaves(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	h := configurationHandlers{options: ConfigurationOptions{Orchestrator: configuration.NewOrchestrator(database, nil)}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{id}/configuration", h.get)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/projects/bee/configuration", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["anonKey"]; ok || strings.Contains(response.Body.String(), "service_role") || strings.Contains(response.Body.String(), "jwt_secret") {
		t.Fatalf("configuration response leaked secret material: %s", response.Body.String())
	}
}

func TestConfigurationHTTPOAuthPatchUsesProviderSubresource(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	operations := operation.NewService(database, func() string { return "oauth-op" }, time.Now)
	manager := configuration.NewOrchestrator(database, operations)
	mux := http.NewServeMux()
	RegisterConfigurationRoutes(mux, ConfigurationOptions{Orchestrator: manager})
	body := strings.NewReader(`{"value":{"enabled":false,"clientId":"","secretSet":false,"secret":{"action":""},"fields":{}}}`)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/projects/bee/configuration/oauth/google", body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("OAuth PATCH status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestConfigurationHTTPRejectsUnsupportedNetworkFields(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterConfigurationRoutes(mux, ConfigurationOptions{Orchestrator: configuration.NewOrchestrator(database, nil)})
	body := strings.NewReader(`{"value":{"gateway":"envoy","httpsMode":"external","certificate":"unexpected"}}`)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/projects/bee/configuration/network", body))
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsupported network field status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestConfigurationBusyIsTypedConflict(t *testing.T) {
	response := httptest.NewRecorder()
	h := configurationHandlers{}
	h.handleConfigError(response, store.ErrConfigurationBusy)
	if response.Code != http.StatusConflict {
		t.Fatalf("busy status = %d, want 409", response.Code)
	}
	if strings.Contains(response.Body.String(), "project configuration operation is busy") {
		t.Fatalf("busy response leaked internal sentinel: %s", response.Body.String())
	}
}

func TestDecodeRawRejectsTrailingJSON(t *testing.T) {
	var value map[string]any
	err := decodeRaw([]byte(`{"services":{}} {"ignored":true}`), &value)
	if err == nil {
		t.Fatalf("decodeRaw trailing value error = %v", err)
	}
}

func TestConfigurationPatchRejectsUnknownAndTrailingJSON(t *testing.T) {
	mux := http.NewServeMux()
	RegisterConfigurationRoutes(mux, ConfigurationOptions{Orchestrator: configuration.NewOrchestrator(nil, nil)})
	for name, body := range map[string]string{
		"unknown field":  `{"value":{},"ignored":true}`,
		"trailing value": `{"value":{}} {"ignored":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, "/api/projects/bee/configuration/general", strings.NewReader(body))
			mux.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s; want 400", response.Code, response.Body.String())
			}
		})
	}
}

func TestConfigurationPatchNotFoundUsesProjectNotFound(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	mux := http.NewServeMux()
	RegisterConfigurationRoutes(mux, ConfigurationOptions{Orchestrator: configuration.NewOrchestrator(database, operation.NewService(database, func() string { return "op" }, time.Now))})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/projects/missing/configuration/general", strings.NewReader(`{"value":{"domain":"missing.example.com","siteUrl":"https://missing.example.com","supabaseVersion":"self-hosted/v0.8.0"}}`))
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "PROJECT_NOT_FOUND") {
		t.Fatalf("status/body = %d/%s; want 404 PROJECT_NOT_FOUND", response.Code, response.Body.String())
	}
}

func TestMergeSMTPPatchRetainsUntouchedConfiguredOAuthSecret(t *testing.T) {
	base := contracts.AuthConfig{
		SMTP: contracts.SMTPConfig{PasswordSet: true, Password: contracts.SecretInput{Action: "replace", Value: "smtp-new"}},
		OAuth: map[string]contracts.OAuthProviderConfig{
			"google": {Enabled: true, SecretSet: true, Secret: contracts.SecretInput{Action: ""}},
		},
	}
	incoming := contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "mailer", PasswordSet: true, Password: contracts.SecretInput{Action: "retain"}, SenderEmail: "bee@example.com"}
	merged := mergeSMTPAuthPatch(base, incoming)
	if merged.SMTP.Password.Action != "retain" {
		t.Fatalf("SMTP action = %q, want retain", merged.SMTP.Password.Action)
	}
	if merged.OAuth["google"].Secret.Action != "retain" {
		t.Fatalf("untouched OAuth action = %q, want retain", merged.OAuth["google"].Secret.Action)
	}
}

func TestMergeOAuthPatchRetainsUntouchedConfiguredSMTPSecret(t *testing.T) {
	base := contracts.AuthConfig{
		SMTP: contracts.SMTPConfig{Enabled: true, PasswordSet: true, Password: contracts.SecretInput{Action: ""}},
		OAuth: map[string]contracts.OAuthProviderConfig{
			"google": {Enabled: true, SecretSet: true, Secret: contracts.SecretInput{Action: "replace", Value: "oauth-new"}},
		},
	}
	incoming := contracts.OAuthProviderConfig{Enabled: true, ClientID: "client", SecretSet: true, Secret: contracts.SecretInput{Action: "retain"}}
	merged := mergeOAuthAuthPatch(base, "google", incoming)
	if merged.SMTP.Password.Action != "retain" {
		t.Fatalf("untouched SMTP action = %q, want retain", merged.SMTP.Password.Action)
	}
	if merged.OAuth["google"].Secret.Action != "retain" {
		t.Fatalf("OAuth action = %q, want retain", merged.OAuth["google"].Secret.Action)
	}
}

func TestOAuthPatchWithNewProviderRetainsConfiguredSMTPAndReplacesSecret(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, err := managersecrets.NewCipher(bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "mailer.example.com", SiteURL: "https://mailer.example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "mailer", PasswordSet: true, SenderEmail: "mailer@example.com", SenderName: "Mailer"}
	proj := contracts.Project{ID: "mailer", Name: "Mailer", Slug: "mailer", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), proj, cfg); err != nil {
		t.Fatal(err)
	}
	smtp, _ := cipher.Encrypt(proj.ID, "smtp.password", []byte("smtp-old"))
	if err := database.PutSecret(context.Background(), proj.ID, "smtp.password", smtp); err != nil {
		t.Fatal(err)
	}
	manager := configuration.NewOrchestrator(database, operation.NewService(database, func() string { return "oauth-http-op" }, time.Now), cipher)
	mux := http.NewServeMux()
	RegisterConfigurationRoutes(mux, ConfigurationOptions{Orchestrator: manager})
	body := strings.NewReader(`{"value":{"enabled":true,"clientId":"google-client","secretSet":true,"secret":{"value":"google-secret"},"fields":{}}}`)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/projects/mailer/configuration/oauth/google", body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("OAuth PATCH status = %d, body = %s", response.Code, response.Body.String())
	}
	stored, err := database.GetSecret(context.Background(), proj.ID, "smtp.password")
	if err != nil {
		t.Fatalf("SMTP secret disappeared: %v", err)
	}
	if plain, _ := cipher.Decrypt(proj.ID, "smtp.password", stored); string(plain) != "smtp-old" {
		t.Fatalf("SMTP secret = %q, want retained", plain)
	}
	if _, err := database.GetSecret(context.Background(), proj.ID, "oauth.google.secret"); err != nil {
		t.Fatalf("Google replacement not stored: %v", err)
	}
}
