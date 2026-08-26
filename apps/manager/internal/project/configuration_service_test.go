package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func admitProjectPatch(service *ConfigurationService, projectID string, patch contracts.ConfigurationPatch) (store.ConfigurationSnapshot, error) {
	cfg, err := service.PreparePatch(context.Background(), projectID, patch)
	if err != nil {
		return store.ConfigurationSnapshot{}, err
	}
	mutations, err := service.PrepareSecretMutations(context.Background(), projectID, &cfg)
	if err != nil {
		return store.ConfigurationSnapshot{}, err
	}
	now := time.Now().UTC()
	operationID := fmt.Sprintf("project-test-%d", now.UnixNano())
	op := contracts.Operation{ID: operationID, ProjectID: projectID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: now}
	snapshot, lease, err := service.store.AdmitConfiguration(context.Background(), store.ConfigurationAdmission{Operation: op, ProjectID: projectID, Owner: operationID, ExpectedRevision: patch.ExpectedRevision, Configuration: cfg, OperationKind: "UPDATE_CONFIG", Mutations: mutations, Now: now})
	if err == nil {
		err = service.store.ReleaseConfigurationLeaseOwned(context.Background(), projectID, operationID, lease.Fence)
	}
	return snapshot, err
}

func TestConfigurationServiceRejectsInvalidServiceClosure(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, err := managersecrets.NewCipher(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	project := contracts.Project{ID: "project-closure", Slug: "bee", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	services := cfg.Services
	services.Gateway = false
	services.REST = true
	service := NewConfigurationService(database, cipher, time.Now)
	_, err = admitProjectPatch(service, project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, Services: &services})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Fields["services.gateway"] == "" {
		t.Fatalf("expected field validation for invalid closure, got %v", err)
	}
}

func TestConfigurationServiceSecretPatches(t *testing.T) {
	cases := []struct {
		action string
		value  string
		want   string
		exists bool
	}{
		{"retain", "", "old-secret", true},
		{"replace", "new-secret", "new-secret", true},
		{"remove", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			cipher, err := managersecrets.NewCipher(bytes.Repeat([]byte{9}, 32))
			if err != nil {
				t.Fatal(err)
			}
			cfg := DefaultConfiguration(contracts.PresetLightweight)
			cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
			cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "OPENAI_API_KEY", ValueSet: true, Value: contracts.SecretInput{Action: "retain"}}}
			project := contracts.Project{ID: "project-1", Slug: "bee", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
			if err := database.CreateProject(context.Background(), project, cfg); err != nil {
				t.Fatal(err)
			}
			envelope, _ := cipher.Encrypt(project.ID, "functions.OPENAI_API_KEY", []byte("old-secret"))
			if err := database.PutSecret(context.Background(), project.ID, "functions.OPENAI_API_KEY", envelope); err != nil {
				t.Fatal(err)
			}
			cfg.Functions.Variables[0].Value = contracts.SecretInput{Action: tc.action, Value: tc.value}
			service := NewConfigurationService(database, cipher, func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) })
			got, err := admitProjectPatch(service, project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, Configuration: &cfg})
			if err != nil {
				t.Fatalf("Patch() error = %v", err)
			}
			if got.Revision != 2 || (tc.exists && got.Configuration.Functions.Variables[0].Value.Value != "") || (!tc.exists && len(got.Configuration.Functions.Variables) != 0) {
				t.Fatalf("Patch() snapshot leaked or has wrong revision: %#v", got)
			}
			stored, err := database.GetSecret(context.Background(), project.ID, "functions.OPENAI_API_KEY")
			if tc.exists {
				if err != nil {
					t.Fatalf("GetSecret() error = %v", err)
				}
				plain, err := cipher.Decrypt(project.ID, "functions.OPENAI_API_KEY", stored)
				if err != nil || string(plain) != tc.want {
					t.Fatalf("secret = %q, %v; want %q", plain, err, tc.want)
				}
			} else if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("GetSecret() error = %v, want not found", err)
			}
		})
	}
}

