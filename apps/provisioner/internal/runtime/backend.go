package runtime

import (
	"context"
	"fmt"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

type LifecycleRunner interface {
	UpDatabase(ctx context.Context, project compose.ProjectRef) error
	UpServices(ctx context.Context, project compose.ProjectRef, services ...string) error
	Stop(ctx context.Context, project compose.ProjectRef) error
	Restart(ctx context.Context, project compose.ProjectRef, services ...string) error
	DownRuntime(ctx context.Context, project compose.ProjectRef) error
	Validate(ctx context.Context, project compose.ProjectRef) error
	UpSelected(ctx context.Context, project compose.ProjectRef, services ...string) error
	Recreate(ctx context.Context, project compose.ProjectRef, services ...string) error
	RemoveStopped(ctx context.Context, project compose.ProjectRef, services ...string) error
}

type HealthInspector interface {
	Project(ctx context.Context, project health.ProjectRef) (health.Report, error)
}

type Backend struct {
	projectFS *projectfs.Root
	runner    LifecycleRunner
	inspector HealthInspector
}

func NewBackend(projectFS *projectfs.Root, runner LifecycleRunner, inspector HealthInspector) *Backend {
	return &Backend{projectFS: projectFS, runner: runner, inspector: inspector}
}

func (backend *Backend) Lifecycle(ctx context.Context, request contracts.LifecycleRequest) error {
	runtime, err := backend.projectFS.CurrentRuntimeFiles(request.Slug)
	if err != nil {
		return err
	}
	project := compose.ProjectRef{Slug: request.Slug, Dir: runtime.ProjectDir, ComposeFile: runtime.ComposeFile, EnvFile: runtime.EnvFile}
	switch request.Action {
	case contracts.LifecycleStart:
		if err := backend.runner.UpDatabase(ctx, project); err != nil {
			return err
		}
		return backend.runner.UpServices(ctx, project, "auth", "rest", "meta", "studio", "api-gw")
	case contracts.LifecycleStop:
		return backend.runner.Stop(ctx, project)
	case contracts.LifecycleRestart:
		return backend.runner.Restart(ctx, project)
	case contracts.LifecycleDeleteRuntime:
		return backend.runner.DownRuntime(ctx, project)
	case contracts.LifecycleDeleteData:
		metadata, err := backend.projectFS.Metadata(request.Slug)
		if err != nil {
			return err
		}
		if request.ConfirmProjectName == "" || request.ConfirmProjectName != metadata.ProjectName {
			return fmt.Errorf("exact project name confirmation is required")
		}
		if err := backend.runner.DownRuntime(ctx, project); err != nil {
			return err
		}
		return backend.projectFS.DeleteProjectData(request.Slug)
	default:
		return fmt.Errorf("unsupported lifecycle action %q", request.Action)
	}
}

func (backend *Backend) Inspect(ctx context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	enabled := request.EnabledServices
	if len(enabled) == 0 {
		enabled = []string{"db", "auth", "rest", "meta", "studio", "api-gw"}
	}
	report, err := backend.inspector.Project(ctx, health.ProjectRef{Slug: request.Slug, Enabled: enabled})
	if err != nil {
		return contracts.InspectProjectResponse{}, err
	}
	return contracts.InspectProjectResponse{ProjectID: request.ProjectID, Health: report.Health, Services: report.Services, CheckedAt: report.CheckedAt}, nil
}
