package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/render"
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

func TestAcceptanceInspectorFailureRestoresPreviousRuntimeAndRecreatesPriorAuth(t *testing.T) {
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
	if !containsString(runner.recreated, "auth") {
		t.Fatalf("recreated services = %#v, want prior Auth recreated during rollback", runner.recreated)
	}
	projectPath, _ := root.ProjectPath("bee")
	currentPointer := filepath.Join(projectPath, ".manager-runtime", "current")
	if target, readErr := os.Readlink(currentPointer); readErr != nil || !strings.HasPrefix(target, "generations/") {
		t.Fatalf("current runtime pointer = %q, err=%v; want restored generation", target, readErr)
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

func TestAcceptanceInspectorFailpointForcesCandidateRollback(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	backend.EnableAcceptanceInspectorFailure()
	changed := baseConfig()
	changed.General.SiteURL = "https://failpoint.example.com"
	changed.Auth.OAuth = map[string]contracts.OAuthProviderConfig{"google": {Enabled: false}}
	result, err := backend.Reconcile(context.Background(), reconcileRequest(changed, 1, 2))
	if err == nil {
		t.Fatal("failpoint reconcile unexpectedly succeeded")
	}
	var failure *contracts.ReconcileFailure
	if !errors.As(err, &failure) || !failure.RollbackSucceeded || !result.RolledBack {
		t.Fatalf("failpoint result=%#v failure=%v, want successful rollback", result, err)
	}
	metadata, err := root.Metadata("bee")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Revision != 1 {
		t.Fatalf("metadata revision=%d, want 1 after failpoint rollback", metadata.Revision)
	}
	if len(runner.recreated) < 2 {
		t.Fatalf("inspector failpoint recreated services=%#v, want candidate and previous runtime", runner.recreated)
	}
}

func TestReconcileRenderFailureReportsRuntimeUnchangedForManagerRecovery(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	backend := NewBackend(root, &fakeReconcileRunner{}, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	changed := baseConfig()
	changed.Database.Extensions = []string{"unsupported-extension"}
	result, err := backend.Reconcile(context.Background(), reconcileRequest(changed, 1, 2))
	if err == nil {
		t.Fatal("render-invalid reconcile unexpectedly succeeded")
	}
	var failure *contracts.ReconcileFailure
	if !errors.As(err, &failure) {
		t.Fatalf("error = %v, want typed reconcile failure", err)
	}
	if failure.RuntimeChanged || result.RuntimeChanged {
		t.Fatalf("render failure reported runtime changed: failure=%v result=%#v", failure.RuntimeChanged, result)
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

func TestHealthFailureErrorNamesUnhealthyServices(t *testing.T) {
	err := healthFailureError(health.Report{
		Health: contracts.HealthUnhealthy,
		Services: []contracts.ServiceState{
			{Name: "db", Health: contracts.HealthHealthy, Status: "running"},
			{Name: "auth", Health: contracts.HealthUnhealthy, Status: "restarting"},
			{Name: "realtime", Health: contracts.HealthStarting, Status: "running"},
		},
	})
	message := err.Error()
	for _, want := range []string{"runtime health is UNHEALTHY", "auth (restarting, UNHEALTHY)", "realtime (running, STARTING)"} {
		if !strings.Contains(message, want) {
			t.Fatalf("healthFailureError() = %q, missing %q", message, want)
		}
	}
}

func TestInitialReconcileFailureRemovesRuntimeBeforeClearingGeneration(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	inspector := &sequenceInspector{reports: []health.Report{{Health: contracts.HealthUnhealthy}}}
	backend := NewBackend(root, runner, inspector)

	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err == nil {
		t.Fatal("initial reconcile unexpectedly succeeded")
	}
	if len(runner.down) != 2 {
		t.Fatalf("down calls = %d, want startup cleanup and rollback", len(runner.down))
	}
	if len(runner.removed) != 0 {
		t.Fatalf("remove-stopped calls = %#v, want none for initial failure", runner.removed)
	}
}

func TestInitialReconcileFailureResetsDatabaseData(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, err := root.ProjectPath("bee")
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{onUpDatabase: func() error {
		data := filepath.Join(project, "volumes", "db", "data")
		if err := os.MkdirAll(data, 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(data, "PG_VERSION"), []byte("17"), 0o600)
	}}
	backend := NewBackend(root, runner, &sequenceInspector{reports: []health.Report{{Health: contracts.HealthUnhealthy}}})

	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err == nil {
		t.Fatal("initial reconcile unexpectedly succeeded")
	}
	if _, err := os.Lstat(filepath.Join(project, "volumes", "db", "data")); !os.IsNotExist(err) {
		t.Fatalf("initial database data remains after failed reconcile: %v", err)
	}
}

func TestInitialReconcileStartsDatabaseBeforeDependents(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	if !equalStrings(runner.calls, []string{"down", "reset-db-config", "db", "verify-bootstrap", "sync-db-roles", "selected"}) {
		t.Fatalf("runtime calls = %#v, want clean database configuration and start before dependents", runner.calls)
	}
}

func TestInitialRetryRemovesPartialDatabaseBeforeBootstrap(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, commit, err := root.StageRuntimeFiles("bee", projectfs.RuntimeFiles{Compose: []byte("previous"), Env: []byte("previous"), FunctionsEnv: []byte("previous")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	project, err := root.ProjectPath("bee")
	if err != nil {
		t.Fatal(err)
	}
	dataDirectory := filepath.Join(project, "volumes", "db", "data")
	if err := os.MkdirAll(dataDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDirectory, "PG_VERSION"), []byte("15"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &fakeReconcileRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatalf("initial retry reconcile: %v", err)
	}
	if !equalStrings(runner.calls, []string{"down", "reset-db-config", "db", "verify-bootstrap", "sync-db-roles", "selected"}) {
		t.Fatalf("runtime calls = %#v, want stop/reset bootstrap and database configuration before db", runner.calls)
	}
	if _, err := os.Lstat(dataDirectory); !os.IsNotExist(err) {
		t.Fatalf("incomplete live data remains: %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(project, "volumes", "db", "data.failed-bootstrap-*"))
	if err != nil || len(backups) != 0 {
		t.Fatalf("database backups = %v, err=%v; want none", backups, err)
	}
}

func TestInitialReconcileRollsBackIncompleteDatabaseBootstrapBeforeStartingDependents(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, err := root.ProjectPath("bee")
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{bootstrapError: errors.New("official PostgreSQL bootstrap is incomplete: schema:graphql_public owner=\"\" want \"supabase_admin\"")}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err == nil || !strings.Contains(errors.Unwrap(err).Error(), "official PostgreSQL bootstrap is incomplete") {
		t.Fatalf("initial reconcile error = %v, want bootstrap contract violation", err)
	}
	if !equalStrings(runner.calls, []string{"down", "reset-db-config", "db", "verify-bootstrap", "down", "reset-db-config"}) {
		t.Fatalf("runtime calls = %#v, want rollback before dependent services", runner.calls)
	}
	if len(runner.up) != 0 {
		t.Fatalf("dependent services were started after an incomplete bootstrap: %#v", runner.up)
	}
	if _, err := os.Lstat(filepath.Join(project, "volumes", "db", "data")); !os.IsNotExist(err) {
		t.Fatalf("incomplete bootstrap data remains after rollback: %v", err)
	}
}

func TestHealthTimeoutErrorNamesServicesStillStarting(t *testing.T) {
	err := healthTimeoutError(health.Report{Services: []contracts.ServiceState{
		{Name: "auth", Health: contracts.HealthStarting, Status: "running"},
		{Name: "realtime", Health: contracts.HealthStarting, Status: "running"},
	}})
	message := err.Error()
	if !strings.Contains(message, "auth") || !strings.Contains(message, "realtime") || !strings.Contains(message, "running") {
		t.Fatalf("timeout error = %q, want pending service diagnostics", message)
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
			c.Services.DirectDB = true
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

func TestReconcileRollbackRemovesServicesAddedByFailedCandidate(t *testing.T) {
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
	changed := baseConfig()
	changed.Services.Imgproxy = true
	result, err := backend.Reconcile(context.Background(), reconcileRequest(changed, 1, 2))
	if err == nil || !result.RolledBack {
		t.Fatalf("result=%#v err=%v, want rollback", result, err)
	}
	if !containsString(runner.removed, "imgproxy") {
		t.Fatalf("removed=%#v, want newly added imgproxy cleanup", runner.removed)
	}
}

func TestReconcileValidationFailureDoesNotRecreatePreviousServices(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	runner.validateError = errors.New("invalid candidate")
	changed := baseConfig()
	changed.General.SiteURL = "https://validation-failure.example.com"
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(changed, 1, 2)); err == nil {
		t.Fatal("validation failure returned success")
	}
	if len(runner.recreated) != 0 {
		t.Fatalf("recreated=%#v, want no runtime mutation on validation failure", runner.recreated)
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
	if !strings.Contains(runner.removedCompose, string(filepath.Separator)+"generations"+string(filepath.Separator)) {
		t.Fatalf("disabled removal did not use immutable previous generation: %q", runner.removedCompose)
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

func TestInitialRollbackRestoresPointerWhenCandidateCleanupFails(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{downError: errors.New("injected candidate cleanup failure")}
	backend := NewBackend(root, runner, &sequenceInspector{reports: []health.Report{{Health: contracts.HealthUnhealthy}}})
	result, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1))
	if err == nil || result.RolledBack {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	current, _ := root.RuntimeComposePath("bee")
	if _, statErr := os.Lstat(current); !os.IsNotExist(statErr) {
		t.Fatalf("candidate current survived cleanup failure: %v", statErr)
	}
}

func TestReconcilePollsRealInspectorTransientServiceState(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source := &sequencedContainerSource{}
	backend := NewBackend(root, &fakeReconcileRunner{}, health.NewInspector(source))
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if source.calls < 2 {
		t.Fatalf("container source calls = %d, want polling", source.calls)
	}
}

func TestRealComposeParserValidatesRevisionZeroFunctionsCandidateWithoutCurrent(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cfg := baseConfig()
	cfg.Revision = 1
	out, err := render.Project(render.Input{ProjectID: "project-1", Slug: "bee", APIPort: 18001, Configuration: cfg, Secrets: contracts.ProjectSecrets{DatabasePassword: "db-password", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"}, RuntimeSecrets: map[string]string{"storage.secretAccessKey": "storage-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	ref, restore, _, err := root.StageRuntimeFilesWithRef("bee", projectfs.RuntimeFiles{Compose: []byte(out.Compose), Env: []byte(out.Env), FunctionsEnv: []byte(out.FunctionsEnv)})
	if err != nil {
		t.Fatal(err)
	}
	if err := pointFunctionsEnvAtCandidate(ref, out.Compose); err != nil {
		t.Fatal(err)
	}
	if err := compose.NewRunner(compose.OSExecutor{}).Validate(context.Background(), compose.ProjectRef{Slug: "bee", Dir: ref.ProjectDir, ComposeFile: ref.ComposeFile, EnvFile: ref.EnvFile}); err != nil {
		t.Fatalf("real compose config validation: %v", err)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
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
	if !strings.Contains(result.Error.Message, "runtime health is") {
		t.Fatalf("failure diagnostic = %q, want the concrete health reason", result.Error.Message)
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

func TestReconcileMetadataWriteAndRuntimeRollbackFailureReportsChanged(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeReconcileRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	writes := 0
	root.SetMetadataWriteHookForTest(func(string, projectfs.Metadata) error {
		writes++
		if writes >= 1 {
			return errors.New("injected final metadata publication failure")
		}
		return nil
	})
	runner.recreateError = errors.New("injected runtime rollback failure")
	changed := baseConfig()
	changed.General.SiteURL = "https://dual-failure.example.com"
	result, err := backend.Reconcile(context.Background(), reconcileRequest(changed, 1, 2))
	if err == nil || result.RolledBack || !result.RuntimeChanged {
		t.Fatalf("result=%#v err=%v, want changed runtime with unknown rollback", result, err)
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
	removeError      error
	downError        error
	validateError    error
	recreateError    error
	down             []string
	calls            []string
	onUpDatabase     func() error
	bootstrapError   error
}

type captureComposeExecutor struct{ calls [][]string }

func (e *captureComposeExecutor) Run(_ context.Context, _ string, args, _ []string) ([]byte, error) {
	e.calls = append(e.calls, append([]string(nil), args...))
	if strings.Contains(strings.Join(args, " "), "exec -T db psql") {
		return []byte("schema:auth:supabase_admin\nschema:graphql_public:supabase_admin\nfunction:auth.email:supabase_auth_admin\nfunction:auth.role:supabase_auth_admin\nfunction:auth.uid:supabase_auth_admin\n"), nil
	}
	return nil, nil
}

func (e *captureComposeExecutor) RunInput(_ context.Context, _ string, args, _ []string, _ []byte) ([]byte, error) {
	e.calls = append(e.calls, append([]string(nil), args...))
	return nil, nil
}

type sequencedContainerSource struct{ calls int }

func (s *sequencedContainerSource) Containers(context.Context, string) ([]health.Container, error) {
	s.calls++
	state, healthState := "restarting", "starting"
	if s.calls > 1 {
		state, healthState = "running", "healthy"
	}
	containers := make([]health.Container, 0, 8)
	for _, service := range []string{"api-gw", "auth", "auth-templates", "db", "functions", "meta", "rest", "storage", "studio"} {
		containers = append(containers, health.Container{Service: service, State: state, Health: healthState})
	}
	return containers, nil
}

func (r *fakeReconcileRunner) UpDatabase(context.Context, compose.ProjectRef) error {
	r.calls = append(r.calls, "db")
	if r.onUpDatabase != nil {
		return r.onUpDatabase()
	}
	return nil
}
func (r *fakeReconcileRunner) VerifyDatabaseBootstrap(context.Context, compose.ProjectRef) error {
	r.calls = append(r.calls, "verify-bootstrap")
	return r.bootstrapError
}
func (r *fakeReconcileRunner) SynchronizeDatabaseRolePasswords(context.Context, compose.ProjectRef) error {
	r.calls = append(r.calls, "sync-db-roles")
	return nil
}
func (r *fakeReconcileRunner) ResetDatabaseConfig(context.Context, compose.ProjectRef) error {
	r.calls = append(r.calls, "reset-db-config")
	return nil
}
func (r *fakeReconcileRunner) UpServices(_ context.Context, _ compose.ProjectRef, services ...string) error {
	r.up = append(r.up, append([]string(nil), services...))
	return nil
}
func (r *fakeReconcileRunner) Stop(context.Context, compose.ProjectRef) error { return nil }
func (r *fakeReconcileRunner) Restart(context.Context, compose.ProjectRef, ...string) error {
	return nil
}
func (r *fakeReconcileRunner) DownRuntime(_ context.Context, project compose.ProjectRef) error {
	r.down = append(r.down, project.ComposeFile)
	r.calls = append(r.calls, "down")
	return r.downError
}
func (r *fakeReconcileRunner) Validate(_ context.Context, project compose.ProjectRef) error {
	r.validated++
	r.validatedDir, r.validatedCompose, r.validatedEnv = project.Dir, project.ComposeFile, project.EnvFile
	return r.validateError
}
func (r *fakeReconcileRunner) UpSelected(_ context.Context, _ compose.ProjectRef, services ...string) error {
	r.calls = append(r.calls, "selected")
	r.up = append(r.up, append([]string(nil), services...))
	return nil
}
func (r *fakeReconcileRunner) Recreate(_ context.Context, _ compose.ProjectRef, services ...string) error {
	r.recreated = append(r.recreated, services...)
	return r.recreateError
}
func (r *fakeReconcileRunner) RemoveStopped(_ context.Context, project compose.ProjectRef, services ...string) error {
	r.removed = append(r.removed, services...)
	r.removedCompose = project.ComposeFile
	if r.removeError != nil {
		return r.removeError
	}
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
		Database: contracts.DatabaseConfig{Version: "17", MaxConnections: 100},
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func mustReadRuntime(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
