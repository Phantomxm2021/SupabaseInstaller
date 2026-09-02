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
	if got.Storage.UploadFileSizeLimit != 50*1024*1024 {
		t.Fatalf("storage upload limit = %d, want 50 MiB", got.Storage.UploadFileSizeLimit)
	}
	if got.Database.Version != "17" {
		t.Fatalf("database version = %q, want the single supported PostgreSQL 17 runtime", got.Database.Version)
	}
	if got.Pooler.TransactionPort != 0 || got.Pooler.SessionPort != 0 {
		t.Fatalf("manager-owned pooler ports = %d/%d, want zero create defaults", got.Pooler.TransactionPort, got.Pooler.SessionPort)
	}
	if !got.Services.Database || !got.Services.Auth || got.Services.Storage || got.Auth.SMTP.Enabled {
		t.Fatalf("unexpected Lightweight defaults: %#v", got)
	}
	if !got.Auth.Email.Enabled || got.Auth.Phone.Enabled || got.Auth.AnonymousSignIn {
		t.Fatalf("unexpected Auth defaults: %#v", got.Auth)
	}
	if got.Auth.RateLimits != (contracts.RateLimitConfig{EmailSent: 30, SMSSent: 30, TokenRefresh: 150, TokenVerification: 30, AnonymousUsers: 30, SignupsAndSignins: 30}) {
		t.Fatalf("unexpected Auth rate limit defaults: %#v", got.Auth.RateLimits)
	}
	if got.Auth.MFA != (contracts.MFAConfig{TOTPEnrollEnabled: true, TOTPVerifyEnabled: true, MaxEnrolledFactors: 10, PhoneOTPLength: 6}) {
		t.Fatalf("unexpected Auth MFA defaults: %#v", got.Auth.MFA)
	}
}

func TestStorageUploadFileSizeLimitBounds(t *testing.T) {
	for _, limit := range []int64{1*1024*1024 - 1, 5*1024*1024*1024 + 1} {
		cfg := DefaultConfiguration(contracts.PresetLightweight)
		cfg.Storage.UploadFileSizeLimit = limit
		var validation *ValidationError
		if err := ValidateConfiguration(cfg); !errors.As(err, &validation) || validation.Fields["storage.uploadFileSizeLimit"] == "" {
			t.Fatalf("limit %d: expected upload limit validation, got %v", limit, err)
		}
	}
}

func TestR2RequiresLowercaseAccountIDAndPathStyle(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Storage = contracts.StorageConfig{Backend: contracts.StorageBackendR2, Bucket: "bee", AccountID: "ABC", AccessKeyID: "key", SecretAccessKeySet: true, SecretAccessKey: contracts.SecretInput{Action: "retain"}}
	var validation *ValidationError
	if err := ValidateConfiguration(cfg); !errors.As(err, &validation) || validation.Fields["storage.accountId"] == "" || validation.Fields["storage.forcePathStyle"] == "" {
		t.Fatalf("expected R2 account/path validation, got %v", err)
	}
}

func TestStoredConfigurationAcceptsLegacyCaddy(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Network.HTTPSMode = contracts.HTTPSModeCaddy
	if err := ValidateStoredConfiguration(cfg); err != nil {
		t.Fatalf("stored legacy Caddy configuration rejected: %v", err)
	}
}

func TestConfigurationRejectsLegacyPostgreSQL15(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Database.Version = "15"
	err := ValidateConfiguration(cfg)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["database.version"] == "" {
		t.Fatalf("ValidateConfiguration() error = %v, want database.version validation error", err)
	}
}

func TestConfigurationRejectsPhoneMFAWithoutProvider(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Auth.MFA.PhoneEnrollEnabled = true
	err := ValidateConfiguration(cfg)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["auth.mfa.phoneEnrollEnabled"] == "" {
		t.Fatalf("ValidateConfiguration() error = %v, want auth.mfa.phoneEnrollEnabled validation error", err)
	}
}

func TestConfigurationRejectsNewCaddyValue(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Network.HTTPSMode = contracts.HTTPSModeCaddy
	err := ValidateConfiguration(cfg)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["network.httpsMode"] == "" {
		t.Fatalf("ValidateConfiguration() error = %v, want network.httpsMode validation error", err)
	}
}

