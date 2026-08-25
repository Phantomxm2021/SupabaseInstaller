package project

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

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
			got, err := service.Patch(context.Background(), project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, Configuration: &cfg})
			if err != nil {
				t.Fatalf("Patch() error = %v", err)
			}
			if got.Revision != 2 || got.Configuration.Functions.Variables[0].Value.Value != "" {
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
	if _, err := service.Patch(context.Background(), project.ID, contracts.ConfigurationPatch{ExpectedRevision: 1, Configuration: &cfg}); err == nil {
		t.Fatal("Patch() accepted empty replacement")
	}
	got, err := database.GetConfiguration(context.Background(), project.ID)
	if err != nil || got.Revision != 1 {
		t.Fatalf("failed patch changed revision: %#v, %v", got, err)
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
	if _, err := service.Save(context.Background(), project.ID, 1, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(context.Background(), project.ID, 1, cfg); !errors.Is(err, ErrStaleConfiguration) {
		t.Fatalf("stale Save() error = %v, want ErrStaleConfiguration", err)
	}
}
