package configuration

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	projectservice "supabase-manager/apps/manager/internal/project"
	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestResumeAfterCommittedPublicationOnlyCompletesOperation(t *testing.T) {
	orchestrator, database, operations, project, snapshot, lease, op := newRecoveryFixture(t, "UPDATE_CONFIG")
	if err := database.MarkConfigurationGoodOwned(context.Background(), project.ID, snapshot.Revision, op.ID, lease.Fence, "COMMITTED", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := operations.Start(context.Background(), op.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.ReleaseConfigurationLeaseOwned(context.Background(), project.ID, op.ID, lease.Fence); err != nil {
		t.Fatal(err)
	}
	if err := orchestrator.Resume(context.Background(), func(context.Context, string) (contracts.Project, error) { return project, nil }); err != nil {
		t.Fatal(err)
	}
	got := waitRecoveryOperation(t, operations, op.ID)
	if got.Status != operation.Succeeded {
		t.Fatalf("resumed committed operation = %s, want SUCCEEDED", got.Status)
	}
	current, err := database.GetConfiguration(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.LastGoodRevision != snapshot.Revision {
		t.Fatalf("last-good revision = %d, want %d", current.LastGoodRevision, snapshot.Revision)
	}
}

func TestResumeMissingProjectUsesServerTerminology(t *testing.T) {
	orchestrator, _, operations, _, _, _, op := newRecoveryFixture(t, "UPDATE_CONFIG")
	if err := orchestrator.Resume(context.Background(), func(context.Context, string) (contracts.Project, error) { return contracts.Project{}, errors.New("missing") }); err != nil {
		t.Fatal(err)
	}
	got := waitRecoveryOperation(t, operations, op.ID)
	if got.Status != operation.Failed || !strings.Contains(got.ErrorMessage, "Server unavailable during operation resume") {
		t.Fatalf("resumed missing operation = %s/%q, want failed server-unavailable message", got.Status, got.ErrorMessage)
	}
}

func TestQueuePatchIgnoresClientRevision(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "direct.example.com", SiteURL: "https://direct.example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	proj := contracts.Project{ID: "direct-project", Name: "Direct", Slug: "direct", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: now, UpdatedAt: now}
	if err := database.CreateProject(context.Background(), proj, cfg); err != nil {
		t.Fatal(err)
	}
	ops := operation.NewService(database, func() string { return "direct-op" }, time.Now)
	orchestrator := NewOrchestrator(database, ops)
	updated := cfg.General
	updated.SiteURL = "https://changed.example.com"
	queued, snapshot, err := orchestrator.QueuePatch(context.Background(), proj.ID, contracts.ConfigurationPatch{ExpectedRevision: 0, General: &updated})
	if err != nil {
		t.Fatalf("QueuePatch() rejected client revision: %v", err)
	}
	if queued.ID == "" || snapshot.Configuration.General.SiteURL != updated.SiteURL {
		t.Fatalf("queued=%#v snapshot=%#v", queued, snapshot)
	}
}

func TestQueuePatchReusesDurableActiveOperation(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "active.example.com", SiteURL: "https://active.example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	proj := contracts.Project{ID: "active-project", Name: "Active", Slug: "active", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: now, UpdatedAt: now}
	if err := database.CreateProject(context.Background(), proj, cfg); err != nil {
		t.Fatal(err)
	}
	active := contracts.Operation{ID: "active-op", ProjectID: proj.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationRunning, CreatedAt: now}
	if err := database.CreateOperation(context.Background(), active, "OPERATION_STARTED", []byte(`{"status":"RUNNING"}`)); err != nil {
		t.Fatal(err)
	}
	ops := operation.NewService(database, func() string { return "new-op" }, time.Now)
	orchestrator := NewOrchestrator(database, ops)
	updated := cfg.General
	updated.SiteURL = "https://ignored.example.com"
	got, snapshot, err := orchestrator.QueuePatch(context.Background(), proj.ID, contracts.ConfigurationPatch{General: &updated})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != active.ID || snapshot.Configuration.General.SiteURL != cfg.General.SiteURL {
		t.Fatalf("reused operation=%q snapshot URL=%q; want %q and canonical URL %q", got.ID, snapshot.Configuration.General.SiteURL, active.ID, cfg.General.SiteURL)
	}
}

func TestQueuePatchClosesAbandonedOperationBeforeAdmittingNewValue(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "abandoned.example.com", SiteURL: "https://abandoned.example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	proj := contracts.Project{ID: "abandoned-project", Name: "Abandoned", Slug: "abandoned", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, CreatedAt: now, UpdatedAt: now}
	if err := database.CreateProject(context.Background(), proj, cfg); err != nil {
		t.Fatal(err)
	}
	active := contracts.Operation{ID: "abandoned-op", ProjectID: proj.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationRunning, CreatedAt: now.Add(-2 * time.Hour)}
	if err := database.CreateOperation(context.Background(), active, "OPERATION_STARTED", []byte(`{"status":"RUNNING"}`)); err != nil {
		t.Fatal(err)
	}
	ops := operation.NewService(database, func() string { return "replacement-op" }, func() time.Time { return now })
	orchestrator := NewOrchestrator(database, ops, func() time.Time { return now })
	updated := cfg.General
	updated.SiteURL = "https://replacement.example.com"
	got, _, err := orchestrator.QueuePatch(context.Background(), proj.ID, contracts.ConfigurationPatch{General: &updated})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == active.ID {
		t.Fatalf("abandoned operation was reused: %q", got.ID)
	}
	closed, err := database.GetOperation(context.Background(), active.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != operation.Failed || !strings.Contains(closed.ErrorMessage, "one-hour deadline") {
		t.Fatalf("abandoned operation = %s/%q, want FAILED with deadline diagnostic", closed.Status, closed.ErrorMessage)
	}
}

func TestResumeAfterRestoredPublicationOnlyCompletesRollback(t *testing.T) {
	orchestrator, database, operations, project, snapshot, lease, op := newRecoveryFixture(t, "UPDATE_CONFIG")
	if err := database.RestoreConfigurationStateOwned(context.Background(), project.ID, snapshot.Revision, op.ID, lease.Fence, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := operations.Start(context.Background(), op.ID); err != nil {
		t.Fatal(err)
	}
	if err := orchestrator.Resume(context.Background(), func(context.Context, string) (contracts.Project, error) { return project, nil }); err != nil {
		t.Fatal(err)
	}
	got := waitRecoveryOperation(t, operations, op.ID)
	if got.Status != operation.Succeeded {
		t.Fatalf("resumed restored operation = %s, want SUCCEEDED", got.Status)
	}
	current, err := database.GetConfiguration(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != 1 || current.LastGoodRevision != 1 {
		t.Fatalf("reconciled configuration = revision %d/last-good %d, want 1/1", current.Revision, current.LastGoodRevision)
	}
}

func TestRotationResumeAfterCommittedPublicationDoesNotReplayRuntime(t *testing.T) {
	orchestrator, database, operations, project, snapshot, lease, op := newRecoveryFixture(t, "ROTATE_DATABASE_PASSWORD")
	if err := database.MarkConfigurationGoodOwned(context.Background(), project.ID, snapshot.Revision, op.ID, lease.Fence, "COMMITTED", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := operations.Start(context.Background(), op.ID); err != nil {
		t.Fatal(err)
	}
	if err := database.ReleaseConfigurationLeaseOwned(context.Background(), project.ID, op.ID, lease.Fence); err != nil {
		t.Fatal(err)
	}
	if err := orchestrator.Resume(context.Background(), func(context.Context, string) (contracts.Project, error) { return project, nil }); err != nil {
		t.Fatal(err)
	}
	got := waitRecoveryOperation(t, operations, op.ID)
	if got.Status != operation.Succeeded {
		t.Fatalf("resumed committed rotation = %s, want SUCCEEDED", got.Status)
	}
}

func newRecoveryFixture(t *testing.T, kind string) (*Orchestrator, *store.Store, *operation.Service, contracts.Project, store.ConfigurationSnapshot, store.ConfigurationLease, contracts.Operation) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	now := time.Now().UTC()
	cfg := projectservice.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "resume.example.com", SiteURL: "https://resume.example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	project := contracts.Project{ID: "resume-project", Name: "Resume", Slug: "resume", Domain: cfg.General.Domain, SiteURL: cfg.General.SiteURL, SupabaseVersion: cfg.General.SupabaseVersion, Services: cfg.Services, Preset: contracts.PresetLightweight, Status: contracts.ProjectStatusDraft, Health: contracts.HealthUnknown, CreatedAt: now, UpdatedAt: now}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	operations := operation.NewService(database, func() string { return "resume-op" }, time.Now)
	op := contracts.Operation{ID: "resume-op", ProjectID: project.ID, Type: operation.TypeUpdateConfig, Status: operation.Queued, CreatedAt: now}
	candidate := cfg
	candidate.General.SiteURL = "https://candidate.resume.example.com"
	var operationSecrets map[string]managersecrets.Envelope
	cipher, err := managersecrets.NewCipher(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if kind == "ROTATE_DATABASE_PASSWORD" {
		envelope, encryptErr := cipher.Encrypt(project.ID, "operation.database-password", []byte("new-password-for-recovery"))
		if encryptErr != nil {
			t.Fatal(encryptErr)
		}
		operationSecrets = map[string]managersecrets.Envelope{"database-password": envelope}
	}
	snapshot, lease, err := database.AdmitConfiguration(context.Background(), store.ConfigurationAdmission{Operation: op, ProjectID: project.ID, Owner: op.ID, ExpectedRevision: 1, Configuration: candidate, OperationKind: kind, OperationSecrets: operationSecrets, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	orchestrator := NewOrchestrator(database, operations, cipher, noopRecoveryProvisioner{})
	return orchestrator, database, operations, project, snapshot, lease, op
}

func waitRecoveryOperation(t *testing.T, operations *operation.Service, id string) operation.Operation {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := operations.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == operation.Succeeded || got.Status == operation.RolledBack || got.Status == operation.Failed {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, _ := operations.Get(context.Background(), id)
	t.Fatalf("operation remained %s", got.Status)
	return got
}

type noopRecoveryProvisioner struct{}

func (noopRecoveryProvisioner) Reconcile(_ context.Context, request contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, EnabledServices: enabledServices(request.Configuration)}, nil
}

func TestEnabledServicesUsesAuthoritativeSupavisorName(t *testing.T) {
	cfg := contracts.ProjectConfiguration{Services: contracts.Services{Database: true, Gateway: true, Auth: true, Supavisor: true, Logs: true, Vector: true}}
	got := enabledServices(cfg)
	for _, want := range []string{"db", "api-gw", "auth", "supavisor", "analytics", "vector"} {
		found := false
		for _, item := range got {
			if item == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing service %q in %v", want, got)
		}
	}
	for _, item := range got {
		if item == "pooler" {
			t.Fatal("legacy pooler service leaked into authoritative projection")
		}
	}
}

func TestSameServicesAcceptsConcreteGatewayProjection(t *testing.T) {
	if !sameServices([]string{"db", "envoy"}, []string{"db", "api-gw"}) {
		t.Fatal("gateway implementation should normalize to api-gw")
	}
}

func TestHydrateUsesDedicatedStudioPasswordOverRuntimeDashboardPassword(t *testing.T) {
	orchestrator, database, _, project, snapshot, _, _ := newRecoveryFixture(t, "UPDATE_CONFIG")
	cfg := snapshot.Configuration
	cfg.General.StudioPasswordSet = true

	for kind, value := range map[string]string{
		"dashboard-password": "runtime-dashboard-password",
		"studio.password":    "operator-studio-password",
	} {
		envelope, err := orchestrator.cipher.Encrypt(project.ID, kind, []byte(value))
		if err != nil {
			t.Fatal(err)
		}
		if err := database.PutSecret(context.Background(), project.ID, kind, envelope); err != nil {
			t.Fatal(err)
		}
	}

	secrets, _, err := orchestrator.hydrate(context.Background(), project.ID, cfg)
	if err != nil {
		t.Fatalf("hydrate() error = %v", err)
	}
	if secrets.DashboardPassword != "operator-studio-password" {
		t.Fatalf("DashboardPassword = %q, want dedicated Studio password", secrets.DashboardPassword)
	}
}

func TestSameServicesIgnoresRendererHelperServices(t *testing.T) {
	expected := []string{"db", "api-gw", "auth", "rest", "meta", "studio", "realtime", "storage", "imgproxy", "functions", "supavisor"}
	actual := append(append([]string(nil), expected...), "auth-templates", "deno-cache", "db-config")
	if !sameServices(actual, expected) {
		t.Fatalf("renderer helper services must not make runtime verification fail: actual=%v expected=%v", actual, expected)
	}
}

func TestRuntimeVerificationUsesServerTerminologyForIDMismatch(t *testing.T) {
	err := runtimeVerificationError(contracts.ReconcileProjectResponse{OperationID: "op-1", ProjectID: "unexpected", Revision: 1}, "op-1", "server-1", 1, contracts.ProjectConfiguration{})
	if err == nil || !strings.Contains(err.Error(), "server ID received") {
		t.Fatalf("runtimeVerificationError() = %v, want server terminology", err)
	}
}

func TestEnabledServicesExcludesRendererHelperServices(t *testing.T) {
	cfg := contracts.ProjectConfiguration{Services: contracts.Services{Database: true, Gateway: true, Auth: true, Functions: true, Supavisor: true, Logs: true}}
	for _, service := range enabledServices(cfg) {
		if service == "auth-templates" || service == "deno-cache" || service == "db-config" || service == "logflare" {
			t.Fatalf("manager verification must not treat renderer helper %q as a configurable service: %v", service, enabledServices(cfg))
		}
	}
}

func TestRunPersistsConcreteRuntimeVerificationMismatch(t *testing.T) {
	orchestrator, _, operations, currentProject, snapshot, _, op := newRecoveryFixture(t, "UPDATE_CONFIG")
	orchestrator.provisioner = staticResponseProvisioner{response: contracts.ReconcileProjectResponse{
		OperationID: op.ID,
		ProjectID:   currentProject.ID,
		Revision:    snapshot.Revision,
		EnabledServices: []string{
			"db",
		},
	}}

	_, err := orchestrator.Run(context.Background(), currentProject, op, snapshot)
	if err == nil || !strings.Contains(err.Error(), "enabled services mismatch") {
		t.Fatalf("Run() error = %v, want concrete enabled service mismatch", err)
	}
	stored, err := operations.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.ErrorMessage, "enabled services mismatch") {
		t.Fatalf("stored error = %q, want concrete verification diagnostic", stored.ErrorMessage)
	}
}

func TestRunPersistsConcreteProvisionerErrorWithoutRevisionProtocol(t *testing.T) {
	orchestrator, _, operations, currentProject, snapshot, _, op := newRecoveryFixture(t, "UPDATE_CONFIG")
	orchestrator.provisioner = staticErrorProvisioner{err: contracts.ErrStaleConfigRevision}

	_, err := orchestrator.Run(context.Background(), currentProject, op, snapshot)
	if !errors.Is(err, contracts.ErrStaleConfigRevision) {
		t.Fatalf("Run() error = %v, want stale configuration revision", err)
	}
	stored, err := operations.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != operation.Failed {
		t.Fatalf("operation status = %s, want FAILED instead of a retryable RUNNING operation", stored.Status)
	}
	if !strings.Contains(stored.ErrorMessage, "stale config revision") {
		t.Fatalf("stored error = %q, want concrete provisioner diagnostic", stored.ErrorMessage)
	}
}

func TestRunDoesNotRestoreOrCompareLegacyCandidateRevision(t *testing.T) {
	orchestrator, database, operations, currentProject, snapshot, _, op := newRecoveryFixture(t, "UPDATE_CONFIG")
	orchestrator.provisioner = staticErrorProvisioner{err: contracts.ErrStaleConfigRevision}
	if _, err := database.DB().Exec(`UPDATE projects SET config_revision=3 WHERE id=?`, currentProject.ID); err != nil {
		t.Fatal(err)
	}

	_, err := orchestrator.Run(context.Background(), currentProject, op, snapshot)
	if !errors.Is(err, contracts.ErrStaleConfigRevision) {
		t.Fatalf("Run() error = %v, want stale configuration revision", err)
	}
	stored, err := operations.Get(context.Background(), op.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != operation.Failed {
		t.Fatalf("operation status = %s, want FAILED instead of a retryable RUNNING operation", stored.Status)
	}
	if !strings.Contains(stored.ErrorMessage, "stale config revision") {
		t.Fatalf("stored error = %q, want concrete provisioner diagnostic", stored.ErrorMessage)
	}
}

type staticResponseProvisioner struct {
	response contracts.ReconcileProjectResponse
}

type staticErrorProvisioner struct{ err error }

func (s staticErrorProvisioner) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return contracts.ReconcileProjectResponse{}, s.err
}

func (s staticResponseProvisioner) Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	return s.response, nil
}

func TestPreRuntimeFailureRestoresAdmittedConfigurationState(t *testing.T) {
	if !shouldRestoreConfiguration(&contracts.ReconcileFailure{RuntimeChanged: false}) {
		t.Fatal("render/stage failure must restore the admitted snapshot")
	}
	if shouldRestoreConfiguration(&contracts.ReconcileFailure{RuntimeChanged: true, RollbackSucceeded: false}) {
		t.Fatal("unrecovered runtime failure must not pretend Manager state was restored")
	}
	if !shouldRestoreConfiguration(&contracts.ReconcileFailure{RuntimeChanged: true, RollbackSucceeded: true}) {
		t.Fatal("confirmed runtime rollback must restore the admitted snapshot")
	}
}
