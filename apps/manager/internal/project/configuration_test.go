package project

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"supabase-manager/internal/contracts"
)

func TestDefaultConfiguration(t *testing.T) {
	got := DefaultConfiguration(contracts.PresetLightweight)
	if !got.Services.Database || !got.Services.Auth || got.Services.Storage || got.Auth.SMTP.Enabled {
		t.Fatalf("unexpected Lightweight defaults: %#v", got)
	}
	if !got.Auth.Email.Enabled || got.Auth.Phone.Enabled || got.Auth.AnonymousSignIn {
		t.Fatalf("unexpected Auth defaults: %#v", got.Auth)
	}
}

func TestOAuthProviderRegistryIsStable(t *testing.T) {
	if len(contracts.OAuthProviderNames) != 20 || contracts.OAuthProviderNames[0] != "apple" || contracts.OAuthProviderNames[len(contracts.OAuthProviderNames)-1] != "zoom" {
		t.Fatalf("unexpected OAuth registry: %#v", contracts.OAuthProviderNames)
	}
}

func TestConfigurationPatchOmitsFullConfigurationForSectionPatch(t *testing.T) {
	payload, err := json.Marshal(contracts.ConfigurationPatch{ExpectedRevision: 2, General: &contracts.GeneralConfig{Domain: "bee.example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), `"configuration"`) {
		t.Fatalf("section patch unexpectedly contains full configuration: %s", payload)
	}
}

func TestConfigurationValidationStrictInputs(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*contracts.ProjectConfiguration)
		field  string
	}{
		{"unsupported version", func(c *contracts.ProjectConfiguration) { c.General.SupabaseVersion = "self-hosted/v1.0.0" }, "general.supabaseVersion"},
		{"empty gateway", func(c *contracts.ProjectConfiguration) { c.Network.Gateway = "" }, "network.gateway"},
		{"empty https mode", func(c *contracts.ProjectConfiguration) { c.Network.HTTPSMode = "" }, "network.httpsMode"},
		{"empty log level", func(c *contracts.ProjectConfiguration) { c.Realtime.LogLevel = "" }, "realtime.logLevel"},
		{"smtp username", func(c *contracts.ProjectConfiguration) {
			c.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, SenderEmail: "noreply@example.com", SenderName: "Bee", PasswordSet: true, Password: contracts.SecretInput{Action: "retain"}}
		}, "auth.smtp.username"},
		{"smtp remove secret", func(c *contracts.ProjectConfiguration) {
			c.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "bee", SenderEmail: "noreply@example.com", SenderName: "Bee", PasswordSet: true, Password: contracts.SecretInput{Action: "remove"}}
		}, "auth.smtp.password"},
		{"oauth missing secret", func(c *contracts.ProjectConfiguration) {
			c.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true, ClientID: "client"}}
		}, "auth.oauth.google.secret"},
		{"unknown oauth field", func(c *contracts.ProjectConfiguration) {
			c.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true, ClientID: "client", SecretSet: true, Secret: contracts.SecretInput{Action: "retain"}, Fields: map[string]string{"bogus": "x"}}}
		}, "auth.oauth.google.fields.bogus"},
		{"object storage access key", func(c *contracts.ProjectConfiguration) {
			c.Storage = contracts.StorageConfig{Backend: contracts.StorageBackendS3, Bucket: "bee", Region: "us-east-1", Endpoint: "https://s3.example.com", SecretAccessKeySet: true, SecretAccessKey: contracts.SecretInput{Action: "retain"}}
		}, "storage.accessKeyId"},
		{"object storage endpoint", func(c *contracts.ProjectConfiguration) {
			c.Storage = contracts.StorageConfig{Backend: contracts.StorageBackendS3, Bucket: "bee", Region: "us-east-1", AccessKeyID: "key", SecretAccessKeySet: true, SecretAccessKey: contracts.SecretInput{Action: "retain"}}
		}, "storage.endpoint"},
		{"r2 account", func(c *contracts.ProjectConfiguration) {
			c.Storage = contracts.StorageConfig{Backend: contracts.StorageBackendR2, Bucket: "bee", AccountID: "", AccessKeyID: "key", SecretAccessKeySet: true, SecretAccessKey: contracts.SecretInput{Action: "retain"}}
		}, "storage.accountId"},
		{"phone provider", func(c *contracts.ProjectConfiguration) {
			c.Auth.Phone = contracts.PhoneAuthConfig{Enabled: true, Provider: "bogus"}
		}, "auth.phone.provider"},
		{"positive realtime bounds", func(c *contracts.ProjectConfiguration) { c.Realtime.MaxConnections = 0 }, "realtime.maxConnections"},
		{"positive pool bounds", func(c *contracts.ProjectConfiguration) { c.Pooler.MaxClientConnections = 0 }, "pooler.maxClientConnections"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfiguration(contracts.PresetLightweight)
			tc.mutate(&cfg)
			err := ValidateConfiguration(cfg)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Fields[tc.field] == "" {
				t.Fatalf("expected field %q, got %v", tc.field, err)
			}
		})
	}
}

func TestValidateConfigurationDependencies(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*contracts.ProjectConfiguration)
		field  string
	}{
		{"database mandatory", func(c *contracts.ProjectConfiguration) { c.Services.Database = false }, "services.database"},
		{"studio requires meta", func(c *contracts.ProjectConfiguration) { c.Services.PostgresMeta = false }, "services.postgresMeta"},
		{"imgproxy requires storage", func(c *contracts.ProjectConfiguration) { c.Services.Imgproxy = true }, "services.imgproxy"},
		{"vector follows logs", func(c *contracts.ProjectConfiguration) { c.Services.Vector = true }, "services.vector"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfiguration(contracts.PresetLightweight)
			tc.mutate(&cfg)
			err := ValidateConfiguration(cfg)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Fields[tc.field] == "" {
				t.Fatalf("expected field error for %s, got %v", tc.field, err)
			}
		})
	}
}
