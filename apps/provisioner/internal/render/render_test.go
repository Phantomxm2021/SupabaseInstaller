package render

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"supabase-manager/internal/contracts"
)

func TestWriteRepresentativeRenderFiles(t *testing.T) {
	root := os.Getenv("RENDER_OUTPUT")
	if root == "" {
		root = t.TempDir()
	}
	t.Logf("representative runtime root: %s", root)
	cases := []struct {
		name     string
		services func(*contracts.ProjectConfiguration)
	}{
		{name: "lightweight", services: func(c *contracts.ProjectConfiguration) {}},
		{name: "standard", services: func(c *contracts.ProjectConfiguration) {
			c.Services.Realtime = true
			c.Services.Storage = true
			c.Services.Imgproxy = true
			c.Services.Functions = true
			c.Services.Supavisor = true
		}},
		{name: "full", services: func(c *contracts.ProjectConfiguration) {
			c.Services.Realtime = true
			c.Services.Storage = true
			c.Services.Imgproxy = true
			c.Services.Functions = true
			c.Services.Supavisor = true
			c.Services.Logs = true
			c.Services.Vector = true
		}},
	}
	for _, tc := range cases {
		cfg := testConfiguration()
		tc.services(&cfg)
		out, err := Project(Input{Slug: tc.name, APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{"logs.publicAccessToken": "public", "logs.privateAccessToken": "private"}})
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, tc.name)
		if err := os.MkdirAll(filepath.Join(dir, "volumes"), 0o700); err != nil {
			t.Fatal(err)
		}
		current := filepath.Join(dir, ".manager-runtime", "current")
		if err := os.MkdirAll(current, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string]string{"docker-compose.yml": out.Compose, ".env": out.Env, ".env.functions": out.FunctionsEnv} {
			if err := os.WriteFile(filepath.Join(current, name), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRepresentativeComposeConfig(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name      string
		configure func(*contracts.ProjectConfiguration)
		secrets   map[string]string
	}{
		{"lightweight", func(*contracts.ProjectConfiguration) {}, nil},
		{"standard", func(c *contracts.ProjectConfiguration) {
			c.Services.Realtime = true
			c.Services.Storage = true
			c.Services.Imgproxy = true
			c.Services.Functions = true
			c.Services.Supavisor = true
		}, nil},
		{"full", func(c *contracts.ProjectConfiguration) {
			c.Services.Realtime = true
			c.Services.Storage = true
			c.Services.Imgproxy = true
			c.Services.Functions = true
			c.Services.Supavisor = true
			c.Services.Logs = true
			c.Services.Vector = true
		}, map[string]string{"logs.publicAccessToken": "public", "logs.privateAccessToken": "private"}},
	}
	for _, tc := range cases {
		cfg := testConfiguration()
		tc.configure(&cfg)
		out, err := Project(Input{Slug: tc.name, APIPort: 18001, Configuration: cfg, RuntimeSecrets: tc.secrets})
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, tc.name)
		current := filepath.Join(dir, ".manager-runtime", "current")
		if err := os.MkdirAll(current, 0o700); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string]string{"docker-compose.yml": out.Compose, ".env": out.Env, ".env.functions": out.FunctionsEnv} {
			if err := os.WriteFile(filepath.Join(current, name), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		command := exec.Command("docker", "compose", "--file", filepath.Join(current, "docker-compose.yml"), "--env-file", filepath.Join(current, ".env"), "--project-directory", dir, "config", "--quiet")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s compose config: %v\n%s", tc.name, err, output)
		}
	}
}

const testCompose = `
name: supabase
services:
  db:
    image: supabase/postgres:17.6.1.136
  auth:
    image: supabase/gotrue:v2.177.0
    depends_on:
      db:
        condition: service_healthy
  rest:
    image: postgrest/postgrest:v13.0.4
  meta:
    image: supabase/postgres-meta:v0.91.0
  studio:
    image: supabase/studio:2026.04.27-sha-5f60601
    depends_on:
      meta:
        condition: service_healthy
      analytics:
        condition: service_healthy
  envoy:
    image: envoyproxy/envoy:v1.35.3
  realtime:
    image: supabase/realtime:v2.44.0
  storage:
    image: supabase/storage-api:v1.25.7
  analytics:
    image: supabase/logflare:1.22.4
`

func testConfiguration() contracts.ProjectConfiguration {
	return contracts.ProjectConfiguration{
		Revision:  1,
		General:   contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"},
		Services:  contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true},
		Auth:      contracts.AuthConfig{Enabled: true, JWTExpiry: 3600, Email: contracts.EmailAuthConfig{Enabled: true, AllowSignup: true}},
		Functions: contracts.FunctionsConfig{DefaultJWTVerification: true},
		Pooler:    contracts.PoolerConfig{SessionPort: 6544, TransactionPort: 6543, PoolSize: 20, MaxClientConnections: 100},
	}
}

