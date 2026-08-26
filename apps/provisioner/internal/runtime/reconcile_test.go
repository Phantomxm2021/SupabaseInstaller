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
	if result.RolledBack {
		t.Fatal("failed reconcile returned a success result")
	}
}

type fakeReconcileRunner struct {
	validated int
	up        [][]string
	recreated []string
	removed   []string
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
func (r *fakeReconcileRunner) Validate(context.Context, compose.ProjectRef) error {
	r.validated++
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
func (r *fakeReconcileRunner) RemoveStopped(_ context.Context, _ compose.ProjectRef, services ...string) error {
	r.removed = append(r.removed, services...)
	return nil
}

type sequenceInspector struct{ reports []health.Report }

func (i *sequenceInspector) Project(context.Context, health.ProjectRef) (health.Report, error) {
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
