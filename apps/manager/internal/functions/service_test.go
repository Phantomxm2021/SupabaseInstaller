package functions

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestQueueAndRunDeployPersistsOperationWithoutArchiveInDatabase(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	p := contracts.Project{ID: "p-1", Name: "Bee", Slug: "bee", Status: contracts.ProjectStatusRunning, Health: contracts.HealthHealthy, Services: contracts.Services{Functions: true}, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), p, contracts.ProjectConfiguration{General: contracts.GeneralConfig{Domain: "bee.example.com"}, Services: p.Services}); err != nil {
		t.Fatal(err)
	}
	ops := operation.NewService(database, func() string { return "op-1" }, time.Now)
	spool, err := NewSpool(filepath.Join(t.TempDir(), "spool"))
	if err != nil {
		t.Fatal(err)
	}
	client := &serviceProvisionerFake{}
	service := NewService(database, ops, spool, client, time.Now)
	queued, err := service.QueueDeploy(context.Background(), p, "demo", bytes.NewBufferString("zip"))
	if err != nil {
		t.Fatal(err)
	}
	if queued.Status != operation.Queued {
		t.Fatalf("queued status = %s", queued.Status)
	}
	if _, err := service.Run(context.Background(), p, queued); err != nil {
		t.Fatal(err)
	}
	completed, err := ops.Get(context.Background(), queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != operation.Succeeded || client.name != "demo" {
		t.Fatalf("operation/client = %#v/%q", completed, client.name)
	}
	if _, err := spool.Open(queued.ID); err == nil {
		t.Fatal("archive spool remains after terminal operation")
	}
}

type serviceProvisionerFake struct{ name string }

func (f *serviceProvisionerFake) DeployFunction(_ context.Context, _ string, name, _ string, _ io.Reader) (contracts.FunctionDeploymentResult, error) {
	f.name = name
	return contracts.FunctionDeploymentResult{}, nil
}
func (f *serviceProvisionerFake) ListFunctions(context.Context, string) ([]contracts.FunctionSummary, error) {
	return nil, nil
}
func (f *serviceProvisionerFake) RollbackFunction(context.Context, string, string, string) (contracts.FunctionDeploymentResult, error) {
	return contracts.FunctionDeploymentResult{}, nil
}
func (f *serviceProvisionerFake) DeleteFunction(context.Context, string, string, string) (contracts.FunctionDeploymentResult, error) {
	return contracts.FunctionDeploymentResult{}, nil
}
