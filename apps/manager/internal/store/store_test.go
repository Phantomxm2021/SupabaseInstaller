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
	if err := s.CreateProject(context.Background(), want, configurationFixture()); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	got, err := s.GetProject(context.Background(), want.ID)
	if err != nil {
		t.Fatalf("GetProject() error = %v", err)
	}
	if got.ID != want.ID || got.Slug != want.Slug || !got.Services.Database || got.Health != contracts.HealthUnknown {
		t.Fatalf("GetProject() = %#v, want persisted project", got)
	}
	var enabled int
	if err := s.DB().QueryRow(`SELECT enabled FROM project_services WHERE project_id = ? AND service = 'database'`, want.ID).Scan(&enabled); err != nil || enabled != 1 {
		t.Fatalf("database service projection = %d, %v; want enabled", enabled, err)
	}
}

func TestConfigurationLeaseSerializesAndRecoversAfterExpiry(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	fence, acquired, err := s.AcquireConfigurationLeaseWithFence(context.Background(), project.ID, "first", now, time.Minute)
	if err != nil || !acquired || fence != 1 {
		t.Fatalf("first lease = %d, %v, %v", fence, acquired, err)
	}
	_, acquired, err = s.AcquireConfigurationLeaseWithFence(context.Background(), project.ID, "second", now.Add(time.Second), time.Minute)
	if err != nil || acquired {
		t.Fatalf("second lease while held = %v, %v", acquired, err)
	}
	fence, acquired, err = s.AcquireConfigurationLeaseWithFence(context.Background(), project.ID, "second", now.Add(2*time.Minute), time.Minute)
	if err != nil || !acquired {
		t.Fatalf("expired lease recovery = %v, %v", acquired, err)
	}
	if err := s.ReleaseConfigurationLeaseOwned(context.Background(), project.ID, "second", fence); err != nil {
		t.Fatal(err)
	}
}

