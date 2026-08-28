package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

type LifecycleRunner interface {
	UpDatabase(ctx context.Context, project compose.ProjectRef) error
	VerifyDatabaseBootstrap(ctx context.Context, project compose.ProjectRef) error
	SynchronizeDatabaseRolePasswords(ctx context.Context, project compose.ProjectRef) error
	ResetDatabaseConfig(ctx context.Context, project compose.ProjectRef) error
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
	projectFS                   *projectfs.Root
	runner                      LifecycleRunner
	inspector                   HealthInspector
	acceptanceInspectorFailOnce atomic.Bool
}

const (
	rollbackBudget       = 4 * time.Minute
	rollbackActionBudget = 90 * time.Second
)

func NewBackend(projectFS *projectfs.Root, runner LifecycleRunner, inspector HealthInspector) *Backend {
	return &Backend{projectFS: projectFS, runner: runner, inspector: inspector}
}

// EnableAcceptanceInspectorFailure is only wired by the disposable acceptance
// compose environment. It consumes one candidate inspection and lets the
// normal runtime rollback path run against the real Inspector afterward.
func (backend *Backend) EnableAcceptanceInspectorFailure() {
	backend.acceptanceInspectorFailOnce.Store(true)
}

func (backend *Backend) consumeAcceptanceInspectorFailure() bool {
	return backend.acceptanceInspectorFailOnce.CompareAndSwap(true, false)
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
		if err := backend.runner.VerifyDatabaseBootstrap(ctx, project); err != nil {
			return err
		}
		if err := backend.runner.SynchronizeDatabaseRolePasswords(ctx, project); err != nil {
			return err
		}
		return backend.runner.UpServices(ctx, project, "auth", "auth-templates", "rest", "meta", "studio", "api-gw")
	case contracts.LifecycleStop:
		return backend.runner.Stop(ctx, project)
	case contracts.LifecycleRestart:
		if err := backend.runner.Restart(ctx, project); err != nil {
			return err
		}
		// Compose restart only affects containers that already exist. Ensure the
		// auth template helper is recreated when an older runtime or manual
		// cleanup removed it.
		return backend.runner.UpServices(ctx, project, "auth-templates")
	case contracts.LifecycleDeleteRuntime:
		return backend.runner.DownRuntime(ctx, project)
	case contracts.LifecycleDeleteData:
		metadata, err := backend.projectFS.Metadata(request.Slug)
		metadataMissing := errors.Is(err, projectfs.ErrNotFound)
		if err != nil && !metadataMissing {
			return err
		}
		// The Manager owns the authoritative project record and sends its
		// authenticated name with destructive requests. A failed installation
		// can leave Provisioner metadata without ProjectName, so rejecting against
		// that empty/stale value would make the project impossible to delete.
		expectedName := metadata.ProjectName
		if request.ProjectName != "" {
			expectedName = request.ProjectName
		}
		if metadataMissing && request.ProjectName == "" {
			return fmt.Errorf("project metadata is missing and Manager project name was not provided")
		}
		if request.ConfirmProjectName == "" || request.ConfirmProjectName != expectedName {
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

// HostResources exposes only capacity/usage metrics needed by the Manager
// landing page. No project secrets or rendered files are returned.
func (backend *Backend) HostResources(ctx context.Context) (contracts.HostResources, error) {
	inspector, ok := backend.inspector.(interface {
		HostResources(context.Context, string) (contracts.HostResources, error)
	})
	if !ok {
		return contracts.HostResources{}, fmt.Errorf("host resource inspection is unavailable")
	}
	return inspector.HostResources(ctx, backend.projectFS.BasePath())
}

// HostPortAvailable checks Docker's host-side published ports through the
// provisioner network namespace.
func (backend *Backend) HostPortAvailable(ctx context.Context, port int) (bool, error) {
	inspector, ok := backend.inspector.(interface {
		HostPortAvailable(context.Context, int) (bool, error)
	})
	if !ok {
		return false, fmt.Errorf("host port inspection is unavailable")
	}
	return inspector.HostPortAvailable(ctx, port)
}