func TestConfigurationServiceRejectsEmptySecretReplacement(t *testing.T) {
	// The service must reject this before any configuration revision is written.
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, _ := managersecrets.NewCipher(bytes.Repeat([]byte{8}, 32))
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	project := contracts.Project{ID: "project-1", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0", Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "OPENAI_API_KEY", Value: contracts.SecretInput{Action: "replace"}}}
	service := NewConfigurationService(database, cipher, time.Now)
	if _, err := admitProjectPatch(service, project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, Configuration: &cfg}); err == nil {
		t.Fatal("Patch() accepted empty replacement")
	}
	got, err := database.GetConfiguration(context.Background(), project.ID)
	if err != nil || got.Revision != 1 {
		t.Fatalf("failed patch changed revision: %#v, %v", got, err)
	}
}

func TestConfigurationServiceRequiresExplicitActionForExistingSecret(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, _ := managersecrets.NewCipher(bytes.Repeat([]byte{7}, 32))
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	project := contracts.Project{ID: "project-1", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0", Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "OPENAI_API_KEY", ValueSet: true, Value: contracts.SecretInput{}}}
	service := NewConfigurationService(database, cipher, time.Now)
	if _, err := admitProjectPatch(service, project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, Configuration: &cfg}); err == nil {
		t.Fatal("Patch() accepted an implicit retain marker")
	}
	got, err := database.GetConfiguration(context.Background(), project.ID)
	if err != nil || got.Revision != 1 {
		t.Fatalf("failed patch changed revision: %#v, %v", got, err)
	}
}

func TestConfigurationServicePartialGeneralPatchPreservesConfiguredSecrets(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, _ := managersecrets.NewCipher(bytes.Repeat([]byte{6}, 32))
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "bee", PasswordSet: true, Password: contracts.SecretInput{Action: "retain"}, SenderEmail: "bee@example.com", SenderName: "Bee"}
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true, ClientID: "client", SecretSet: true, Secret: contracts.SecretInput{Action: "retain"}}}
	project := contracts.Project{ID: "project-1", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0", Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	for kind, value := range map[string]string{"smtp.password": "smtp-secret", "oauth.google.secret": "oauth-secret"} {
		envelope, err := cipher.Encrypt(project.ID, kind, []byte(value))
		if err != nil {
			t.Fatal(err)
		}
		if err := database.PutSecret(context.Background(), project.ID, kind, envelope); err != nil {
			t.Fatal(err)
		}
	}
	service := NewConfigurationService(database, cipher, time.Now)
	got, err := admitProjectPatch(service, project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, General: &contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}})
	if err != nil {
		t.Fatalf("General-only Patch() error = %v", err)
	}
	if got.Revision != 2 || got.Configuration.Auth.SMTP.Password.Action != "" || got.Configuration.Auth.OAuth["google"].Secret.Action != "" {
		t.Fatalf("partial patch returned non-redacted secret state: %#v", got.Configuration.Auth)
	}
}

func TestConfigurationServicePartialGeneralPatchAcceptsDefaultLocalAndDisabledPhone(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	project := contracts.Project{ID: "project-1", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0", Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	service := NewConfigurationService(database, nil, time.Now)
	if _, err := admitProjectPatch(service, project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, General: &contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}}); err != nil {
		t.Fatalf("default local General-only Patch() error = %v", err)
	}
}

func TestConfigurationServiceRejectsStaleRevision(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	project := contracts.Project{ID: "project-1", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0", Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	service := NewConfigurationService(database, nil, time.Now)
	if _, err := admitProjectPatch(service, project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, Configuration: &cfg}); err != nil {
		t.Fatal(err)
	}
	if _, err := admitProjectPatch(service, project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, Configuration: &cfg}); !errors.Is(err, ErrStaleConfiguration) {
		t.Fatalf("stale Save() error = %v, want ErrStaleConfiguration", err)
	}
}