func TestAuthRateLimitsAndMFAValidation(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Auth.RateLimits.EmailSent = -1
	cfg.Auth.RateLimits.TokenRefresh = 0
	cfg.Auth.MFA.MaxEnrolledFactors = 0
	cfg.Auth.MFA.PhoneOTPLength = 3
	err := ValidateConfiguration(cfg)
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("ValidateConfiguration() error = %v, want ValidationError", err)
	}
	for _, field := range []string{"auth.rateLimits.emailSent", "auth.rateLimits.tokenRefresh", "auth.mfa.maxEnrolledFactors", "auth.mfa.phoneOtpLength"} {
		if validation.Fields[field] == "" {
			t.Errorf("missing validation error for %s: %#v", field, validation.Fields)
		}
	}
}

func TestAuthMailerTemplatesRejectInvalidTemplateBodies(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Auth.Mailer.Templates.Confirmation.Body = `{{ .Unsupported }}`
	err := ValidateConfiguration(cfg)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["auth.mailer.templates.confirmation.body"] == "" {
		t.Fatalf("ValidateConfiguration() error = %v, want invalid template body error", err)
	}
}

func TestAuthMailerTemplatesAllowSubjectAndBodyVariables(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Auth.Mailer.Templates.Confirmation.Subject = "Confirm {{ .Email }}"
	cfg.Auth.Mailer.Templates.Confirmation.Body = "<a href=\"{{ .ConfirmationURL }}\">Confirm</a>"
	if err := ValidateConfiguration(cfg); err != nil {
		t.Fatalf("ValidateConfiguration() error = %v, want nil", err)
	}
}

func TestAuthMailerTemplatesHaveSaneDefaults(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	if cfg.Auth.Mailer.Templates.Confirmation.Subject == "" || cfg.Auth.Mailer.Templates.Confirmation.Body == "" {
		t.Fatalf("confirmation template defaults = %#v, want subject and default HTML body", cfg.Auth.Mailer.Templates.Confirmation)
	}
	if cfg.Auth.Mailer.Notifications.PasswordChanged.Enabled {
		t.Fatal("password changed notification should default disabled")
	}
}

func TestOAuthProviderRegistryIsStable(t *testing.T) {
	if len(contracts.OAuthProviderNames) != 20 || contracts.OAuthProviderNames[0] != "apple" || contracts.OAuthProviderNames[len(contracts.OAuthProviderNames)-1] != "zoom" {
		t.Fatalf("unexpected OAuth registry: %#v", contracts.OAuthProviderNames)
	}
}

func TestPhoneValidationRejectsDisabledUnknownConfiguration(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Auth.Phone = contracts.PhoneAuthConfig{Provider: "bogus", Fields: map[string]string{"authToken": "plaintext"}}
	err := ValidateConfiguration(cfg)
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["auth.phone.provider"] == "" || validation.Fields["auth.phone.fields.authToken"] == "" {
		t.Fatalf("expected disabled Phone provider and field errors, got %v", err)
	}
}

func TestPhoneProvidersRequireTypedFieldsAndSecret(t *testing.T) {
	cases := []struct {
		provider string
		field    string
	}{
		{"twilio", "accountSid"},
		{"messagebird", "originator"},
		{"textlocal", "sender"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			cfg := DefaultConfiguration(contracts.PresetLightweight)
			cfg.Auth.Phone = contracts.PhoneAuthConfig{Enabled: true, Provider: tc.provider}
			err := ValidateConfiguration(cfg)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Fields["auth.phone.fields."+tc.field] == "" || validation.Fields["auth.phone.secret"] == "" {
				t.Fatalf("expected required Phone fields and secret, got %v", err)
			}
		})
	}
}

func TestPhoneSecretUsesSecretInput(t *testing.T) {
	phone := contracts.PhoneAuthConfig{Enabled: true, Provider: "messagebird", SecretSet: true, Secret: contracts.SecretInput{Action: "retain"}, Fields: map[string]string{"originator": "Bee"}}
	payload, err := json.Marshal(phone)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "authToken") || strings.Contains(string(payload), "accessKey") {
		t.Fatalf("Phone JSON contains credential field: %s", payload)
	}
}

