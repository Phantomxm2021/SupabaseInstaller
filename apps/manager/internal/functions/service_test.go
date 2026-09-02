package functions

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/provisioner"
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

func TestRunPersistsClientErrorUnchanged(t *testing.T) {
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
	clientErr := &provisioner.ClientError{Code: "FUNCTION_DEPLOY_FAILED", Message: "function archive requires root index.ts", Status: 422}
	service := NewService(database, ops, spool, &serviceProvisionerFake{err: clientErr}, time.Now)
	queued, err := service.QueueDeploy(context.Background(), p, "demo", bytes.NewBufferString("zip"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Run(context.Background(), p, queued)
	if !errors.Is(err, clientErr) {
		t.Fatalf("Run() error = %v, want ClientError", err)
	}
	stored, err := ops.Get(context.Background(), queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ErrorMessage != clientErr.Error() {
		t.Fatalf("stored error = %q, want %q", stored.ErrorMessage, clientErr.Error())
	}
}

type serviceProvisionerFake struct {
	name string
	err  error
}

func (f *serviceProvisionerFake) DeployFunction(_ context.Context, _ string, name, _ string, _ io.Reader) (contracts.FunctionDeploymentResult, error) {
	f.name = name
	return contracts.FunctionDeploymentResult{}, f.err
}
func (f *serviceProvisionerFake) ListFunctions(context.Context, string) ([]contracts.FunctionSummary, error) {
	return nil, nil
}
func (f *serviceProvisionerFake) RollbackFunction(context.Context, string, string, string) (contracts.FunctionDeploymentResult, error) {
	return contracts.FunctionDeploymentResult{}, f.err
}
func (f *serviceProvisionerFake) DeleteFunction(context.Context, string, string, string) (contracts.FunctionDeploymentResult, error) {
	return contracts.FunctionDeploymentResult{}, f.err
}
