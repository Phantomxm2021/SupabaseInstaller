package runtime

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

func TestReconcileRecreatesOnlyAffectedService(t *testing.T) {
	cases := []struct {
		name   string
		change func(*contracts.ProjectConfiguration)
		want   []string
	}{
		{name: "smtp", change: func(c *contracts.ProjectConfiguration) { c.Auth.SMTP.SenderName = "Bee" }, want: []string{"auth"}},
		{name: "google oauth", change: func(c *contracts.ProjectConfiguration) {
			c.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: false}}
		}, want: []string{"auth"}},
		{name: "functions environment", change: func(c *contracts.ProjectConfiguration) { c.Functions.Directory = "./functions-v2" }, want: []string{"functions"}},
		{name: "storage backend", change: func(c *contracts.ProjectConfiguration) { c.Storage.Backend = contracts.StorageBackendS3 }, want: []string{"storage"}},
		{name: "site URL", change: func(c *contracts.ProjectConfiguration) { c.General.SiteURL = "https://new.example.com" }, want: []string{"auth", "studio", "api-gw"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, err := projectfs.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			runner := &fakeReconcileRunner{}
			backend := NewBackend(root, runner, &sequenceInspector{})
			first := reconcileRequest(baseConfig(), 0, 1)
			if _, err := backend.Reconcile(context.Background(), first); err != nil {
				t.Fatalf("initial reconcile: %v", err)
			}
			next := baseConfig()
			tc.change(&next)
			result, err := backend.Reconcile(context.Background(), reconcileRequest(next, 1, 2))
			if err != nil {
				t.Fatalf("update reconcile: %v", err)
			}
			if !equalStrings(runner.recreated, tc.want) {
				t.Fatalf("recreated = %#v, want %#v", runner.recreated, tc.want)
			}
			if !equalStrings(result.RecreatedServices, tc.want) {
				t.Fatalf("result recreated = %#v, want %#v", result.RecreatedServices, tc.want)
			}
		})
	}
}

func TestReconcileFailureRestoresPreviousRuntimeAndRecreatesIt(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	inspector := &sequenceInspector{}
	backend := NewBackend(root, runner, inspector)
	first := reconcileRequest(baseConfig(), 0, 1)
	if _, err := backend.Reconcile(context.Background(), first); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	previous, _ := root.RuntimeComposePath("bee")
	previousBytes := string(mustReadRuntime(t, previous))
	inspector.reports = []health.Report{{Health: contracts.HealthUnhealthy}, {Health: contracts.HealthHealthy}}
	changed := baseConfig()
	changed.General.SiteURL = "https://broken.example.com"
	result, err := backend.Reconcile(context.Background(), reconcileRequest(changed, 1, 2))
	if err == nil {
		t.Fatal("reconcile succeeded despite failed health check")
	}
	var failure *contracts.ReconcileFailure
	if !errors.As(err, &failure) || !failure.RollbackSucceeded {
		t.Fatalf("error = %v, want typed successful rollback", err)
	}
	if got := string(mustReadRuntime(t, previous)); got != previousBytes {
		t.Fatalf("runtime after rollback changed: %q", got)
	}
	if len(runner.removed) != 0 {
		t.Fatalf("removed services = %#v, want none", runner.removed)
	}
	if len(runner.recreated) != 6 {
		t.Fatalf("recreate calls = %#v, want candidate and rollback service sets", runner.recreated)
	}
	metadata, err := root.Metadata("bee")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Revision != 1 {
		t.Fatalf("metadata revision = %d, want 1", metadata.Revision)
	}
	if !result.RolledBack {
		t.Fatal("failed reconcile did not report rollback success")
	}
}

func TestReconcileValidatesStagedCandidateOnStableProjectDir(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(runner.validatedCompose, ".candidate-") {
		t.Fatalf("validated compose = %q, want staged candidate", runner.validatedCompose)
	}
	if !strings.HasSuffix(runner.validatedEnv, "/.env") || strings.Contains(runner.validatedEnv, ".manager-runtime/current") {
		t.Fatalf("validated env = %q, want candidate env", runner.validatedEnv)
	}
	project, _ := root.ProjectPath("bee")
	if runner.validatedDir != project {
		t.Fatalf("validated project dir = %q, want %q", runner.validatedDir, project)
	}
}

func TestReconcilePollsStartingHealthBeforeAdvancingRevision(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	inspector := &sequenceInspector{reports: []health.Report{{Health: contracts.HealthStarting}, {Health: contracts.HealthHealthy}}}
	backend := NewBackend(root, runner, inspector)
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatalf("reconcile with transient starting health: %v", err)
	}
	if inspector.calls < 2 {
		t.Fatalf("health calls = %d, want bounded polling", inspector.calls)
	}
}