func TestServiceCreationAtomicallyEncryptsAllConfigurationSecrets(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, _ := managersecrets.NewCipher(bytes.Repeat([]byte{4}, 32))
	cfg := DefaultConfiguration(contracts.PresetFull)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, Username: "bee", Password: contracts.SecretInput{Action: "replace", Value: "smtp-secret"}, SenderEmail: "bee@example.com", SenderName: "Bee"}
	cfg.Auth.Phone = contracts.PhoneAuthConfig{Enabled: true, Provider: "messagebird", Secret: contracts.SecretInput{Action: "replace", Value: "phone-secret"}, Fields: map[string]string{"originator": "Bee"}}
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: true, ClientID: "client", Secret: contracts.SecretInput{Action: "replace", Value: "oauth-secret"}}}
	cfg.Storage = contracts.StorageConfig{Backend: contracts.StorageBackendS3, Bucket: "bee", Region: "us-east-1", Endpoint: "https://s3.example.com", AccessKeyID: "access", SecretAccessKey: contracts.SecretInput{Action: "replace", Value: "storage-secret"}}
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "OPENAI_API_KEY", Value: contracts.SecretInput{Action: "replace", Value: "function-one"}}, {Name: "SECOND_SECRET", Value: contracts.SecretInput{Action: "replace", Value: "function-two"}}}
	draft := Draft{Name: "Bee", Slug: "bee", Configuration: cfg, Preset: contracts.PresetFull}
	service := NewServiceWithCipher(database, func() string { return "project-1" }, time.Now, cipher)
	if _, err := service.Create(context.Background(), draft); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if cfg.Auth.SMTP.PasswordSet || cfg.Auth.SMTP.Password.Action != "replace" || cfg.Auth.SMTP.Password.Value != "smtp-secret" || cfg.Functions.Variables[0].ValueSet || cfg.Functions.Variables[0].Value.Action != "replace" || cfg.Functions.Variables[0].Value.Value != "function-one" {
		t.Fatal("creation secret preparation mutated caller-owned aggregate")
	}
	for kind, want := range map[string]string{"smtp.password": "smtp-secret", "phone.secret": "phone-secret", "oauth.google.secret": "oauth-secret", "storage.secretAccessKey": "storage-secret", "functions.OPENAI_API_KEY": "function-one", "functions.SECOND_SECRET": "function-two"} {
		envelope, err := database.GetSecret(context.Background(), "project-1", kind)
		if err != nil {
			t.Fatalf("GetSecret(%s) error = %v", kind, err)
		}
		plain, err := cipher.Decrypt("project-1", kind, envelope)
		if err != nil || string(plain) != want {
			t.Fatalf("secret %s = %q, %v; want %q", kind, plain, err, want)
		}
	}
	snapshot, err := database.GetConfiguration(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Configuration.Auth.SMTP.Password.Value != "" || snapshot.Configuration.Auth.SMTP.Password.Action != "" || snapshot.Configuration.Auth.Phone.Secret.Value != "" || snapshot.Configuration.Auth.Phone.Secret.Action != "" || snapshot.Configuration.Auth.OAuth["google"].Secret.Value != "" || snapshot.Configuration.Auth.OAuth["google"].Secret.Action != "" || snapshot.Configuration.Storage.SecretAccessKey.Value != "" || snapshot.Configuration.Storage.SecretAccessKey.Action != "" || snapshot.Configuration.Functions.Variables[0].Value.Value != "" || snapshot.Configuration.Functions.Variables[0].Value.Action != "" {
		t.Fatal("configuration read returned plaintext secret")
	}
	if !snapshot.Configuration.Auth.SMTP.PasswordSet || !snapshot.Configuration.Auth.Phone.SecretSet || !snapshot.Configuration.Auth.OAuth["google"].SecretSet || !snapshot.Configuration.Storage.SecretAccessKeySet || !snapshot.Configuration.Functions.Variables[0].ValueSet || !snapshot.Configuration.Functions.Variables[1].ValueSet {
		t.Fatal("creation did not persist secret-set markers")
	}
}