func TestRenderCustomAuthAndSMTP(t *testing.T) {
	cfg := testConfiguration()
	cfg.Auth.Email.ConfirmEmail = true
	cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "mailer", SenderEmail: "noreply@example.com", SenderName: "Example"}
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true, ClientID: "google-client", SecretSet: true}}
	out, err := Project(Input{Slug: "bee", APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{"smtp.password": "smtp-secret", "oauth.google.secret": "oauth-secret"}, TemplateCompose: []byte(testCompose)})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"ENABLE_EMAIL_AUTOCONFIRM=false", "SMTP_HOST=smtp.example.com", "SMTP_PORT=587", "GOOGLE_ENABLED=true", "GOOGLE_CLIENT_ID=google-client", "GOOGLE_SECRET=oauth-secret"} {
		if !strings.Contains(out.Env, line) {
			t.Errorf("missing %q", line)
		}
	}
	if !strings.Contains(out.Compose, "GOTRUE_EXTERNAL_GOOGLE_ENABLED") {
		t.Fatal("Google mapping missing from Auth service")
	}
}

func TestRenderFunctionsSecretsStayInFunctionsEnv(t *testing.T) {
	cfg := testConfiguration()
	cfg.Services.Functions = true
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "STRIPE_KEY", ValueSet: true}}
	out, err := Project(Input{Slug: "bee", APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{"functions.STRIPE_KEY": "stripe-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.FunctionsEnv, "STRIPE_KEY=stripe-secret") || strings.Contains(out.Env, "stripe-secret") || strings.Contains(out.Compose, "stripe-secret") {
		t.Fatal("function secret leaked outside .env.functions")
	}
}

func TestRenderFullMergesOfficialLogsOverlay(t *testing.T) {
	cfg := testConfiguration()
	cfg.Services.Realtime = true
	cfg.Services.Storage = true
	cfg.Services.Imgproxy = true
	cfg.Services.Functions = true
	cfg.Services.Supavisor = true
	cfg.Services.Logs = true
	cfg.Services.Vector = true
	out, err := Project(Input{Slug: "full", APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{"logs.publicAccessToken": "public", "logs.privateAccessToken": "private"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []string{"analytics", "vector", "realtime", "storage", "functions", "supavisor"} {
		if !strings.Contains(out.Compose, "  "+service+":") {
			t.Errorf("full output missing %s", service)
		}
	}
}

func TestRenderFunctionsWiresPrivateEnvFile(t *testing.T) {
	cfg := testConfiguration()
	cfg.Services.Functions = true
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "STRIPE_KEY", ValueSet: true}}
	out, err := Project(Input{Slug: "functions", APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{"functions.STRIPE_KEY": "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Compose, "env_file:") || !strings.Contains(out.Compose, "./.manager-runtime/current/.env.functions") {
		t.Fatal("functions service is not wired to .env.functions")
	}
	if strings.Contains(out.Env, "STRIPE_KEY=secret") || !strings.Contains(out.FunctionsEnv, "STRIPE_KEY=secret") {
		t.Fatal("function secret placement is incorrect")
	}
}

func TestRenderRequiresMarkedRuntimeSecrets(t *testing.T) {
	cfg := testConfiguration()
	cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", PasswordSet: true}
	if _, err := Project(Input{Slug: "missing", APIPort: 18001, Configuration: cfg}); err == nil || !strings.Contains(err.Error(), "smtp.password") {
		t.Fatal("missing SMTP secret was accepted")
	}
	cfg.Auth.SMTP.PasswordSet = false
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true, SecretSet: true}}
	if _, err := Project(Input{Slug: "missing", APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{"smtp.password": "ok"}}); err == nil || !strings.Contains(err.Error(), "oauth.google.secret") {
		t.Fatal("missing OAuth secret was accepted")
	}
}

func TestRenderDotEnvQuotesInjectionCharacters(t *testing.T) {
	cfg := testConfiguration()
	cfg.General.SiteURL = `https://example.com/a # comment $HOME "x`
	out, err := Project(Input{Slug: "quoted", APIPort: 18001, Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(out.Env, "\n") {
		if strings.HasPrefix(line, "SITE_URL=") && !strings.Contains(line, `"`) {
			t.Fatalf("SITE_URL was not safely encoded: %q", line)
		}
	}
}

func TestRenderPhoneProviderRegistry(t *testing.T) {
	cases := []struct {
		provider string
		fields   map[string]string
		want     string
	}{
		{"twilio", map[string]string{"accountSid": "AC123", "messageServiceSid": "MG123"}, "GOTRUE_SMS_TWILIO_AUTH_TOKEN"},
		{"messagebird", map[string]string{"originator": "Bee"}, "GOTRUE_SMS_MESSAGEBIRD_ACCESS_KEY"},
		{"textlocal", map[string]string{"sender": "Bee"}, "GOTRUE_SMS_TEXTLOCAL_API_KEY"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			cfg := testConfiguration()
			cfg.Auth.Phone = contracts.PhoneAuthConfig{Enabled: true, Provider: tc.provider, SecretSet: true, Fields: tc.fields}
			out, err := Project(Input{Slug: "phone-" + tc.provider, APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{SecretPhone: "phone-secret"}})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.Compose, tc.want+": ${PHONE_SECRET}") {
				t.Fatalf("missing exact provider secret mapping %s", tc.want)
			}
		})
	}
}

func TestRenderPhoneProviderEnvironmentKeysPreserveSMSPrefix(t *testing.T) {
	cases := []struct {
		provider string
		fields   map[string]string
		env      []string
		compose  []string
		secret   string
	}{
		{
			provider: "twilio",
			fields: map[string]string{
				"accountSid":        "AC123",
				"messageServiceSid": "MG123",
				"verifySid":         "VS123",
			},
			env: []string{
				"SMS_TWILIO_ACCOUNT_SID=AC123",
				"SMS_TWILIO_MESSAGE_SERVICE_SID=MG123",
				"SMS_TWILIO_VERIFY_MESSAGE_SERVICE_SID=VS123",
			},
			compose: []string{
				"GOTRUE_SMS_TWILIO_ACCOUNT_SID: ${SMS_TWILIO_ACCOUNT_SID}",
				"GOTRUE_SMS_TWILIO_MESSAGE_SERVICE_SID: ${SMS_TWILIO_MESSAGE_SERVICE_SID}",
				"GOTRUE_SMS_TWILIO_VERIFY_MESSAGE_SERVICE_SID: ${SMS_TWILIO_VERIFY_MESSAGE_SERVICE_SID}",
				"GOTRUE_SMS_TWILIO_AUTH_TOKEN: ${PHONE_SECRET}",
				"GOTRUE_SMS_TWILIO_VERIFY_AUTH_TOKEN: ${PHONE_SECRET}",
			},
			secret: "PHONE_SECRET=phone-secret",
		},
		{
			provider: "messagebird",
			fields:   map[string]string{"originator": "Bee"},
			env:      []string{"SMS_MESSAGEBIRD_ORIGINATOR=Bee"},
			compose: []string{
				"GOTRUE_SMS_MESSAGEBIRD_ORIGINATOR: ${SMS_MESSAGEBIRD_ORIGINATOR}",
				"GOTRUE_SMS_MESSAGEBIRD_ACCESS_KEY: ${PHONE_SECRET}",
			},
			secret: "PHONE_SECRET=phone-secret",
		},
		{
			provider: "textlocal",
			fields:   map[string]string{"sender": "Bee"},
			env:      []string{"SMS_TEXTLOCAL_SENDER=Bee"},
			compose: []string{
				"GOTRUE_SMS_TEXTLOCAL_SENDER: ${SMS_TEXTLOCAL_SENDER}",
				"GOTRUE_SMS_TEXTLOCAL_API_KEY: ${PHONE_SECRET}",
			},
			secret: "PHONE_SECRET=phone-secret",
		},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			cfg := testConfiguration()
			cfg.Auth.Phone = contracts.PhoneAuthConfig{Enabled: true, Provider: tc.provider, SecretSet: true, Fields: tc.fields}
			out, err := Project(Input{Slug: "phone-" + tc.provider, APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{SecretPhone: "phone-secret"}})
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range tc.env {
				if !strings.Contains(out.Env, line) {
					t.Errorf(".env missing %q", line)
				}
			}
			if !strings.Contains(out.Env, tc.secret) {
				t.Errorf(".env missing secret source %q", tc.secret)
			}
			for _, line := range tc.compose {
				if !strings.Contains(out.Compose, line) {
					t.Errorf("Compose missing %q", line)
				}
			}
			if strings.Contains(out.Compose, "GOTRUE_"+strings.ToUpper(tc.provider)+"_") {
				t.Errorf("Compose dropped SMS prefix for %s", tc.provider)
			}
		})
	}
}

