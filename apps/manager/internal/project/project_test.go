package project

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestNormalizeSlugProducesDNSCompatibleIdentifier(t *testing.T) {
	if got := NormalizeSlug("  My Bee 2!  "); got != "my-bee-2" {
		t.Fatalf("NormalizeSlug() = %q, want my-bee-2", got)
	}
}

func TestNormalizeProjectAddressDerivesDomainFromSlugAndBaseSiteURL(t *testing.T) {
	general := contracts.GeneralConfig{
		// Domain is client-controlled input today; it must never define the
		// public runtime address.
		Domain:  "untrusted.example.net",
		SiteURL: "https://BeeGame.Studio/",
	}

	if err := NormalizeProjectAddress("bgs", &general); err != nil {
		t.Fatal(err)
	}
	if general.Domain != "bgs.beegame.studio" {
		t.Fatalf("Domain = %q, want bgs.beegame.studio", general.Domain)
	}
	if general.SiteURL != "https://beegame.studio" {
		t.Fatalf("SiteURL = %q, want canonical base URL", general.SiteURL)
	}
}

func TestNormalizeProjectAddressRejectsNonHostnameBaseURL(t *testing.T) {
	general := contracts.GeneralConfig{SiteURL: "https://beegame.studio/path"}

	err := NormalizeProjectAddress("bee", &general)
	if err == nil || !strings.Contains(err.Error(), "base hostname") {
		t.Fatalf("NormalizeProjectAddress() error = %v, want base hostname validation", err)
	}
}

func TestConfigurationPresetsApplyDependencyClosure(t *testing.T) {
	cases := []struct {
		preset contracts.Preset
		check  func(contracts.ProjectConfiguration) bool
	}{
		{contracts.PresetLightweight, func(c contracts.ProjectConfiguration) bool {
			return !c.Services.Storage && !c.Services.Logs && !c.Services.Vector
		}},
		{contracts.PresetStandard, func(c contracts.ProjectConfiguration) bool {
			return c.Services.Realtime && c.Services.Storage && c.Services.Functions && c.Services.Supavisor && c.Pooler.PoolSize > 0 && c.Pooler.MaxClientConnections > 0
		}},
		{contracts.PresetFull, func(c contracts.ProjectConfiguration) bool {
			return c.Services.Imgproxy && c.Services.Storage && c.Services.Logs && c.Services.Vector && c.Pooler.PoolSize > 0 && c.Pooler.MaxClientConnections > 0
		}},
	}
	for _, tc := range cases {
		got := ApplyConfigurationPreset(tc.preset)
		if !tc.check(got) {
			t.Fatalf("ApplyConfigurationPreset(%s) = %#v", tc.preset, got)
		}
	}
}

func TestProjectJSONDoesNotExposeConfigurationSecrets(t *testing.T) {
	draft := validDraft()
	draft.Configuration = DefaultConfiguration(contracts.PresetLightweight)
	draft.Configuration.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	draft.Configuration.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "bee", PasswordSet: false, Password: contracts.SecretInput{Action: "replace", Value: "smtp-plaintext"}, SenderEmail: "bee@example.com", SenderName: "Bee"}
	draft.Configuration.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true, ClientID: "client", Secret: contracts.SecretInput{Action: "replace", Value: "oauth-plaintext"}}}
	draft.Configuration.Storage = contracts.StorageConfig{Backend: contracts.StorageBackendS3, Bucket: "bucket", Region: "us-east-1", Endpoint: "https://s3.example.com", AccessKeyID: "access-key", SecretAccessKey: contracts.SecretInput{Action: "replace", Value: "s3-plaintext"}}
	draft.Configuration.Functions.Variables = []contracts.FunctionVariable{{Name: "BEE_SECRET", Value: contracts.SecretInput{Action: "replace", Value: "function-plaintext"}}}
	draft.Configuration.Auth.Phone = contracts.PhoneAuthConfig{Enabled: true, Provider: "messagebird", Secret: contracts.SecretInput{Action: "replace", Value: "phone-plaintext"}, Fields: map[string]string{"originator": "Bee"}}
	project := contracts.Project{Domain: draft.Configuration.General.Domain, SiteURL: draft.Configuration.General.SiteURL, SupabaseVersion: draft.Configuration.General.SupabaseVersion, Services: draft.Configuration.Services}
	payload, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"smtp-plaintext", "oauth-plaintext", "s3-plaintext", "function-plaintext", "phone-plaintext", "access-key"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("Project JSON leaked %q: %s", secret, payload)
		}
	}
}