func TestConfigurationLeaseReleaseIsOwnerAndFenceBound(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	fence, acquired, err := s.AcquireConfigurationLeaseWithFence(context.Background(), project.ID, "first", now, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire = %d, %v, %v", fence, acquired, err)
	}
	if err := s.ReleaseConfigurationLeaseOwned(context.Background(), project.ID, "stale-owner", fence); err != nil {
		t.Fatal(err)
	}
	if _, acquired, err := s.AcquireConfigurationLeaseWithFence(context.Background(), project.ID, "second", now.Add(2*time.Minute), time.Minute); err != nil || !acquired {
		t.Fatalf("expired takeover = %v, %v", acquired, err)
	}
	if err := s.ReleaseConfigurationLeaseOwned(context.Background(), project.ID, "first", fence); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM project_configuration_leases WHERE project_id = ?`, project.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("stale owner release removed successor: count=%d err=%v", count, err)
	}
}

func TestAdmitConfigurationIgnoresPortsOwnedByDisabledServices(t *testing.T) {
	s := openTestStore(t)
	first := projectFixture()
	if err := s.CreateProject(context.Background(), first, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	second := projectFixture()
	second.ID, second.Slug, second.Name, second.Domain = "project-2", "bee-two", "Bee Two", "two.example.com"
	second.Services = contracts.Services{Database: true}
	secondCfg := configurationFixture()
	secondCfg.General.Domain = second.Domain
	secondCfg.Services = second.Services
	if err := s.CreateProject(context.Background(), second, secondCfg); err != nil {
		t.Fatal(err)
	}
	// Project one owns the pooler ports, while project two carries the same
	// values in its disabled-owner aggregate. They must not participate in
	// admission conflict checks until Supavisor is enabled.
	if _, err := s.DB().Exec(`INSERT INTO port_allocations(port,project_id,kind,created_at) VALUES(6543,?,?,?), (6544,?,?,?)`, first.ID, "POOLER_TRANSACTION", formatTime(time.Now()), first.ID, "POOLER_SESSION", formatTime(time.Now())); err != nil {
		t.Fatal(err)
	}
	candidate := secondCfg
	candidate.Pooler.TransactionPort, candidate.Pooler.SessionPort = 6543, 6544
	candidate.Network.PoolerPort = 6543
	now := time.Now().UTC()
	queued := contracts.Operation{ID: "op-disabled-owner", ProjectID: second.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: now}
	if _, _, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: queued, ProjectID: second.ID, Owner: queued.ID, ExpectedRevision: 1, Configuration: candidate, OperationKind: "UPDATE_CONFIG", Now: now}); err != nil {
		t.Fatalf("disabled-owner pooler ports blocked admission: %v", err)
	}
	if err := s.ReleaseConfigurationLeaseOwned(context.Background(), second.ID, queued.ID, 1); err != nil {
		t.Fatal(err)
	}
	// Enabling the owner makes the same collision real and admission must then
	// reject it before creating an operation or advancing the revision.
	candidate.Services.Supavisor = true
	enabled := queued
	enabled.ID = "op-enabled-owner"
	if _, _, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: enabled, ProjectID: second.ID, Owner: enabled.ID, ExpectedRevision: 2, Configuration: candidate, OperationKind: "UPDATE_CONFIG", Now: now.Add(time.Second)}); !errors.Is(err, ErrConfigurationConflict) {
		t.Fatalf("enabled-owner pooler conflict = %v, want ErrConfigurationConflict", err)
	}
}

func TestAdmitConfigurationReservesPortsAcrossKindsAndPendingCandidates(t *testing.T) {
	s := openTestStore(t)
	first, second := projectFixture(), projectFixture()
	first.ID, first.Slug, first.Name, first.Domain = "project-1", "bee-one", "Bee One", "one.example.com"
	second.ID, second.Slug, second.Name, second.Domain = "project-2", "bee-two", "Bee Two", "two.example.com"
	firstCfg, secondCfg := configurationFixture(), configurationFixture()
	firstCfg.General.Domain, secondCfg.General.Domain = first.Domain, second.Domain
	if err := s.CreateProject(context.Background(), first, firstCfg); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateProject(context.Background(), second, secondCfg); err != nil {
		t.Fatal(err)
	}
	candidate := configurationFixture()
	candidate.General.Domain = "pending.example.com"
	candidate.Network.APIPort = 6123
	queued := contracts.Operation{ID: "op-pending-a", ProjectID: first.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: time.Now()}
	if _, _, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: queued, ProjectID: first.ID, Owner: queued.ID, ExpectedRevision: 1, Configuration: candidate, OperationKind: "UPDATE_CONFIG", Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	conflicting := candidate
	conflicting.Network.StudioPort = 6123
	conflicting.Services.Studio = true
	queued2 := contracts.Operation{ID: "op-pending-b", ProjectID: second.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: time.Now()}
	if _, _, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: queued2, ProjectID: second.ID, Owner: queued2.ID, ExpectedRevision: 1, Configuration: conflicting, OperationKind: "UPDATE_CONFIG", Now: time.Now()}); !errors.Is(err, ErrConfigurationConflict) {
		t.Fatalf("pending cross-kind conflict = %v, want ErrConfigurationConflict", err)
	}
}

func TestRestoreConfigurationStateOwnedDoesNotEraseSuccessor(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	candidate := configurationFixture()
	candidate.General.Domain = "successor.example.com"
	queued := contracts.Operation{ID: "op-owner", ProjectID: project.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: time.Now()}
	snapshot, lease, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: queued, ProjectID: project.ID, Owner: queued.ID, ExpectedRevision: 1, Configuration: candidate, OperationKind: "UPDATE_CONFIG", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`UPDATE projects SET config_revision=3 WHERE id=?`, project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`INSERT INTO project_secrets(project_id,kind,envelope_version,nonce,ciphertext,updated_at) VALUES(?,?,?,?,?,?)`, project.ID, "successor", 1, []byte("n"), []byte("successor-ciphertext"), formatTime(time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreConfigurationStateOwned(context.Background(), project.ID, snapshot.Revision, queued.ID, lease.Fence, time.Now()); !errors.Is(err, ErrStaleConfiguration) {
		t.Fatalf("stale restore = %v, want ErrStaleConfiguration", err)
	}
	var count int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM project_secrets WHERE project_id=? AND kind='successor'`, project.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("successor secret count = %d, %v", count, err)
	}
}

func TestRestoreConfigurationStateOwnedCleansFailedRevisionForNextAdmission(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	candidate := configurationFixture()
	candidate.General.Domain = "failed.example.com"
	now := time.Now().UTC()
	op := contracts.Operation{ID: "op-failed", ProjectID: project.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: now}
	snapshot, lease, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: op, ProjectID: project.ID, Owner: op.ID, ExpectedRevision: 1, Configuration: candidate, OperationKind: "UPDATE_CONFIG", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreConfigurationStateOwned(context.Background(), project.ID, snapshot.Revision, op.ID, lease.Fence, now); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: contracts.Operation{ID: "op-next", ProjectID: project.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: now.Add(time.Second)}, ProjectID: project.ID, Owner: "op-next", ExpectedRevision: 1, Configuration: configurationFixture(), OperationKind: "UPDATE_CONFIG", Now: now.Add(time.Second)}); err != nil {
		t.Fatalf("next admission after restore = %v", err)
	}
}