func TestRenderAuthSignupAndEmailChangeTruthTables(t *testing.T) {
	t.Run("signup", func(t *testing.T) {
		cases := []struct {
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
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := testConfiguration()
				cfg.Auth.DisableSignup = tc.disable
				cfg.Auth.Email.AllowSignup = tc.allow
				cfg.Auth.Phone.Enabled = tc.phone
				cfg.Auth.AnonymousSignIn = tc.anonymous
				if tc.oauth {
					cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true}}
				}
				out, err := Project(Input{Slug: "auth-signup-" + strings.ReplaceAll(tc.name, " ", "-"), APIPort: 18001, Configuration: cfg})
				if tc.wantError {
					if err == nil || !strings.Contains(err.Error(), "auth") {
						t.Fatalf("Project() error = %v, want auth field error", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				want := "DISABLE_SIGNUP=" + boolString(tc.disable)
				if !strings.Contains(out.Env, want) {
					t.Fatalf(".env missing exact global mapping %q", want)
				}
				if !strings.Contains(out.Compose, "GOTRUE_DISABLE_SIGNUP: ${DISABLE_SIGNUP}") {
					t.Fatal("Compose missing exact GOTRUE_DISABLE_SIGNUP mapping")
				}
			})
		}
	})

	t.Run("secure email change", func(t *testing.T) {
		cases := []struct {
			name         string
			secure       bool
			double       bool
			wantError    bool
			wantEnvValue string
		}{
			{name: "both disabled", secure: false, double: false, wantEnvValue: "false"},
			{name: "both enabled", secure: true, double: true, wantEnvValue: "true"},
			{name: "secure only", secure: true, double: false, wantError: true},
			{name: "double only", secure: false, double: true, wantError: true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := testConfiguration()
				cfg.Auth.Email.SecureEmailChange = tc.secure
				cfg.Auth.Email.DoubleConfirmChanges = tc.double
				out, err := Project(Input{Slug: "auth-email-" + strings.ReplaceAll(tc.name, " ", "-"), APIPort: 18001, Configuration: cfg})
				if tc.wantError {
					if err == nil || !strings.Contains(err.Error(), "secureEmailChange") {
						t.Fatalf("Project() error = %v, want secureEmailChange field error", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out.Env, "SECURE_EMAIL_CHANGE_ENABLED="+tc.wantEnvValue) {
					t.Fatalf(".env missing pinned capability value %q", tc.wantEnvValue)
				}
				if !strings.Contains(out.Compose, "GOTRUE_MAILER_SECURE_EMAIL_CHANGE_ENABLED: ${SECURE_EMAIL_CHANGE_ENABLED}") {
					t.Fatal("Compose missing official secure email change mapping")
				}
			})
		}
	})
}

