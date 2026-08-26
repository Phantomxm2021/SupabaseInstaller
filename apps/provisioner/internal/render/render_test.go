package render

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"supabase-manager/internal/contracts"
)

func TestWriteRepresentativeRenderFiles(t *testing.T) {
	root := os.Getenv("RENDER_OUTPUT")
	if root == "" {
		t.Skip("RENDER_OUTPUT not set")
	}
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
		out, err := Project(Input{Slug: tc.name, APIPort: 18001, Configuration: cfg})
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, tc.name)
		if err := os.MkdirAll(filepath.Join(dir, "volumes"), 0o700); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string]string{"docker-compose.yml": out.Compose, ".env": out.Env, ".env.functions": out.FunctionsEnv} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
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
		General:   contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "0.8.0"},
		Services:  contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true},
		Auth:      contracts.AuthConfig{Enabled: true, JWTExpiry: 3600, Email: contracts.EmailAuthConfig{Enabled: true, AllowSignup: true}},
		Functions: contracts.FunctionsConfig{DefaultJWTVerification: true},
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
	out, err := Project(Input{Slug: "bee", APIPort: 18001, Configuration: cfg, RuntimeSecrets: map[string]string{"functions.STRIPE_KEY": "stripe-secret"}, TemplateCompose: []byte(testCompose)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.FunctionsEnv, "STRIPE_KEY=stripe-secret") || strings.Contains(out.Env, "stripe-secret") || strings.Contains(out.Compose, "stripe-secret") {
		t.Fatal("function secret leaked outside .env.functions")
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
			out, err := Project(Input{Slug: "bee", APIPort: 18001, Configuration: cfg, TemplateCompose: compose})
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

func TestLightweightRenderUsesPinnedImagesAndUniqueComposeName(t *testing.T) {
	output, err := Lightweight(Input{
		ProjectID: "project-1", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com",
		APIPort: 18001, TemplateCompose: []byte(testCompose),
	})
	if err != nil {
		t.Fatalf("Lightweight() error = %v", err)
	}
	if !strings.Contains(output.Env, "SUPABASE_PUBLIC_URL=https://bee.example.com") {
		t.Fatalf(".env missing public URL: %s", output.Env)
	}
	if strings.Contains(output.Compose, ":latest") {
		t.Fatal("Compose contains an unpinned latest image")
	}
	for _, disabled := range []string{"realtime:", "storage:", "analytics:"} {
		if strings.Contains(output.Compose, disabled) {
			t.Fatalf("Lightweight Compose contains disabled service %s", disabled)
		}
	}
	if strings.Contains(output.Compose, "analytics") {
		t.Fatal("Lightweight Compose retained dependency on disabled analytics")
	}
	if output.ComposeProjectName != "supabase-manager-bee" {
		t.Fatalf("ComposeProjectName = %q", output.ComposeProjectName)
	}
}

func TestLightweightRejectsLatestImage(t *testing.T) {
	compose := strings.Replace(testCompose, "supabase/gotrue:v2.177.0", "supabase/gotrue:latest", 1)
	_, err := Lightweight(Input{Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", APIPort: 18001, TemplateCompose: []byte(compose)})
	if err == nil || !strings.Contains(err.Error(), "latest") {
		t.Fatalf("Lightweight() error = %v, want latest rejection", err)
	}
}

func TestEmbeddedOfficialTemplateRendersOnlyLightweightServices(t *testing.T) {
	output, err := Lightweight(Input{Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", APIPort: 18001})
	if err != nil {
		t.Fatalf("Lightweight() with embedded template error = %v", err)
	}
	var document struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(output.Compose), &document); err != nil {
		t.Fatalf("decode rendered Compose: %v", err)
	}
	for _, required := range []string{"api-gw", "auth", "db", "meta", "rest", "studio"} {
		if _, ok := document.Services[required]; !ok {
			t.Fatalf("embedded Lightweight Compose missing %s", required)
		}
	}
	for _, disabled := range []string{"realtime", "storage", "imgproxy", "functions", "supavisor"} {
		if _, ok := document.Services[disabled]; ok {
			t.Fatalf("embedded Lightweight Compose contains %s", disabled)
		}
	}
	if strings.Contains(output.Compose, "container_name:") {
		t.Fatal("rendered Compose retains global container names that break project isolation")
	}
	envKeys := map[string]bool{}
	for _, line := range strings.Split(output.Env, "\n") {
		if key, _, ok := strings.Cut(line, "="); ok && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			envKeys[strings.TrimSpace(key)] = true
		}
	}
	requiredVariable := regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)
	for _, match := range requiredVariable.FindAllStringSubmatch(output.Compose, -1) {
		if !envKeys[match[1]] {
			t.Fatalf("rendered .env does not define required Compose variable %s", match[1])
		}
	}
}
