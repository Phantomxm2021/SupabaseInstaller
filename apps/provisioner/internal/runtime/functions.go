package runtime

import (
	"context"
	"fmt"
	"io"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

type FunctionReleaseStore interface {
	StageFunctionRelease(slug, name, operationID string, archive io.Reader) (projectfs.FunctionReleaseStage, error)
	ActivateFunctionRelease(slug, name string, stage projectfs.FunctionReleaseStage) (projectfs.FunctionActivation, error)
	RestoreFunctionRelease(slug, name string, activation projectfs.FunctionActivation) error
	RollbackFunctionRelease(slug, name, operationID string) (projectfs.FunctionActivation, error)
	DeleteFunction(slug, name string) (projectfs.FunctionActivation, error)
}

func (s *FunctionService) Delete(ctx context.Context, project compose.ProjectRef, request contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error) {
	if s.releases == nil || s.runner == nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("functions deployment is unavailable")
	}
	if err := contracts.ValidateFunctionName(request.Name); err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	activation, err := s.releases.DeleteFunction(project.Slug, request.Name)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	if err := s.runner.Restart(ctx, project, "functions"); err != nil {
		return contracts.FunctionDeploymentResult{Previous: activation.Previous}, fmt.Errorf("restart functions: %w", err)
	}
	return contracts.FunctionDeploymentResult{Previous: activation.Previous}, nil
}

func (s *FunctionService) Rollback(ctx context.Context, project compose.ProjectRef, request contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error) {
	if s.releases == nil || s.runner == nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("functions deployment is unavailable")
	}
	if err := contracts.ValidateFunctionName(request.Name); err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	activation, err := s.releases.RollbackFunctionRelease(project.Slug, request.Name, request.OperationID)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	if err := s.runner.Restart(ctx, project, "functions"); err != nil {
		return contracts.FunctionDeploymentResult{Current: activation.Current, Previous: activation.Previous}, fmt.Errorf("restart functions: %w", err)
	}
	return contracts.FunctionDeploymentResult{Current: activation.Current, Previous: activation.Previous}, nil
}

type FunctionRunner interface {
	Restart(context.Context, compose.ProjectRef, ...string) error
}

type FunctionService struct {
	releases FunctionReleaseStore
	runner   FunctionRunner
}

func NewFunctionService(releases FunctionReleaseStore, runner FunctionRunner) *FunctionService {
	return &FunctionService{releases: releases, runner: runner}
}

func (s *FunctionService) Deploy(ctx context.Context, project compose.ProjectRef, request contracts.DeployFunctionRequest, archive io.Reader) (contracts.FunctionDeploymentResult, error) {
	if s.releases == nil || s.runner == nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("functions deployment is unavailable")
	}
	if err := contracts.ValidateFunctionName(request.Name); err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	stage, err := s.releases.StageFunctionRelease(project.Slug, request.Name, request.OperationID, archive)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	activation, err := s.releases.ActivateFunctionRelease(project.Slug, request.Name, stage)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	result := contracts.FunctionDeploymentResult{Current: activation.Current, Previous: activation.Previous}
	if err := s.runner.Restart(ctx, project, "functions"); err == nil {
		return result, nil
	} else {
		if restoreErr := s.releases.RestoreFunctionRelease(project.Slug, request.Name, activation); restoreErr != nil {
			return result, fmt.Errorf("restart functions: %w; rollback release: %v", err, restoreErr)
		}
		if restartErr := s.runner.Restart(ctx, project, "functions"); restartErr != nil {
			return result, fmt.Errorf("restart functions: %w; restart rollback: %v", err, restartErr)
		}
		result.RolledBack = true
		return result, fmt.Errorf("restart functions: %w", err)
	}
}