func TestRenderRejectsUnicodeControlInjection(t *testing.T) {
	cfg := testConfiguration()
	cfg.General.SiteURL = "https://example.com/\u0085"
	if _, err := Project(Input{Slug: "control", APIPort: 18001, Configuration: cfg}); err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatal("C1 control character was accepted")
	}
}

func TestRenderRequiresGeneratedRealtimeCredential(t *testing.T) {
	cfg := testConfiguration()
	cfg.Services.Realtime = true
	secrets := contracts.ProjectSecrets{DatabasePassword: "database", JWTSecret: "jwt", SecretKeyBase: "key"}
	if _, err := Project(Input{Slug: "missing-generated", APIPort: 18001, Configuration: cfg, Secrets: secrets}); err == nil || !strings.Contains(err.Error(), "realtimeDbEncryptionKey") {
		t.Fatal("missing generated realtime credential was accepted")
	}
}

func TestRenderGoldenFixtures(t *testing.T) {
	cases := []struct {
		name, fixture string
		configure     func(*contracts.ProjectConfiguration)
		secrets       map[string]string
	}{
		{"lightweight", "lightweight.golden.yml", func(*contracts.ProjectConfiguration) {}, nil},
		{"standard", "standard.golden.yml", func(c *contracts.ProjectConfiguration) {
			c.Services.Realtime = true
			c.Services.Storage = true
			c.Services.Imgproxy = true
			c.Services.Functions = true
			c.Services.Supavisor = true
		}, nil},
		{"full", "full.golden.yml", func(c *contracts.ProjectConfiguration) {
			c.Services.Realtime = true
			c.Services.Storage = true
			c.Services.Imgproxy = true
			c.Services.Functions = true
			c.Services.Supavisor = true
			c.Services.Logs = true
			c.Services.Vector = true
		}, map[string]string{"logs.publicAccessToken": "public", "logs.privateAccessToken": "private"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfiguration()
			tc.configure(&cfg)
			out, err := Project(Input{Slug: tc.name, APIPort: 18001, Configuration: cfg, RuntimeSecrets: tc.secrets})
			if err != nil {
				t.Fatal(err)
			}
			golden, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			var expected, actual any
			if err := yaml.Unmarshal(golden, &expected); err != nil {
				t.Fatal(err)
			}
			if err := yaml.Unmarshal([]byte(out.Compose), &actual); err != nil {
				t.Fatal(err)
			}
			expectedCanonical, err := yaml.Marshal(expected)
			if err != nil {
				t.Fatal(err)
			}
			actualCanonical, err := yaml.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(expectedCanonical, actualCanonical) {
				t.Fatalf("canonical Compose differs from %s", tc.fixture)
			}
		})
	}
	cfg := testConfiguration()
	cfg.Auth.Email.ConfirmEmail = true
	cfg.Auth.RedirectURLs = []string{"https://example.com/callback"}
	cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "mailer", SenderEmail: "noreply@example.com", SenderName: "Example"}
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true, ClientID: "google-client", SecretSet: true}}
	out, err := Project(Input{Slug: "custom", APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{"smtp.password": "smtp-secret", "oauth.google.secret": "oauth-secret"}, TemplateCompose: []byte(testCompose)})
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile(filepath.Join("testdata", "custom-auth.env.golden"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(golden), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(out.Env, line) {
			t.Errorf("custom auth golden line missing: %q", line)
		}
	}
}

