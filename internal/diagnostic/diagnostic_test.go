package diagnostic

import (
	"strings"
	"testing"

	"supabase-manager/internal/contracts"
)

func TestConfigurationSecretValuesIncludesEverySecretInput(t *testing.T) {
	values := ConfigurationSecretValues(contracts.ProjectConfiguration{
		General: contracts.GeneralConfig{StudioPassword: contracts.SecretInput{Value: "studio-secret"}},
		Auth: contracts.AuthConfig{
			SMTP:  contracts.SMTPConfig{Password: contracts.SecretInput{Value: "smtp-secret"}},
			Phone: contracts.PhoneAuthConfig{Secret: contracts.SecretInput{Value: "phone-secret"}},
			OAuth: map[string]contracts.OAuthProviderConfig{"google": {Secret: contracts.SecretInput{Value: "oauth-secret"}}},
		},
		Storage:   contracts.StorageConfig{SecretAccessKey: contracts.SecretInput{Value: "storage-secret"}},
		Functions: contracts.FunctionsConfig{Variables: []contracts.FunctionVariable{{Value: contracts.SecretInput{Value: "function-secret"}}}},
	})
	for _, secret := range []string{"studio-secret", "smtp-secret", "phone-secret", "oauth-secret", "storage-secret", "function-secret"} {
		if !contains(values, secret) {
			t.Fatalf("ConfigurationSecretValues() omitted %q: %q", secret, values)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestSanitizeRedactsLongestKnownSecretsFirst(t *testing.T) {
	got := Sanitize("failed with overlapping-secret-value", []string{"secret", "overlapping-secret-value"})
	if strings.Contains(got, "secret") || strings.Contains(got, "overlapping-secret-value") {
		t.Fatalf("Sanitize leaked a known secret: %q", got)
	}
	if got != "failed with [REDACTED]" {
		t.Fatalf("Sanitize = %q", got)
	}
}

func TestSanitizeRedactsCredentialAssignmentsAndBearerTokens(t *testing.T) {
	input := "POSTGRES_PASSWORD=hunter2 api_key: abc123 Authorization: Bearer header.payload.signature Authorization: Basic dXNlcjpwYXNz Bearer standalone-token"
	got := Sanitize(input, nil)
	for _, leaked := range []string{"hunter2", "abc123", "header.payload.signature", "dXNlcjpwYXNz", "standalone-token"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("Sanitize leaked %q: %q", leaked, got)
		}
	}
	if strings.Count(got, "[REDACTED]") != 5 {
		t.Fatalf("Sanitize markers = %d, want 5: %q", strings.Count(got, "[REDACTED]"), got)
	}
}

func TestSanitizeFlattensControlsAndBoundsOutput(t *testing.T) {
	got := Sanitize(" first\nsecond\tthird\x00 "+strings.Repeat("x", maxOutputBytes), nil)
	if strings.ContainsAny(got, "\n\r\t\x00") {
		t.Fatalf("Sanitize retained a control character: %q", got)
	}
	if len(got) > maxOutputBytes {
		t.Fatalf("Sanitize length = %d, want at most %d", len(got), maxOutputBytes)
	}
}
