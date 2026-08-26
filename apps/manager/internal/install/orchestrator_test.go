package install

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/ports"
	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestInstallFailureRollsBackRuntimeAndPreservesData(t *testing.T) {
	orchestrator, provisioner, project := newTestOrchestrator(t)
	provisioner.failStart = errors.New("auth unhealthy")
	result, err := orchestrator.Install(context.Background(), project)
	if err == nil {
		t.Fatal("Install() succeeded, want start failure")
	}
	if result.Status != operation.RolledBack {
		t.Fatalf("operation status = %s, want ROLLED_BACK", result.Status)
	}
	if !provisioner.runtimeRemoved || provisioner.dataRemoved {
		t.Fatalf("rollback runtimeRemoved=%v dataRemoved=%v", provisioner.runtimeRemoved, provisioner.dataRemoved)
	}
}

func TestInstallPersistsEncryptedUniqueSecretsBeforePrepare(t *testing.T) {
	orchestrator, provisioner, project := newTestOrchestrator(t)
	result, err := orchestrator.Install(context.Background(), project)
	if err != nil || result.Status != operation.Succeeded {
		t.Fatalf("Install() = %#v, %v", result, err)
	}
	if provisioner.prepare.Secrets.DatabasePassword == "" || provisioner.prepare.Secrets.JWTSecret == "" {
		t.Fatalf("prepare request missing generated secrets: %#v", provisioner.prepare)
	}
	stored, err := orchestrator.store.GetSecret(context.Background(), project.ID, "database-password")
	if err != nil || bytes.Contains(stored.Ciphertext, []byte(provisioner.prepare.Secrets.DatabasePassword)) {
		t.Fatalf("stored database secret is not encrypted: %v", err)
	}
}

func TestHydrateConfiguredSecretsSkipsDisabledAuthConsumers(t *testing.T) {
	orchestrator, _, project := newTestOrchestrator(t)
	envelope, err := orchestrator.cipher.Encrypt(project.ID, "smtp.password", []byte("auth-secret-sentinel"))
	if err != nil {
		t.Fatal(err)
	}
	if err := orchestrator.store.PutSecret(context.Background(), project.ID, "smtp.password", envelope); err != nil {
		t.Fatal(err)
	}
	cfg := contracts.ProjectConfiguration{Services: contracts.Services{Auth: false}, Auth: contracts.AuthConfig{SMTP: contracts.SMTPConfig{Enabled: true, PasswordSet: true}}}
	runtime, err := orchestrator.hydrateConfiguredSecrets(context.Background(), project.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := runtime["smtp.password"]; ok {
		t.Fatalf("disabled auth secret was hydrated: %#v", runtime)
	}
}

type fakeProvisioner struct {
	prepare        contracts.PrepareProjectRequest
	failStart      error
	runtimeRemoved bool
	dataRemoved    bool
}

func (fake *fakeProvisioner) Prepare(_ context.Context, request contracts.PrepareProjectRequest) (contracts.PrepareProjectResponse, error) {
	fake.prepare = request
	return contracts.PrepareProjectResponse{ProjectID: request.ProjectID, Slug: request.Slug, Revision: request.NextRevision}, nil
}

func (fake *fakeProvisioner) Lifecycle(_ context.Context, request contracts.LifecycleRequest) error {
	switch request.Action {
	case contracts.LifecycleStart:
		return fake.failStart
	case contracts.LifecycleDeleteRuntime:
		fake.runtimeRemoved = true
	case contracts.LifecycleDeleteData:
		fake.dataRemoved = true
	}
	return nil
}

func (fake *fakeProvisioner) Inspect(_ context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{ProjectID: request.ProjectID, Health: contracts.HealthHealthy}, nil
}

func newTestOrchestrator(t *testing.T) (*Orchestrator, *fakeProvisioner, contracts.Project) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project := contracts.Project{ID: "project-1", Name: "Bee", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", Status: contracts.ProjectStatusDraft, Health: contracts.HealthUnknown, SupabaseVersion: "self-hosted/v0.8.0", Preset: contracts.PresetLightweight, Services: contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	operationService := operation.NewService(database, func() string { return "operation-1" }, time.Now)
	allocator := ports.NewAllocator(database, 18001, 18010, availableProbe{})
	cipher, _ := managersecrets.NewCipher(bytes.Repeat([]byte{5}, 32))
	provisioner := &fakeProvisioner{}
	orchestrator := NewOrchestrator(database, operationService, allocator, cipher, provisioner, deterministicGenerator{}, time.Now)
	return orchestrator, provisioner, project
}

type availableProbe struct{}

func (availableProbe) Available(int) bool { return true }

type deterministicGenerator struct{}

func (deterministicGenerator) Generate() (contracts.ProjectSecrets, error) {
	return contracts.ProjectSecrets{DatabasePassword: "database-secret-value", JWTSecret: "jwt-secret-value-which-is-long-enough", AnonKey: "anon-key", ServiceRoleKey: "service-role-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"}, nil
}