func TestRenderAllOAuthProvidersAndSpecialFields(t *testing.T) {
	cfg := testConfiguration()
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{}
	runtime := map[string]string{}
	for _, name := range contracts.OAuthProviderNames {
		cfg.Auth.OAuth[name] = contracts.OAuthProviderConfig{Enabled: true, ClientID: name + "-client", SecretSet: true}
		runtime["oauth."+name+".secret"] = name + "-secret"
	}
	cfg.Auth.OAuth["azure"] = contracts.OAuthProviderConfig{Enabled: true, ClientID: "azure-client", SecretSet: true, Fields: map[string]string{"tenantUrl": "https://login.microsoftonline.com/tenant"}}
	cfg.Auth.OAuth["github"] = contracts.OAuthProviderConfig{Enabled: true, ClientID: "github-client", SecretSet: true, Fields: map[string]string{"enterpriseUrl": "https://github.example.com"}}
	cfg.Auth.OAuth["gitlab"] = contracts.OAuthProviderConfig{Enabled: true, ClientID: "gitlab-client", SecretSet: true, Fields: map[string]string{"selfHostedUrl": "https://gitlab.example.com"}}
	cfg.Auth.OAuth["keycloak"] = contracts.OAuthProviderConfig{Enabled: true, ClientID: "keycloak-client", SecretSet: true, Fields: map[string]string{"realmUrl": "https://keycloak.example.com/realms/example"}}
	out, err := Project(Input{Slug: "oauth", APIPort: 18001, Configuration: cfg, RuntimeSecrets: runtime})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range contracts.OAuthProviderNames {
		provider := providerDefinitionFor(name)
		if !strings.Contains(out.Env, provider.Name+"_ENABLED=true") || !strings.Contains(out.Compose, "GOTRUE_EXTERNAL_"+provider.Name+"_ENABLED") {
			t.Errorf("provider %s missing", name)
		}
	}
	for _, key := range []string{"GOTRUE_EXTERNAL_AZURE_URL", "GOTRUE_EXTERNAL_GITHUB_URL", "GOTRUE_EXTERNAL_GITLAB_URL", "GOTRUE_EXTERNAL_KEYCLOAK_URL"} {
		if !strings.Contains(out.Compose, key) {
			t.Errorf("special provider key missing: %s", key)
		}
	}
}

