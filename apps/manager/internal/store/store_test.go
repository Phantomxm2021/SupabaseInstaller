package store

import (
	"bytes"
	"context"
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