func TestServiceCreationRejectsReplacementWithoutCipher(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "OPENAI_API_KEY", Value: contracts.SecretInput{Action: "replace", Value: "secret"}}}
	service := NewService(database, func() string { return "project-1" }, time.Now)
	if _, err := service.Create(context.Background(), Draft{Name: "Bee", Slug: "bee", Configuration: cfg}); err == nil {
		t.Fatal("Create() accepted replacement without cipher")
	}
	if _, err := database.GetProject(context.Background(), "project-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed creation left project row: %v", err)
	}
}

func TestServiceCreationRejectsUpdateOnlyRemoveMarker(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "OPENAI_API_KEY", ValueSet: true, Value: contracts.SecretInput{Action: "remove"}}}
	service := NewService(database, func() string { return "project-1" }, time.Now)
	if _, err := service.Create(context.Background(), Draft{Name: "Bee", Slug: "bee", Configuration: cfg}); err == nil {
		t.Fatal("Create() accepted update-only remove marker")
	}
	if _, err := database.GetProject(context.Background(), "project-1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("rejected creation left project row: %v", err)
	}
}

func TestServiceCreationIgnoresMaliciousSecretMarkers(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := DefaultConfiguration(contracts.PresetFull)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	cfg.Auth.SMTP.PasswordSet = true
	cfg.Auth.Phone = contracts.PhoneAuthConfig{Provider: "messagebird", SecretSet: true}
	cfg.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {SecretSet: true}}
	cfg.Storage = contracts.StorageConfig{Backend: contracts.StorageBackendS3, Bucket: "bee", Region: "us-east-1", Endpoint: "https://s3.example.com", AccessKeyID: "access", SecretAccessKeySet: true}
	cfg.Functions.Variables = []contracts.FunctionVariable{{Name: "OPENAI_API_KEY", ValueSet: true}}
	service := NewService(database, func() string { return "project-1" }, time.Now)
	if _, err := service.Create(context.Background(), Draft{Name: "Bee", Slug: "bee", Configuration: cfg}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	snapshot, err := database.GetConfiguration(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Configuration.Auth.SMTP.PasswordSet || snapshot.Configuration.Auth.Phone.SecretSet || snapshot.Configuration.Auth.OAuth["google"].SecretSet || snapshot.Configuration.Storage.SecretAccessKeySet || snapshot.Configuration.Functions.Variables[0].ValueSet {
		t.Fatal("creation trusted client-supplied secret markers")
	}
	var count int
	if err := database.DB().QueryRow(`SELECT COUNT(*) FROM project_secrets WHERE project_id = ?`, "project-1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("malicious marker creation persisted %d secrets", count)
	}
}

func TestPreparePatchBackfillsNewAuthDefaultsForLegacyConfiguration(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	// Simulate a configuration persisted before RateLimits and MFA existed.
	cfg.Auth.RateLimits = contracts.RateLimitConfig{}
	cfg.Auth.MFA = contracts.MFAConfig{}
	project := contracts.Project{ID: "project-legacy-auth", Slug: "bee", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	service := NewConfigurationService(database, nil, time.Now)
	got, err := service.PreparePatch(context.Background(), project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, General: &cfg.General})
	if err != nil {
		t.Fatalf("PreparePatch() error = %v", err)
	}
	if got.Auth.RateLimits.EmailSent != 30 || got.Auth.MFA.MaxEnrolledFactors != 10 || got.Auth.MFA.PhoneOTPLength != 6 || !got.Auth.MFA.TOTPEnrollEnabled || !got.Auth.MFA.TOTPVerifyEnabled {
		t.Fatalf("legacy Auth defaults were not backfilled: %#v %#v", got.Auth.RateLimits, got.Auth.MFA)
	}
}
