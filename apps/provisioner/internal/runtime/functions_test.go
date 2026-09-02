package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

func TestFunctionServiceDeployRestartsOnlyFunctions(t *testing.T) {
	releases := &functionReleaseFake{stage: projectfs.FunctionReleaseStage{Name: "demo", OperationID: "op-1", SHA256: "abc"}}
	runner := &functionRunnerFake{}
	service := NewFunctionService(releases, runner)
	_, err := service.Deploy(context.Background(), compose.ProjectRef{Slug: "bee"}, contracts.DeployFunctionRequest{Name: "demo", OperationID: "op-1"}, bytes.NewBufferString("zip"))
	if err != nil {
		t.Fatalf("Deploy() error = %v", err)
	}
	if len(runner.services) != 1 || runner.services[0] != "functions" {
		t.Fatalf("restart services = %v, want [functions]", runner.services)
	}
}

func TestFunctionServiceDeployRestoresReleaseWhenRestartFails(t *testing.T) {
	releases := &functionReleaseFake{stage: projectfs.FunctionReleaseStage{Name: "demo", OperationID: "op-1", SHA256: "abc"}}
	runner := &functionRunnerFake{failFirst: true}
	service := NewFunctionService(releases, runner)
	result, err := service.Deploy(context.Background(), compose.ProjectRef{Slug: "bee"}, contracts.DeployFunctionRequest{Name: "demo", OperationID: "op-1"}, bytes.NewBufferString("zip"))
	if err == nil {
		t.Fatal("Deploy() succeeded, want restart error")
	}
	if !result.RolledBack || !releases.restored {
		t.Fatalf("result = %#v restored=%v, want compensated rollback", result, releases.restored)
	}
	if len(runner.services) != 2 {
		t.Fatalf("restart count = %d, want 2", len(runner.services))
	}
}

func TestFunctionServiceDeployPreservesRestartAndRestoreCauses(t *testing.T) {
	restartErr := errors.New("compose restart output: function failed to bind")
	restoreErr := errors.New("release restore output: rename failed")
	releases := &functionReleaseFake{stage: projectfs.FunctionReleaseStage{Name: "demo", OperationID: "op-1", SHA256: "abc"}, restoreErr: restoreErr}
	runner := &functionRunnerFake{restartErrors: []error{restartErr}}
	service := NewFunctionService(releases, runner)
	_, err := service.Deploy(context.Background(), compose.ProjectRef{Slug: "bee"}, contracts.DeployFunctionRequest{Name: "demo", OperationID: "op-1"}, bytes.NewBufferString("zip"))
	if err == nil || !errors.Is(err, restartErr) || !errors.Is(err, restoreErr) {
		t.Fatalf("Deploy() error = %v, want restart and release restore causes", err)
	}
}

func TestFunctionServiceRollbackRestartsOnlyFunctions(t *testing.T) {
	releases := &functionReleaseFake{}
	runner := &functionRunnerFake{}
	service := NewFunctionService(releases, runner)
	_, err := service.Rollback(context.Background(), compose.ProjectRef{Slug: "bee"}, contracts.FunctionOperationRequest{Name: "demo", OperationID: "op-3"})
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if !releases.rolledBack || len(runner.services) != 1 || runner.services[0] != "functions" {
		t.Fatalf("rolledBack/services = %v/%v", releases.rolledBack, runner.services)
	}
}

type functionReleaseFake struct {
	stage      projectfs.FunctionReleaseStage
	restored   bool
	rolledBack bool
	restoreErr error
}

func (f *functionReleaseFake) StageFunctionRelease(string, string, string, io.Reader) (projectfs.FunctionReleaseStage, error) {
	return f.stage, nil
}
func (f *functionReleaseFake) ActivateFunctionRelease(string, string, projectfs.FunctionReleaseStage) (projectfs.FunctionActivation, error) {
	return projectfs.FunctionActivation{Current: &contracts.FunctionRelease{SHA256: "abc"}}, nil
}
func (f *functionReleaseFake) RestoreFunctionRelease(string, string, projectfs.FunctionActivation) error {
	f.restored = true
	return f.restoreErr
}
func (f *functionReleaseFake) RollbackFunctionRelease(string, string, string) (projectfs.FunctionActivation, error) {
	f.rolledBack = true
	return projectfs.FunctionActivation{}, nil
}
func (f *functionReleaseFake) DeleteFunction(string, string) (projectfs.FunctionActivation, error) {
	return projectfs.FunctionActivation{}, nil
}

type functionRunnerFake struct {
	services      []string
	failFirst     bool
	restartErrors []error
}

func (f *functionRunnerFake) Restart(_ context.Context, _ compose.ProjectRef, services ...string) error {
	f.services = append(f.services, services...)
	if len(f.restartErrors) > 0 {
		err := f.restartErrors[0]
		f.restartErrors = f.restartErrors[1:]
		return err
	}
	if f.failFirst && len(f.services) == 1 {
		return context.DeadlineExceeded
	}
	return nil
}