func TestRestoreConfigurationStateOwnedAllowsExpiredOriginalFence(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	op := contracts.Operation{ID: "expired-owner", ProjectID: project.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: now}
	candidate := configurationFixture()
	candidate.General.Domain = "expired-failed.example.com"
	snapshot, lease, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: op, ProjectID: project.ID, Owner: op.ID, ExpectedRevision: 1, Configuration: candidate, OperationKind: "UPDATE_CONFIG", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreConfigurationStateOwned(context.Background(), project.ID, snapshot.Revision, op.ID, lease.Fence, now.Add(2*time.Hour)); err != nil {
		t.Fatalf("expired original owner restore = %v", err)
	}
}

func TestOperationCompensationStateIsDurable(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	op := contracts.Operation{ID: "comp-op", ProjectID: project.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: time.Now()}
	if err := s.CreateOperation(context.Background(), op, "OPERATION_QUEUED", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOperationCompensation(context.Background(), op.ID, "ROLLBACK_PENDING", "comp-op:rollback"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetOperationCompensation(context.Background(), op.ID)
	if err != nil || got.Phase != "ROLLBACK_PENDING" || got.Key != "comp-op:rollback" {
		t.Fatalf("compensation state=%#v err=%v", got, err)
	}
}

func TestMarkConfigurationGoodOwnedPublishesCommitPhaseAtomically(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	candidate := configurationFixture()
	candidate.General.Domain = "committed.example.com"
	op := contracts.Operation{ID: "commit-phase-op", ProjectID: project.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: now}
	snapshot, lease, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: op, ProjectID: project.ID, Owner: op.ID, ExpectedRevision: 1, Configuration: candidate, OperationKind: "UPDATE_CONFIG", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MarkConfigurationGoodOwned(context.Background(), project.ID, snapshot.Revision, op.ID, lease.Fence, "COMMITTED", now); err != nil {
		t.Fatal(err)
	}
	var phase string
	if err := s.DB().QueryRow(`SELECT compensation_phase FROM operations WHERE id=?`, op.ID).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != "COMMITTED" {
		t.Fatalf("commit phase = %q, want COMMITTED", phase)
	}
}

func TestOwnedConfigurationPublicationRejectsEmptyOwnerOrFence(t *testing.T) {
	database := openTestStore(t)
	project := projectFixture()
	if err := database.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	op := contracts.Operation{ID: "strict-owner-op", ProjectID: project.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: time.Now()}
	snapshot, _, err := database.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: op, ProjectID: project.ID, Owner: op.ID, ExpectedRevision: 1, Configuration: configurationFixture(), OperationKind: "UPDATE_CONFIG", Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.MarkConfigurationGoodOwned(context.Background(), project.ID, snapshot.Revision, "", 1, "COMMITTED", time.Now()); err == nil {
		t.Fatal("MarkConfigurationGoodOwned accepted an empty owner")
	}
	if err := database.MarkConfigurationGoodOwned(context.Background(), project.ID, snapshot.Revision, op.ID, 0, "COMMITTED", time.Now()); err == nil {
		t.Fatal("MarkConfigurationGoodOwned accepted an empty fence")
	}
}

func TestRestoreConfigurationStateOwnedIsIdempotentAfterStateRestored(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	candidate := configurationFixture()
	candidate.General.Domain = "restored.example.com"
	op := contracts.Operation{ID: "restored-op", ProjectID: project.ID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: now}
	snapshot, lease, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: op, ProjectID: project.ID, Owner: op.ID, ExpectedRevision: 1, Configuration: candidate, OperationKind: "UPDATE_CONFIG", Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreConfigurationStateOwned(context.Background(), project.ID, snapshot.Revision, op.ID, lease.Fence, now); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreConfigurationStateOwned(context.Background(), project.ID, snapshot.Revision, op.ID, lease.Fence, now); err != nil {
		t.Fatalf("second restore = %v, want idempotent success", err)
	}
}

func TestSnapshotMarkerDistinguishesEmptyRevision(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	var present int
	if err := s.DB().QueryRow(`SELECT present FROM project_secret_snapshot_markers WHERE project_id = ? AND revision = 1`, project.ID).Scan(&present); err != nil || present != 0 {
		t.Fatalf("revision-1 empty marker = %d, %v", present, err)
	}
}

func TestStorePersistsOnlyEncryptedSecretEnvelope(t *testing.T) {
	s := openTestStore(t)
	project := projectFixture()
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
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
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
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
	saved, err := admitStoreConfiguration(s, project.ID, 1, cfg, "revision-first")
	if err != nil {
		t.Fatalf("admission error = %v", err)
	}
	if saved.Revision != 2 {
		t.Fatalf("saved revision = %d, want 2", saved.Revision)
	}
	if _, err := admitStoreConfiguration(s, project.ID, 1, cfg, "revision-stale"); !errors.Is(err, ErrStaleConfiguration) {
		t.Fatalf("stale admission error = %v, want ErrStaleConfiguration", err)
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
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
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
	if _, err := legacy.Exec(`UPDATE projects SET config_revision = 7 WHERE id = ?`, project.ID); err != nil {
		t.Fatalf("set legacy revision: %v", err)
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
	if snapshot.Revision != 7 || snapshot.LastGoodRevision != 7 {
		t.Fatalf("upgraded snapshot = %#v", snapshot)
	}
	if snapshot.Configuration.General.Domain != project.Domain || snapshot.Configuration.General.SiteURL != project.SiteURL || snapshot.Configuration.General.SupabaseVersion != project.SupabaseVersion {
		t.Fatalf("legacy general projection = %#v", snapshot.Configuration.General)
	}
	services := snapshot.Configuration.Services
	if !services.Database || !services.Gateway || !services.Studio || !services.PostgresMeta || services.Imgproxy || services.Storage || services.Logs || services.Vector {
		t.Fatalf("legacy service defaults/closure invalid: %#v", services)
	}
	if snapshot.Configuration.General.SupabaseVersion != "self-hosted/v0.8.0" || snapshot.Configuration.General.Domain == "" || snapshot.Configuration.General.SiteURL == "" || snapshot.Configuration.Realtime.MaxConnections != 100 || snapshot.Configuration.Realtime.DatabasePoolSize != 5 || snapshot.Configuration.Realtime.LogLevel != contracts.LogLevelInfo || snapshot.Configuration.Database.MaxConnections != 100 || snapshot.Configuration.Pooler.PoolSize != 20 || snapshot.Configuration.Pooler.MaxClientConnections != 100 || snapshot.Configuration.Network.Gateway != contracts.GatewayEnvoy || snapshot.Configuration.Network.HTTPSMode != contracts.HTTPSModeExternal || snapshot.Configuration.Storage.Backend != contracts.StorageBackendLocal {
		t.Fatalf("legacy safe defaults missing: %#v", snapshot.Configuration)
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
	if err := s.CreateProject(context.Background(), project, configurationFixture()); err != nil {
		t.Fatal(err)
	}
	first := configurationFixture()
	first.General.Domain = "first.example.com"
	if _, err := admitStoreConfiguration(s, project.ID, 1, first, "immutable-first"); err != nil {
		t.Fatal(err)
	}
	second := first
	second.General.Domain = "second.example.com"
	if _, err := admitStoreConfiguration(s, project.ID, 2, second, "immutable-second"); err != nil {
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

// admitStoreConfiguration is test-local: production writes must enter through
// the owned admission boundary rather than a direct Save API.
func admitStoreConfiguration(s *Store, projectID string, expected int64, cfg contracts.ProjectConfiguration, operationID string) (ConfigurationSnapshot, error) {
	now := time.Now().UTC()
	op := contracts.Operation{ID: operationID, ProjectID: projectID, Type: contracts.OperationUpdateConfig, Status: contracts.OperationQueued, CreatedAt: now}
	snapshot, lease, err := s.AdmitConfiguration(context.Background(), ConfigurationAdmission{Operation: op, ProjectID: projectID, Owner: operationID, ExpectedRevision: expected, Configuration: cfg, OperationKind: "UPDATE_CONFIG", Now: now})
	if err != nil {
		return ConfigurationSnapshot{}, err
	}
	if err := s.ReleaseConfigurationLeaseOwned(context.Background(), projectID, operationID, lease.Fence); err != nil {
		return ConfigurationSnapshot{}, err
	}
	return snapshot, nil
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
