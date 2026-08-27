package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if provisioner.reconcile.Secrets.DatabasePassword == "" || provisioner.reconcile.Secrets.JWTSecret == "" {
		t.Fatalf("reconcile request missing generated secrets: %#v", provisioner.reconcile)
	}
	stored, err := orchestrator.store.GetSecret(context.Background(), project.ID, "database-password")
	if err != nil || bytes.Contains(stored.Ciphertext, []byte(provisioner.reconcile.Secrets.DatabasePassword)) {
		t.Fatalf("stored database secret is not encrypted: %v", err)
	}
}

func TestInstallRetryReusesPersistedDatabaseSecrets(t *testing.T) {
	orchestrator, provisioner, project := newTestOrchestrator(t)
	generator := &changingGenerator{}
	orchestrator.generator = generator
	sequence := 0
	orchestrator.operations = operation.NewService(orchestrator.store, func() string {
		sequence++
		return fmt.Sprintf("operation-%d", sequence)
	}, time.Now)

	if _, err := orchestrator.Install(context.Background(), project); err != nil {
		t.Fatalf("initial Install() error = %v", err)
	}
	initialPassword := provisioner.reconcile.Secrets.DatabasePassword

	if _, err := orchestrator.Install(context.Background(), project); err != nil {
		t.Fatalf("retry Install() error = %v", err)
	}
	if generator.calls != 1 {
		t.Fatalf("secret generator calls = %d, want 1", generator.calls)
	}
	if got := provisioner.reconcile.Secrets.DatabasePassword; got != initialPassword {
		t.Fatalf("retry database password = %q, want persisted %q", got, initialPassword)
	}
}

func TestInstallHydratesConfiguredSMTPSecretIntoRevisionOneReconcile(t *testing.T) {
	orchestrator, provisioner, project := newTestOrchestrator(t)
	snapshot, err := orchestrator.store.GetDesiredConfiguration(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := snapshot.Configuration
	cfg.Auth.SMTP = contracts.SMTPConfig{Enabled: true, Host: "smtp.example.com", Port: 587, PasswordSet: true}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.store.DB().Exec(`UPDATE project_configs SET config_json=? WHERE project_id=? AND section='aggregate' AND revision=1`, string(raw), project.ID); err != nil {
		t.Fatal(err)
	}
	envelope, err := orchestrator.cipher.Encrypt(project.ID, "smtp.password", []byte("configured-smtp-secret"))
	if err != nil {
		t.Fatal(err)
	}
	if err := orchestrator.store.PutSecret(context.Background(), project.ID, "smtp.password", envelope); err != nil {
		t.Fatal(err)
	}
	result, err := orchestrator.Install(context.Background(), project)
	if err != nil || result.Status != operation.Succeeded {
		t.Fatalf("Install() = %#v, %v", result, err)
	}
	if got := provisioner.reconcile.RuntimeSecrets["smtp.password"]; got != "configured-smtp-secret" {
		t.Fatalf("revision-1 reconcile SMTP secret = %q, want configured value", got)
	}
}

func TestInstallAllocatesAndPersistsAllServerOwnedPorts(t *testing.T) {
	orchestrator, provisioner, project := newTestOrchestrator(t)
	project.Services.DirectDB = true
	project.Services.Supavisor = true
	snapshot, err := orchestrator.store.GetConfiguration(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Configuration.Services = project.Services
	snapshot.Configuration.Services = project.Services
	configOperation := contracts.Operation{ID: "install-config-op", ProjectID: project.ID, Type: contracts.OperationUpdateConfig, Status: operation.Queued, CreatedAt: time.Now()}
	saved, lease, err := orchestrator.store.AdmitConfiguration(context.Background(), store.ConfigurationAdmission{Operation: configOperation, ProjectID: project.ID, Owner: configOperation.ID, ExpectedRevision: snapshot.Revision, Configuration: snapshot.Configuration, OperationKind: "UPDATE_CONFIG", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := orchestrator.store.MarkConfigurationGoodOwned(context.Background(), project.ID, saved.Revision, configOperation.ID, lease.Fence, "COMMITTED", time.Now()); err != nil {
		t.Fatal(err)
	}
	result, err := orchestrator.Install(context.Background(), project)
	if err != nil || result.Status != operation.Succeeded {
		t.Fatalf("Install() = %#v, %v", result, err)
	}
	cfg := provisioner.reconcile.Configuration
	ports := []int{cfg.Network.APIPort, cfg.Network.StudioPort, cfg.Network.DirectDatabasePort, cfg.Pooler.TransactionPort, cfg.Pooler.SessionPort}
	seen := map[int]bool{}
	for _, port := range ports[:5] {
		if port < 18001 || port > 18010 || seen[port] {
			t.Fatalf("allocated ports = %#v, expected unique ports in allocator range", ports)
		}
		seen[port] = true
	}
	if cfg.Network.PoolerPort != 0 || cfg.Database.DirectPortNumber != cfg.Network.DirectDatabasePort || !cfg.Database.DirectPort {
		t.Fatalf("allocated configuration is not synchronized: %#v", cfg)
	}
	stored, err := orchestrator.store.GetConfiguration(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Configuration.Network.DirectDatabasePort != cfg.Network.DirectDatabasePort || stored.Configuration.Network.APIPort != cfg.Network.APIPort {
		t.Fatalf("stored allocated ports = %#v, reconcile = %#v", stored.Configuration.Network, cfg.Network)
	}
}

func TestInstallFailsWhenDesiredAggregateCannotBeRead(t *testing.T) {
	orchestrator, provisioner, project := newTestOrchestrator(t)
	if _, err := orchestrator.store.DB().Exec(`DELETE FROM project_configs WHERE project_id = ?`, project.ID); err != nil {
		t.Fatal(err)
	}
	result, err := orchestrator.Install(context.Background(), project)
	if err == nil {
		t.Fatal("Install() succeeded without a desired aggregate")
	}
	if result.Status != operation.RolledBack {
		t.Fatalf("operation status = %s, want ROLLED_BACK", result.Status)
	}
	if provisioner.reconcile.Configuration.General.Domain != "" {
		t.Fatalf("reconcile used a projection fallback: %#v", provisioner.reconcile.Configuration)
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
	reconcile      contracts.ReconcileProjectRequest
	failStart      error
	runtimeRemoved bool
	dataRemoved    bool
}

func (fake *fakeProvisioner) Reconcile(_ context.Context, request contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	fake.reconcile = request
	return contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.NextRevision, EnabledServices: []string{"db", "api-gw", "auth", "rest", "meta", "studio"}}, nil
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
	configuration := contracts.ProjectConfiguration{General: contracts.GeneralConfig{Domain: project.Domain, SiteURL: project.SiteURL, SupabaseVersion: project.SupabaseVersion}, Services: project.Services, Auth: contracts.AuthConfig{Enabled: true}}
	if err := database.CreateProject(context.Background(), project, configuration); err != nil {
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

type changingGenerator struct{ calls int }

func (generator *changingGenerator) Generate() (contracts.ProjectSecrets, error) {
	generator.calls++
	return contracts.ProjectSecrets{
		DatabasePassword: fmt.Sprintf("database-secret-%d", generator.calls),
		JWTSecret:        "jwt-secret-value-which-is-long-enough",
		AnonKey:          "anon-key", ServiceRoleKey: "service-role-key",
		DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base",
		VaultEncryptionKey: "vault-key",
	}, nil
}
