package runtime

import (
	"context"
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
	backend := NewBackend(root, &recordingRunner{}, staticInspector{})
	err := backend.Lifecycle(context.Background(), contracts.LifecycleRequest{Slug: "bee", Action: contracts.LifecycleDeleteData})
	if err == nil || !strings.Contains(err.Error(), "separate confirmed") {
		t.Fatalf("Lifecycle() error = %v, want destructive-operation rejection", err)
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

type staticInspector struct{}

func (staticInspector) Project(context.Context, health.ProjectRef) (health.Report, error) {
	return health.Report{Health: contracts.HealthHealthy}, nil
}