func TestAuthSignupAndEmailChangeTruthTables(t *testing.T) {
	signupCases := []struct {
		name      string
		disable   bool
		allow     bool
		phone     bool
		anonymous bool
		oauth     bool
		wantError bool
	}{
		{name: "enabled", disable: false, allow: true},
		{name: "globally disabled", disable: true, allow: false},
		{name: "mismatched disabled false", disable: false, allow: false, wantError: true},
		{name: "mismatched allowed true", disable: true, allow: true, wantError: true},
		{name: "phone path conflicts", disable: true, allow: false, phone: true, wantError: true},
		{name: "anonymous path conflicts", disable: true, allow: false, anonymous: true, wantError: true},
		{name: "oauth path conflicts", disable: true, allow: false, oauth: true, wantError: true},
	}
	for _, tc := range signupCases {
		t.Run("signup/"+tc.name, func(t *testing.T) {
			cfg := DefaultConfiguration(contracts.PresetLightweight)
			cfg.Auth.DisableSignup = tc.disable
			cfg.Auth.Email.AllowSignup = tc.allow
			cfg.Auth.Phone.Enabled = tc.phone
			cfg.Auth.AnonymousSignIn = tc.anonymous
			if tc.oauth {
				cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true}}
			}
			err := ValidateConfiguration(cfg)
			if tc.wantError {
				var validation *ValidationError
				if !errors.As(err, &validation) || validation.Fields["auth.disableSignup"] == "" {
					t.Fatalf("ValidateConfiguration() error = %v, want auth.disableSignup field error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateConfiguration() error = %v", err)
			}
		})
	}

	emailCases := []struct {
		name      string
		secure    bool
		double    bool
		wantError bool
	}{
		{name: "both disabled", secure: false, double: false},
		{name: "both enabled", secure: true, double: true},
		{name: "secure only", secure: true, double: false, wantError: true},
		{name: "double only", secure: false, double: true, wantError: true},
	}
	for _, tc := range emailCases {
		t.Run("email-change/"+tc.name, func(t *testing.T) {
			cfg := DefaultConfiguration(contracts.PresetLightweight)
			cfg.Auth.Email.SecureEmailChange = tc.secure
			cfg.Auth.Email.DoubleConfirmChanges = tc.double
			err := ValidateConfiguration(cfg)
			if tc.wantError {
				var validation *ValidationError
				if !errors.As(err, &validation) || validation.Fields["auth.email.secureEmailChange"] == "" {
					t.Fatalf("ValidateConfiguration() error = %v, want secureEmailChange field error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateConfiguration() error = %v", err)
			}
		})
	}
}

func TestAuthJWTExpiryMatchesPinnedRuntimeRange(t *testing.T) {
	for _, expiry := range []int{-1, 31536001} {
		cfg := DefaultConfiguration(contracts.PresetLightweight)
		cfg.Auth.JWTExpiry = expiry
		var validation *ValidationError
		if err := ValidateConfiguration(cfg); !errors.As(err, &validation) || validation.Fields["auth.jwtExpiry"] == "" {
			t.Fatalf("ValidateConfiguration(jwtExpiry=%d) = %v, want auth.jwtExpiry error", expiry, err)
		}
	}
}

func TestAuthSMTPEmailRejectsDisplayNameLikeFrontend(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "bee", Password: contracts.SecretInput{Action: "replace", Value: "secret"}, SenderEmail: "Bee <bee@example.com>", SenderName: "Bee"}
	var validation *ValidationError
	if err := ValidateConfiguration(cfg); !errors.As(err, &validation) || validation.Fields["auth.smtp.senderEmail"] == "" {
		t.Fatalf("ValidateConfiguration(display-name sender) = %v, want senderEmail error", err)
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

func TestValidateStoredConfigurationAcceptsConsumedSecretActions(t *testing.T) {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "mailer", PasswordSet: true, SenderEmail: "mailer@example.com", SenderName: "Mailer"}
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true, ClientID: "client", SecretSet: true}}
	if err := ValidateStoredConfiguration(cfg); err != nil {
		t.Fatalf("stored canonical validation = %v", err)
	}
	if err := ValidateConfiguration(cfg); err == nil {
		t.Fatal("command validation accepted an empty configured action")
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
		{"functions requires gateway", func(c *contracts.ProjectConfiguration) { c.Services.Functions = true; c.Services.Gateway = false }, "services.gateway"},
		{"caddy requires gateway", func(c *contracts.ProjectConfiguration) {
			c.Network.HTTPSMode = contracts.HTTPSModeCaddy
			c.Services.Gateway = false
		}, "services.gateway"},
		{"storage requires rest", func(c *contracts.ProjectConfiguration) { c.Services.Storage = true; c.Services.REST = false }, "services.storage"},
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
