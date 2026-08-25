package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/internal/contracts"
)

func TestStoreCreatesAndReadsProject(t *testing.T) {
	s := openTestStore(t)
	want := projectFixture()
	if err := s.CreateProject(context.Background(), want); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	got, err := s.GetProject(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.ID != want.ID || got.Slug != want.Slug || !got.Services.Database || got.Health != contracts.HealthUnknown {
		t.Fatalf("GetProject() = %#v, want persisted project", got)
	}
}

func TestStorePersistsOnlyEncryptedSecretEnvelope(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	cipher, _ := secrets.NewCipher(bytes.Repeat([]byte{3}, 32))
	envelope, _ := cipher.Encrypt(project.ID, "postgres-password", []byte("plain-postgres-password"))
	if err := s.PutSecret(context.Background(), project.ID, "postgres-password", envelope); err != nil {
		t.Fatalf("PutSecret() error = %v", err)
	}
	stored, err := s.GetSecret(context.Background(), project.ID, "postgres-password")
	if err != nil {
		t.Fatalf("GetSecret() error = %v", err)
	}
	if bytes.Contains(stored.Ciphertext, []byte("plain-postgres-password")) {
		t.Fatal("stored ciphertext contains plaintext")
	}
	got, err := cipher.Decrypt(project.ID, "postgres-password", stored)
	if err != nil || string(got) != "plain-postgres-password" {
		t.Fatalf("decrypted stored secret = %q, %v", got, err)
	}
}

func TestConfigurationRevisionIsOptimisticAndRedacted(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	cfg := configurationFixture()
	got, err := s.GetConfiguration(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("GetConfiguration() error = %v", err)
	}
	if got.Revision != 1 {
		t.Fatalf("initial revision = %d, want 1", got.Revision)
	}
	cfg.Auth.SMTP.Password = contracts.SecretInput{Action: "replace", Value: "smtp-plaintext"}
	cfg.Auth.SMTP.PasswordSet = true
	saved, err := s.SaveConfiguration(context.Background(), project.ID, 1, cfg, time.Now())
	if err != nil {
		t.Fatalf("SaveConfiguration() error = %v", err)
	}
	if saved.Revision != 2 {
		t.Fatalf("saved revision = %d, want 2", saved.Revision)
	}
	if _, err := s.SaveConfiguration(context.Background(), project.ID, 1, cfg, time.Now()); !errors.Is(err, ErrStaleConfiguration) {
		t.Fatalf("stale SaveConfiguration() error = %v, want ErrStaleConfiguration", err)
	}
	var raw string
	if err := s.DB().QueryRow(`SELECT config_json FROM project_configs WHERE project_id = ? AND section = 'aggregate' AND revision = 2`, project.ID).Scan(&raw); err != nil {
		t.Fatalf("read config_json: %v", err)
	}
	if bytes.Contains([]byte(raw), []byte("smtp-plaintext")) {
		t.Fatalf("config_json contains plaintext secret: %s", raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("decode config_json: %v", err)
	}
}

func TestMigration002AddsLastGoodRevisionAndInitialSnapshot(t *testing.T) {
	s := openTestStore(t)
	var migrationCount int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version IN (1, 2)`).Scan(&migrationCount); err != nil {
		t.Fatalf("read schema migrations: %v", err)
	}
	if migrationCount != 2 {
		t.Fatalf("migration count = %d, want 2", migrationCount)
	}
	var lastGood int64
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if err := s.DB().QueryRow(`SELECT last_good_revision FROM projects WHERE id = ?`, project.ID).Scan(&lastGood); err != nil {
		t.Fatalf("last_good_revision missing: %v", err)
	}
	if lastGood != 1 {
		t.Fatalf("last_good_revision = %d, want 1", lastGood)
	}
}

func TestMigration002UpgradesExistingV1Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manager.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := migrationFiles.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(string(initial)); err != nil {
		t.Fatalf("apply legacy migration: %v", err)
	}
	project := projectFixture()
	servicesJSON, _ := json.Marshal(project.Services)
	if _, err := legacy.Exec(`INSERT INTO projects(id, name, slug, domain, site_url, status, health, supabase_version, preset, services_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, project.ID, project.Name, project.Slug, project.Domain, project.SiteURL, project.Status, project.Health, project.SupabaseVersion, project.Preset, servicesJSON, formatTime(project.CreatedAt), formatTime(project.UpdatedAt)); err != nil {
		t.Fatalf("insert legacy project: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatalf("Open() legacy database: %v", err)
	}
	defer upgraded.Close()
	snapshot, err := upgraded.GetConfiguration(context.Background(), project.ID)
	if err != nil {
		t.Fatalf("GetConfiguration() upgraded database: %v", err)
	}
	if snapshot.Revision != 1 || snapshot.Configuration.Services != project.Services {
		t.Fatalf("upgraded snapshot = %#v", snapshot)
	}
}

func TestCreateProjectWithSecretFailureRollsBackAllRows(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	err := s.CreateProjectWithSecrets(context.Background(), project, configurationFixture(), []SecretMutation{{Kind: "", Envelope: secrets.Envelope{Version: 1}}})
	if err == nil {
		t.Fatal("CreateProjectWithSecrets() accepted invalid mutation")
	}
	if _, err := s.GetProject(context.Background(), project.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed creation left project row: %v", err)
	}
	var count int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM project_configs WHERE project_id = ?`, project.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed creation left %d config rows", count)
	}
}

func TestMigrationNamesSortNumericallyAndRejectDuplicates(t *testing.T) {
	migrations, err := parseMigrationNames([]string{"10_later.sql", "2_middle.sql", "001_first.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if migrations[0].version != 1 || migrations[1].version != 2 || migrations[2].version != 10 {
		t.Fatalf("migration order = %#v", migrations)
	}
	if _, err := parseMigrationNames([]string{"02_a.sql", "2_b.sql"}); err == nil {
		t.Fatal("parseMigrationNames() accepted duplicate numeric versions")
	}
}

func TestGetConfigurationRedactsLegacyNestedPlaintext(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	cfg := configurationFixture()
	cfg.Auth.SMTP.Password = contracts.SecretInput{Value: "smtp-plaintext"}
	cfg.Auth.Phone.Secret = contracts.SecretInput{Value: "phone-plaintext"}
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Secret: contracts.SecretInput{Value: "oauth-plaintext"}}}
	cfg.Storage.SecretAccessKey = contracts.SecretInput{Value: "storage-plaintext"}
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "OPENAI_API_KEY", Value: contracts.SecretInput{Value: "function-plaintext"}}}
	if err := s.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cfg)
	if _, err := s.DB().Exec(`UPDATE project_configs SET config_json = ? WHERE project_id = ? AND section = 'aggregate' AND revision = 1`, raw, project.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetConfiguration(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{got.Configuration.Auth.SMTP.Password.Value, got.Configuration.Auth.Phone.Secret.Value, got.Configuration.Auth.OAuth["google"].Secret.Value, got.Configuration.Storage.SecretAccessKey.Value, got.Configuration.Functions.Variables[0].Value.Value} {
		if value != "" {
			t.Fatalf("legacy plaintext crossed read boundary: %q", value)
		}
	}
}

func TestMarkConfigurationGoodRequiresCurrentRevisionAndNeverRegresses(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveConfiguration(context.Background(), project.ID, 1, configurationFixture(), time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, revision := range []int64{0, 1, 3} {
		if err := s.MarkConfigurationGood(context.Background(), project.ID, revision); !errors.Is(err, ErrStaleConfiguration) {
			t.Fatalf("MarkConfigurationGood(%d) = %v, want stale error", revision, err)
		}
	}
	if err := s.MarkConfigurationGood(context.Background(), project.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkConfigurationGood(context.Background(), project.ID, 1); !errors.Is(err, ErrStaleConfiguration) {
		t.Fatalf("regressing MarkConfigurationGood() = %v", err)
	}
	var lastGood int64
	if err := s.DB().QueryRow(`SELECT last_good_revision FROM projects WHERE id = ?`, project.ID).Scan(&lastGood); err != nil {
		t.Fatal(err)
	}
	if lastGood != 2 {
		t.Fatalf("last_good_revision = %d, want 2", lastGood)
	}
}

func TestRedactionDoesNotMutateInputAggregate(t *testing.T) {
	cfg := configurationFixture()
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Secret: contracts.SecretInput{Action: "replace", Value: "oauth-plaintext"}, Fields: map[string]string{"tenant": "keep"}}}
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "OPENAI_API_KEY", Value: contracts.SecretInput{Action: "replace", Value: "function-plaintext"}}}
	redacted := redactConfiguration(cfg)
	if redacted.Auth.OAuth["google"].Secret.Value != "" || redacted.Functions.Variables[0].Value.Value != "" {
		t.Fatal("redaction did not clear secret values")
	}
	if cfg.Auth.OAuth["google"].Secret.Value != "oauth-plaintext" || cfg.Functions.Variables[0].Value.Value != "function-plaintext" {
		t.Fatal("redaction mutated input aggregate")
	}
	redacted.Auth.OAuth["google"].Fields["tenant"] = "changed"
	if cfg.Auth.OAuth["google"].Fields["tenant"] != "keep" {
		t.Fatal("redaction shared nested OAuth map")
	}
}

func TestConfigurationSnapshotsAreImmutable(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	first := configurationFixture()
	first.General.Domain = "first.example.com"
	if _, err := s.SaveConfiguration(context.Background(), project.ID, 1, first, time.Now()); err != nil {
		t.Fatal(err)
	}
	second := first
	second.General.Domain = "second.example.com"
	if _, err := s.SaveConfiguration(context.Background(), project.ID, 2, second, time.Now()); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := s.DB().QueryRow(`SELECT config_json FROM project_configs WHERE project_id = ? AND section = 'aggregate' AND revision = 2`, project.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(raw), []byte("first.example.com")) || bytes.Contains([]byte(raw), []byte("second.example.com")) {
		t.Fatalf("immutable revision changed: %s", raw)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func projectFixture() contracts.Project {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	return contracts.Project{
		ID:              "project-1",
		Name:            "Bee",
		Slug:            "bee",
		Domain:          "bee.example.com",
		SiteURL:         "https://example.com",
		Status:          contracts.ProjectStatusDraft,
		Health:          contracts.HealthUnknown,
		SupabaseVersion: "self-hosted/v0.8.0",
		Preset:          contracts.PresetLightweight,
		Services:        contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func configurationFixture() contracts.ProjectConfiguration {
	return contracts.ProjectConfiguration{
		General:  contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"},
		Services: contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true},
		Auth:     contracts.AuthConfig{SMTP: contracts.SMTPConfig{Port: 587}},
	}
}