func TestRenderStorageModesAndR2Endpoint(t *testing.T) {
	for _, backend := range []contracts.StorageBackend{contracts.StorageBackendLocal, contracts.StorageBackendS3, contracts.StorageBackendAWSS3, contracts.StorageBackendR2} {
		t.Run(string(backend), func(t *testing.T) {
			cfg := testConfiguration()
			cfg.Storage.Backend = backend
			cfg.Storage.Bucket = "bucket"
			cfg.Storage.Region = "us-east-1"
			cfg.Storage.AccessKeyID = "access"
			cfg.Storage.Endpoint = "https://s3.example.com"
			cfg.Storage.SecretAccessKeySet = backend != contracts.StorageBackendLocal
			if backend == contracts.StorageBackendR2 {
				cfg.Storage.Endpoint = ""
				cfg.Storage.AccountID = "account"
			}
			secrets := map[string]string{}
			if cfg.Storage.SecretAccessKeySet {
				secrets[SecretStorageKey] = "secret"
			}
			out, err := Project(Input{Slug: "storage", APIPort: 18001, Configuration: cfg, RuntimeSecrets: secrets})
			if err != nil {
				t.Fatal(err)
			}
			want := "STORAGE_BACKEND=file"
			if backend != contracts.StorageBackendLocal {
				want = "STORAGE_BACKEND=s3"
			}
			if !strings.Contains(out.Env, want) {
				t.Errorf("missing backend mapping %q", want)
			}
			if backend == contracts.StorageBackendR2 && !strings.Contains(out.Env, "GLOBAL_S3_ENDPOINT=https://account.r2.cloudflarestorage.com") {
				t.Error("missing R2 endpoint")
			}
		})
	}
}