func TestCreateEncryptsConfiguredStudioPassword(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, err := managersecrets.NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	service := NewServiceWithCipher(database, func() string { return "studio-project" }, time.Now, cipher)
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.General.Domain = "studio.example.com"
	cfg.General.SiteURL = "https://studio.example.com"
	cfg.General.StudioUsername = "admin"
	cfg.General.StudioPassword = contracts.SecretInput{Action: "replace", Value: "studio-password"}
	created, err := service.Create(context.Background(), Draft{Name: "Studio", Slug: "studio", Preset: contracts.PresetLightweight, Configuration: cfg})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := database.GetSecret(context.Background(), created.ID, "dashboard-password")
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := cipher.Decrypt(created.ID, "dashboard-password", envelope)
	if err != nil || string(plaintext) != "studio-password" {
		t.Fatalf("decrypted Studio password = %q, err = %v", plaintext, err)
	}
	snapshot, err := database.GetConfiguration(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Configuration.General.StudioPassword.Value != "" || snapshot.Configuration.General.StudioPassword.Action != "" || !snapshot.Configuration.General.StudioPasswordSet {
		t.Fatalf("stored configuration exposed Studio password: %#v", snapshot.Configuration.General.StudioPassword)
	}
}

func TestNormalizeSlugCollapsesSeparators(t *testing.T) {
	if got := NormalizeSlug("Bee___API---Prod"); got != "bee-api-prod" {
		t.Fatalf("NormalizeSlug() = %q, want bee-api-prod", got)
	}
}

func TestLightweightPresetMatchesPRD(t *testing.T) {
	got := ApplyPreset(PresetLightweight)
	if !got.Database || !got.Gateway || !got.Auth || !got.REST || !got.Studio || !got.PostgresMeta {
		t.Fatalf("Lightweight core services = %#v, want all enabled", got)
	}
	if got.Realtime || got.Storage || got.Imgproxy || got.Functions || got.Supavisor || got.Logs || got.Vector || got.DirectDB {
		t.Fatalf("Lightweight optional services = %#v, want all disabled", got)
	}
}

func TestValidateDraftRejectsStudioWithoutPostgresMeta(t *testing.T) {
	draft := validDraft()
	draft.Configuration.Services.Studio = true
	draft.Configuration.Services.PostgresMeta = false

	err := ValidateDraft(draft)
	if err == nil || !strings.Contains(err.Error(), "postgres-meta") {
		t.Fatalf("ValidateDraft() error = %v, want postgres-meta dependency", err)
	}
}

func TestValidateDraftRejectsRelativeSiteURL(t *testing.T) {
	draft := validDraft()
	draft.Configuration.General.SiteURL = "localhost:3000"

	err := ValidateDraft(draft)
	if err == nil || !strings.Contains(err.Error(), "siteUrl") {
		t.Fatalf("ValidateDraft() error = %v, want siteUrl validation", err)
	}
}

func TestValidateDraftRejectsLatestRuntimeVersion(t *testing.T) {
	draft := validDraft()
	draft.Configuration.General.SupabaseVersion = "latest"

	err := ValidateDraft(draft)
	if err == nil || !strings.Contains(err.Error(), "supabaseVersion") {
		t.Fatalf("ValidateDraft() error = %v, want pinned version validation", err)
	}
}

func validDraft() Draft {
	return Draft{
		Name:   "Bee",
		Slug:   "bee",
		Preset: PresetLightweight,
		Configuration: func() contracts.ProjectConfiguration {
			cfg := DefaultConfiguration(PresetLightweight)
			cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
			return cfg
		}(),
	}
}
