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
	if strings.Join(runner.calls, ",") != "db,services:auth|rest|meta|studio|api-gw" {
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

type recordingRunner struct{ calls []string }

func (runner *recordingRunner) UpDatabase(context.Context, compose.ProjectRef) error {
	runner.calls = append(runner.calls, "db")
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