func TestRenderS3ProtocolCanBeEnabledIndependently(t *testing.T) {
	cfg := testConfiguration()
	cfg.Services.Storage = true
	cfg.Storage.S3CompatibleAPI = true
	cfg.Storage.Backend = contracts.StorageBackendLocal
	out, err := Project(Input{Slug: "s3-protocol", APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{SecretS3Access: "access", SecretS3Secret: "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Env, "S3_PROTOCOL_ACCESS_KEY_ID=access") || !strings.Contains(out.Compose, "S3_PROTOCOL_ACCESS_KEY_ID") {
		t.Fatal("S3 protocol configuration missing")
	}
}

func TestRenderDatabaseVersionsAndGatewayChoices(t *testing.T) {
	cfg := testConfiguration()
	cfg.Database.Version = "15"
	cfg.Network.Gateway = contracts.GatewayKong
	out, err := Project(Input{Slug: "pg15-kong", APIPort: 18002, Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Compose, "supabase/postgres:15.8.1.085") || !strings.Contains(out.Compose, "kong/kong:3.9.3") || !strings.Contains(out.Compose, "127.0.0.1:18002:8000") {
		t.Fatal("Postgres 15/Kong selection incorrect")
	}
	cfg.Database.Version = "17"
	cfg.Network.Gateway = contracts.GatewayEnvoy
	cfg.Network.HTTPSMode = contracts.HTTPSModeCaddy
	out, err = Project(Input{Slug: "pg17-caddy", APIPort: 18003, Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Compose, "caddy:2.9.1") || !strings.Contains(out.Compose, "  caddy:") {
		t.Fatal("Caddy selection incorrect")
	}
}

func TestRenderRealtimeDatabaseAndPoolerTuning(t *testing.T) {
	cfg := testConfiguration()
	cfg.Services.Realtime = true
	cfg.Services.Supavisor = true
	cfg.Database.MaxConnections = 321
	cfg.Database.SharedBuffers = "256MB"
	cfg.Realtime.MaxConnections = 88
	cfg.Realtime.DatabasePoolSize = 12
	cfg.Realtime.LogLevel = contracts.LogLevelDebug
	cfg.Pooler.SessionPort = 6544
	cfg.Pooler.TransactionPort = 6543
	cfg.Pooler.PoolSize = 20
	cfg.Pooler.MaxClientConnections = 100
	out, err := Project(Input{Slug: "tuning", APIPort: 18001, Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"max_connections=321", "shared_buffers=256MB", "REALTIME_MAX_CONNECTIONS", "REALTIME_DB_POOL_SIZE", "REALTIME_LOG_LEVEL", "127.0.0.1:6544:5432", "127.0.0.1:6543:6543"} {
		if !strings.Contains(out.Compose, want) && !strings.Contains(out.Env, want) {
			t.Errorf("tuning mapping missing %s", want)
		}
	}
}

func TestRenderServiceSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*contracts.ProjectConfiguration)
		want      []string
		no        []string
	}{
		{name: "standard", configure: func(c *contracts.ProjectConfiguration) {}, want: []string{"db", "envoy", "auth", "rest", "meta", "studio"}, no: []string{"realtime", "storage", "functions", "supavisor"}},
		{name: "full", configure: func(c *contracts.ProjectConfiguration) {
			c.Services.Realtime = true
			c.Services.Storage = true
			c.Services.Imgproxy = true
			c.Services.Functions = true
			c.Services.Supavisor = true
			c.Services.Logs = true
			c.Services.Vector = true
		}, want: []string{"realtime", "storage", "imgproxy", "functions", "supavisor"}, no: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfiguration()
			tc.configure(&cfg)
			var compose []byte
			if tc.name != "full" {
				compose = []byte(testCompose)
			}
			secrets := map[string]string{"logs.publicAccessToken": "public", "logs.privateAccessToken": "private"}
			out, err := Project(Input{Slug: "bee", APIPort: 18001, Configuration: cfg, RuntimeSecrets: secrets, TemplateCompose: compose})
			if err != nil {
				t.Fatal(err)
			}
			for _, service := range tc.want {
				if !strings.Contains(out.Compose, "  "+service+":") {
					t.Errorf("missing service %s", service)
				}
			}
			for _, service := range tc.no {
				if strings.Contains(out.Compose, "  "+service+":") {
					t.Errorf("unexpected service %s", service)
				}
			}
		})
	}
}
