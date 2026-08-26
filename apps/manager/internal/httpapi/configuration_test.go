package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

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