func TestAffectedServicesCoversRenderedTopologyAndRuntimeFields(t *testing.T) {
	before := baseConfig()
	cases := []struct {
		name   string
		change func(*contracts.ProjectConfiguration)
		want   []string
	}{
		{"realtime tuning", func(c *contracts.ProjectConfiguration) { c.Realtime.MaxConnections = 200; c.Services.Realtime = true }, []string{"realtime"}},
		{"direct db", func(c *contracts.ProjectConfiguration) {
			c.Database.DirectPort = true
			c.Database.DirectPortNumber = 15432
		}, []string{"db"}},
		{"imgproxy storage", func(c *contracts.ProjectConfiguration) { c.Services.Imgproxy = true }, []string{"storage"}},
		{"gateway toggle", func(c *contracts.ProjectConfiguration) { c.Services.Gateway = false }, []string{"api-gw"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after := before
			tc.change(&after)
			if got := affectedServices(before, after); !equalStrings(got, tc.want) {
				t.Fatalf("affected = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestReconcileRemovesDisabledServicesUsingPreviousCurrentModel(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	changed := baseConfig()
	changed.Services.Storage = false
	changed.Services.Functions = false
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(changed, 1, 2)); err != nil {
		t.Fatal(err)
	}
	if !equalStrings(runner.removed, []string{"functions", "storage"}) {
		t.Fatalf("removed = %#v", runner.removed)
	}
	if strings.Contains(runner.removedCompose, ".candidate-") {
		t.Fatalf("disabled removal used candidate path: %q", runner.removedCompose)
	}
}

func TestReconcileProductionRunnerUsesCandidateThenCurrentAndDbFirst(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	executor := &captureComposeExecutor{}
	backend := NewBackend(root, compose.NewRunner(executor), &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	if len(executor.calls) < 3 {
		t.Fatalf("compose calls = %#v", executor.calls)
	}
	if !strings.Contains(strings.Join(executor.calls[0], " "), ".candidate-") {
		t.Fatalf("first compose call did not validate candidate: %#v", executor.calls[0])
	}
	db, dependent := -1, -1
	for i, call := range executor.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, " up -d --wait db") {
			db = i
		}
		if strings.Contains(joined, " up -d --remove-orphans") {
			dependent = i
		}
	}
	if db < 0 || dependent < 0 || db > dependent {
		t.Fatalf("compose calls not db-first: %#v", executor.calls)
	}
}

func TestReconcileRejectsNonAdvancingOrMismatchedSnapshotRevision(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(root, &fakeReconcileRunner{}, &sequenceInspector{})
	first := reconcileRequest(baseConfig(), 0, 1)
	if _, err := backend.Reconcile(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	for _, next := range []int64{1, 0} {
		request := reconcileRequest(baseConfig(), 1, next)
		request.IdempotencyKey += "-invalid"
		if _, err := backend.Reconcile(context.Background(), request); err == nil {
			t.Fatalf("next revision %d was accepted", next)
		}
	}
	mismatch := reconcileRequest(baseConfig(), 1, 2)
	mismatch.Configuration.Revision = 1
	if _, err := backend.Reconcile(context.Background(), mismatch); err == nil {
		t.Fatal("mismatched snapshot revision was accepted")
	}
}

func TestReconcileFailureIsTypedCachedAndReplayedWithoutDocker(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	inspector := &sequenceInspector{}
	backend := NewBackend(root, runner, inspector)
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	inspector.reports = []health.Report{{Health: contracts.HealthUnhealthy}, {Health: contracts.HealthHealthy}}
	request := reconcileRequest(func() contracts.ProjectConfiguration {
		c := baseConfig()
		c.General.SiteURL = "https://failure.example.com"
		return c
	}(), 1, 2)
	before := runner.validated + len(runner.recreated) + len(runner.removed)
	result, err := backend.Reconcile(context.Background(), request)
	if err == nil || !result.RolledBack || result.Error == nil {
		t.Fatalf("first failure result=%#v err=%v", result, err)
	}
	after := runner.validated + len(runner.recreated) + len(runner.removed)
	if _, replayErr := backend.Reconcile(context.Background(), request); replayErr == nil {
		t.Fatal("failure replay returned success")
	}
	if got := runner.validated + len(runner.recreated) + len(runner.removed); got != after || after <= before {
		t.Fatalf("replay performed Docker operations: before=%d after=%d now=%d", before, after, got)
	}
}

func TestReconcileMetadataWriteFailureRestoresRuntimeBeforeReturning(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	composePath, _ := root.RuntimeComposePath("bee")
	before := string(mustReadRuntime(t, composePath))
	failMetadata := true
	root.SetMetadataWriteHookForTest(func(string, projectfs.Metadata) error {
		if failMetadata {
			return errors.New("injected metadata write failure")
		}
		return nil
	})
	changed := baseConfig()
	changed.General.SiteURL = "https://metadata-failure.example.com"
	result, err := backend.Reconcile(context.Background(), reconcileRequest(changed, 1, 2))
	if err == nil || !result.RolledBack {
		t.Fatalf("result=%#v err=%v, want rollback", result, err)
	}
	if got := string(mustReadRuntime(t, composePath)); got != before {
		t.Fatalf("runtime after metadata failure = %q, want old", got)
	}
}

type fakeReconcileRunner struct {
	validated        int
	validatedDir     string
	validatedCompose string
	validatedEnv     string
	up               [][]string
	recreated        []string
	removed          []string
	removedCompose   string
}

type captureComposeExecutor struct{ calls [][]string }

func (e *captureComposeExecutor) Run(_ context.Context, _ string, args, _ []string) ([]byte, error) {
	e.calls = append(e.calls, append([]string(nil), args...))
	return nil, nil
}

func (r *fakeReconcileRunner) UpDatabase(context.Context, compose.ProjectRef) error { return nil }
func (r *fakeReconcileRunner) UpServices(_ context.Context, _ compose.ProjectRef, services ...string) error {
	r.up = append(r.up, append([]string(nil), services...))
	return nil
}
func (r *fakeReconcileRunner) Stop(context.Context, compose.ProjectRef) error { return nil }
func (r *fakeReconcileRunner) Restart(context.Context, compose.ProjectRef, ...string) error {
	return nil
}
func (r *fakeReconcileRunner) DownRuntime(context.Context, compose.ProjectRef) error { return nil }
func (r *fakeReconcileRunner) Validate(_ context.Context, project compose.ProjectRef) error {
	r.validated++
	r.validatedDir, r.validatedCompose, r.validatedEnv = project.Dir, project.ComposeFile, project.EnvFile
	return nil
}
func (r *fakeReconcileRunner) UpSelected(_ context.Context, _ compose.ProjectRef, services ...string) error {
	r.up = append(r.up, append([]string(nil), services...))
	return nil
}
func (r *fakeReconcileRunner) Recreate(_ context.Context, _ compose.ProjectRef, services ...string) error {
	r.recreated = append(r.recreated, services...)
	return nil
}
func (r *fakeReconcileRunner) RemoveStopped(_ context.Context, project compose.ProjectRef, services ...string) error {
	r.removed = append(r.removed, services...)
	r.removedCompose = project.ComposeFile
	return nil
}

type sequenceInspector struct {
	reports []health.Report
	calls   int
}

func (i *sequenceInspector) Project(context.Context, health.ProjectRef) (health.Report, error) {
	i.calls++
	if len(i.reports) == 0 {
		return health.Report{Health: contracts.HealthHealthy}, nil
	}
	report := i.reports[0]
	i.reports = i.reports[1:]
	return report, nil
}

func baseConfig() contracts.ProjectConfiguration {
	return contracts.ProjectConfiguration{
		General:  contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://bee.example.com", SupabaseVersion: "self-hosted/v0.8.0"},
		Services: contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true, Storage: true, Functions: true},
		Auth:     contracts.AuthConfig{Enabled: true, Email: contracts.EmailAuthConfig{Enabled: true, AllowSignup: true}},
		Storage:  contracts.StorageConfig{Backend: contracts.StorageBackendLocal},
		Database: contracts.DatabaseConfig{Version: "15", MaxConnections: 100},
		Network:  contracts.NetworkConfig{Gateway: contracts.GatewayEnvoy, HTTPSMode: contracts.HTTPSModeExternal, APIPort: 18001},
	}
}

func reconcileRequest(config contracts.ProjectConfiguration, expected, next int64) contracts.ReconcileProjectRequest {
	config.Revision = next
	return contracts.ReconcileProjectRequest{OperationID: "op-" + strings.TrimSpace(config.General.SiteURL), IdempotencyKey: "key-" + strings.TrimSpace(config.General.SiteURL) + string(rune(next)), ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", ExpectedRevision: expected, NextRevision: next, APIPort: 18001, Configuration: config, RuntimeSecrets: map[string]string{"storage.secretAccessKey": "storage-secret"}, Secrets: contracts.ProjectSecrets{DatabasePassword: "db-password", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"}}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func mustReadRuntime(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
