package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestStopCompletesOperationAndMarksProjectStopped(t *testing.T) {
	service, database, provisioner := newLifecycleService(t)
	project, _ := database.GetProject(context.Background(), "bee")
	queued, err := service.Queue(context.Background(), project, ActionStop, "")
	if err != nil {
		t.Fatalf("Queue() error = %v", err)
	}
	result, err := service.Run(context.Background(), project, ActionStop, queued)
	if err != nil || result.Status != operation.Succeeded || provisioner.lastAction != contracts.LifecycleStop {
		t.Fatalf("Run() = %#v, %v, action=%s", result, err, provisioner.lastAction)
	}
	updated, _ := database.GetProject(context.Background(), "bee")
	if updated.Status != contracts.ProjectStatusStopped || updated.Health != contracts.HealthStopped {
		t.Fatalf("project status = %s/%s", updated.Status, updated.Health)
	}
}

func TestDeleteDataRequiresExactProjectName(t *testing.T) {
	service, database, provisioner := newLifecycleService(t)
	project, _ := database.GetProject(context.Background(), "bee")
	_, err := service.Queue(context.Background(), project, ActionDeleteData, "bee")
	if err == nil {
		t.Fatal("Queue() accepted wrong-case project name")
	}
	if provisioner.lastAction != "" {
		t.Fatalf("Provisioner action = %s, want none", provisioner.lastAction)
	}
}

func TestForceDeleteRemovesProjectMetadataOnlyAfterProvisionerSucceeds(t *testing.T) {
	service, database, provisioner := newLifecycleService(t)
	project, _ := database.GetProject(context.Background(), "bee")
	if err := service.ForceDelete(context.Background(), project, ActionDeleteData, "Bee"); err != nil {
		t.Fatalf("ForceDelete() error = %v", err)
	}
	if provisioner.lastAction != contracts.LifecycleDeleteData {
		t.Fatalf("provisioner action = %s, want delete data", provisioner.lastAction)
	}
	if _, err := database.GetProject(context.Background(), project.ID); err != store.ErrNotFound {
		t.Fatalf("project lookup error = %v, want metadata deleted", err)
	}
}

func TestForceDeleteLeavesMetadataWhenProvisionerFails(t *testing.T) {
	service, database, provisioner := newLifecycleService(t)
	project, _ := database.GetProject(context.Background(), "bee")
	provisioner.err = errors.New("runtime removal failed")
	if err := service.ForceDelete(context.Background(), project, ActionDeleteData, "Bee"); err == nil {
		t.Fatal("ForceDelete() succeeded despite provisioner failure")
	}
	if _, err := database.GetProject(context.Background(), project.ID); err != nil {
		t.Fatalf("project metadata was not preserved: %v", err)
	}
}

type fakeProvisioner struct {
	lastAction contracts.LifecycleAction
	err        error
}

func (fake *fakeProvisioner) Lifecycle(_ context.Context, request contracts.LifecycleRequest) error {
	fake.lastAction = request.Action
	return fake.err
}
func (fake *fakeProvisioner) Inspect(_ context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{ProjectID: request.ProjectID, Health: contracts.HealthHealthy}, nil
}

func newLifecycleService(t *testing.T) (*Service, *store.Store, *fakeProvisioner) {
	t.Helper()
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	t.Cleanup(func() { _ = database.Close() })
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", Status: contracts.ProjectStatusRunning, Health: contracts.HealthHealthy, SupabaseVersion: "self-hosted/v0.8.0", Preset: contracts.PresetLightweight, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_ = database.CreateProject(context.Background(), project, contracts.ProjectConfiguration{General: contracts.GeneralConfig{Domain: project.Domain, SiteURL: project.SiteURL, SupabaseVersion: project.SupabaseVersion}, Services: project.Services})
	operations := operation.NewService(database, func() string { return "op-1" }, time.Now)
	provisioner := &fakeProvisioner{}
	return NewService(database, operations, provisioner), database, provisioner
}
