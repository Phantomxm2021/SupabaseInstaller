package httpapi

import (
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
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestConfigurationHTTPGetRedactsSecretLeaves(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil { t.Fatal(err) }
	defer database.Close()
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil { t.Fatal(err) }
	h := configurationHandlers{options: ConfigurationOptions{Orchestrator: configuration.NewOrchestrator(database, nil)}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects/{id}/configuration", h.get)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/projects/bee/configuration", nil))
	if response.Code != http.StatusOK { t.Fatalf("GET status = %d, body = %s", response.Code, response.Body.String()) }
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil { t.Fatal(err) }
	if _, ok := payload["anonKey"]; ok || strings.Contains(response.Body.String(), "service_role") || strings.Contains(response.Body.String(), "jwt_secret") { t.Fatalf("configuration response leaked secret material: %s", response.Body.String()) }
}

func TestConfigurationHTTPOAuthPatchUsesProviderSubresource(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil { t.Fatal(err) }
	defer database.Close()
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil { t.Fatal(err) }
	operations := operation.NewService(database, func() string { return "oauth-op" }, time.Now)
	manager := configuration.NewOrchestrator(database, operations)
	mux := http.NewServeMux()
	RegisterConfigurationRoutes(mux, ConfigurationOptions{Orchestrator: manager})
	body := strings.NewReader(`{"expectedRevision":1,"value":{"enabled":false,"clientId":"","secretSet":false,"secret":{"action":""},"fields":{}}}`)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/api/projects/bee/configuration/oauth/google", body))
	if response.Code != http.StatusAccepted { t.Fatalf("OAuth PATCH status = %d, body = %s", response.Code, response.Body.String()) }
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
