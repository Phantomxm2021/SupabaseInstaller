package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

func TestStartRunsDatabaseBeforeDependentServices(t *testing.T) {
	runner := &recordingRunner{}
	root, _ := projectfs.New(t.TempDir())
	backend := NewBackend(root, runner, staticInspector{})

	err := backend.Lifecycle(context.Background(), contracts.LifecycleRequest{Slug: "bee", Action: contracts.LifecycleStart})
	if err != nil {
		t.Fatalf("Lifecycle() error = %v", err)
	}
	if strings.Join(runner.calls, ",") != "db,verify-bootstrap,sync-db-roles,services:auth|rest|meta|studio|api-gw" {
		t.Fatalf("lifecycle calls = %#v", runner.calls)
	}
}

func TestDeleteDataIsRejectedWithoutManagerConfirmedPath(t *testing.T) {
	root, _ := projectfs.New(t.TempDir())
	_, _ = root.UpdateMetadata("bee", func(metadata *projectfs.Metadata) error { metadata.ProjectName = "Bee"; return nil })
	backend := NewBackend(root, &recordingRunner{}, staticInspector{})
	err := backend.Lifecycle(context.Background(), contracts.LifecycleRequest{Slug: "bee", Action: contracts.LifecycleDeleteData, ConfirmProjectName: "bee"})
	if err == nil || !strings.Contains(err.Error(), "exact project name") {
		t.Fatalf("Lifecycle() error = %v, want destructive-operation rejection", err)
	}
}

func TestDeleteDataRemovesOnlyConfirmedContainedProject(t *testing.T) {
	base := t.TempDir()
	root, _ := projectfs.New(base)
	_, _ = root.UpdateMetadata("bee", func(metadata *projectfs.Metadata) error { metadata.ProjectName = "Bee"; return nil })
	if err := os.WriteFile(filepath.Join(base, "bee", "data.marker"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	backend := NewBackend(root, &recordingRunner{}, staticInspector{})

	err := backend.Lifecycle(context.Background(), contracts.LifecycleRequest{Slug: "bee", Action: contracts.LifecycleDeleteData, ConfirmProjectName: "Bee"})
	if err != nil {
		t.Fatalf("Lifecycle() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "bee")); !os.IsNotExist(err) {
		t.Fatalf("project directory still exists or stat failed: %v", err)
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("project root was removed: %v", err)
	}
}

func TestDeleteDataUsesManagerProjectNameWhenMetadataNameIsMissing(t *testing.T) {
	base := t.TempDir()
	root, _ := projectfs.New(base)
	if _, err := root.UpdateMetadata("bee", func(metadata *projectfs.Metadata) error {
		metadata.ProjectID = "project-1"
		metadata.ProjectName = ""
		return nil
	}); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "bee", "data.marker"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	backend := NewBackend(root, &recordingRunner{}, staticInspector{})
	err := backend.Lifecycle(context.Background(), contracts.LifecycleRequest{
		ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", Action: contracts.LifecycleDeleteData, ConfirmProjectName: "Bee",
	})
	if err != nil {
		t.Fatalf("Lifecycle() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "bee")); !os.IsNotExist(err) {
		t.Fatalf("project directory still exists or stat failed: %v", err)
	}
}

func TestDeleteDataAllowsMissingProvisionerMetadataAfterManagerConfirmation(t *testing.T) {
	base := t.TempDir()
	root, _ := projectfs.New(base)
	if err := os.MkdirAll(filepath.Join(base, "bee", "runtime-data"), 0o700); err != nil {
		t.Fatalf("create project data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "bee", "runtime-data", "marker"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	backend := NewBackend(root, &recordingRunner{}, staticInspector{})
	err := backend.Lifecycle(context.Background(), contracts.LifecycleRequest{
		ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", Action: contracts.LifecycleDeleteData, ConfirmProjectName: "Bee",
	})
	if err != nil {
		t.Fatalf("Lifecycle() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "bee")); !os.IsNotExist(err) {
		t.Fatalf("project directory still exists or stat failed: %v", err)
	}
}

type recordingRunner struct{ calls []string }

func (runner *recordingRunner) UpDatabase(context.Context, compose.ProjectRef) error {
	runner.calls = append(runner.calls, "db")
	return nil
}
func (runner *recordingRunner) VerifyDatabaseBootstrap(context.Context, compose.ProjectRef) error {
	runner.calls = append(runner.calls, "verify-bootstrap")
	return nil
}
func (runner *recordingRunner) SynchronizeDatabaseRolePasswords(context.Context, compose.ProjectRef) error {
	runner.calls = append(runner.calls, "sync-db-roles")
	return nil
}
func (runner *recordingRunner) ResetDatabaseConfig(context.Context, compose.ProjectRef) error {
	runner.calls = append(runner.calls, "reset-db-config")
	return nil
}
func (runner *recordingRunner) UpServices(_ context.Context, _ compose.ProjectRef, services ...string) error {
	runner.calls = append(runner.calls, "services:"+strings.Join(services, "|"))
	return nil
}
func (runner *recordingRunner) Stop(context.Context, compose.ProjectRef) error { return nil }
func (runner *recordingRunner) Restart(context.Context, compose.ProjectRef, ...string) error {
	return nil
}
func (runner *recordingRunner) DownRuntime(context.Context, compose.ProjectRef) error { return nil }
func (runner *recordingRunner) Validate(context.Context, compose.ProjectRef) error    { return nil }
func (runner *recordingRunner) UpSelected(_ context.Context, _ compose.ProjectRef, services ...string) error {
	runner.calls = append(runner.calls, "selected:"+strings.Join(services, "|"))
	return nil
}
func (runner *recordingRunner) Recreate(_ context.Context, _ compose.ProjectRef, services ...string) error {
	runner.calls = append(runner.calls, "recreate:"+strings.Join(services, "|"))
	return nil
}
func (runner *recordingRunner) RemoveStopped(_ context.Context, _ compose.ProjectRef, services ...string) error {
	runner.calls = append(runner.calls, "remove:"+strings.Join(services, "|"))
	return nil
}

type staticInspector struct{}

func (staticInspector) Project(context.Context, health.ProjectRef) (health.Report, error) {
	return health.Report{Health: contracts.HealthHealthy}, nil
}
