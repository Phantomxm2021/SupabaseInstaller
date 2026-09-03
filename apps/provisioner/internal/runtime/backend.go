package runtime

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/officialtemplate"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/proxy"
	"supabase-manager/internal/contracts"
)

type LifecycleRunner interface {
	Pull(ctx context.Context, project compose.ProjectRef) error
	UpDatabase(ctx context.Context, project compose.ProjectRef) error
	StorageObjectCount(ctx context.Context, project compose.ProjectRef) (int64, error)
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
	RecreateRuntime(ctx context.Context, project compose.ProjectRef) error
	RemoveStopped(ctx context.Context, project compose.ProjectRef, services ...string) error
}

type HealthInspector interface {
	Project(ctx context.Context, project health.ProjectRef) (health.Report, error)
}

type templateSource interface {
	Resolve(context.Context, string, bool) (officialtemplate.Snapshot, error)
}

// testTemplateSourceFactory is nil in production. Runtime package tests inject
// a local fixture through it, so tests never make a network call.
var testTemplateSourceFactory func(*projectfs.Root) templateSource

type Backend struct {
	projectFS                   *projectfs.Root
	runner                      LifecycleRunner
	inspector                   HealthInspector
	proxy                       proxy.Client
	acceptanceInspectorFailOnce atomic.Bool
	functions                   *FunctionService
	templates                   templateSource
	provisionerImageRef         string
}

// SetProvisionerImageRef supplies the exact image running this process for
// project-local collector services. It is deployment configuration, not a
// user-controlled project value.
func (backend *Backend) SetProvisionerImageRef(image string) {
	backend.provisionerImageRef = image
}

const (
	rollbackBudget       = 4 * time.Minute
	rollbackActionBudget = 90 * time.Second
)

func NewBackend(projectFS *projectfs.Root, runner LifecycleRunner, inspector HealthInspector, proxyClients ...proxy.Client) *Backend {
	proxyClient := proxy.Client(proxy.DisabledClient{})
	if len(proxyClients) > 0 && proxyClients[0] != nil {
		proxyClient = proxyClients[0]
	}
	var templates templateSource
	if testTemplateSourceFactory != nil {
		templates = testTemplateSourceFactory(projectFS)
	} else {
		var err error
		templates, err = officialtemplate.New(filepath.Join(projectFS.BasePath(), ".manager-template-cache"), nil)
		if err != nil {
			panic(err)
		}
	}
	backend := &Backend{projectFS: projectFS, runner: runner, inspector: inspector, proxy: proxyClient, functions: NewFunctionService(projectFS, runner), templates: templates}
	if testTemplateSourceFactory != nil {
		backend.provisionerImageRef = "supabase-provisioner:test"
	}
	return backend
}

// DeployFunction is the only runtime entry point used by the Provisioner HTTP
// boundary. It derives Compose paths from the validated project slug and never
// accepts a caller-controlled Docker command or directory.
func (backend *Backend) DeployFunction(ctx context.Context, request contracts.DeployFunctionRequest) (contracts.FunctionDeploymentResult, error) {
	if request.Slug == "" {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("server slug is required")
	}
	if request.Archive == nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("function archive is required")
	}
	if metadata, metadataErr := backend.projectFS.Metadata(request.Slug); metadataErr == nil && len(metadata.EnabledServices) > 0 {
		enabled := false
		for _, service := range metadata.EnabledServices {
			if service == "functions" {
				enabled = true
				break
			}
		}
		if !enabled {
			return contracts.FunctionDeploymentResult{}, fmt.Errorf("functions service is disabled")
		}
	}
	runtime, err := backend.projectFS.CurrentRuntimeFiles(request.Slug)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	project := compose.ProjectRef{Slug: request.Slug, Dir: runtime.ProjectDir, ComposeFile: runtime.ComposeFile, EnvFile: runtime.EnvFile}
	return backend.functions.Deploy(ctx, project, request, request.Archive)
}

func (backend *Backend) ListFunctions(ctx context.Context, request contracts.FunctionOperationRequest) ([]contracts.FunctionSummary, error) {
	if request.Slug == "" {
		return nil, fmt.Errorf("server slug is required")
	}
	return backend.projectFS.ListFunctions(request.Slug)
}

func (backend *Backend) RollbackFunction(ctx context.Context, request contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error) {
	if request.Slug == "" {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("server slug is required")
	}
	runtime, err := backend.projectFS.CurrentRuntimeFiles(request.Slug)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	project := compose.ProjectRef{Slug: request.Slug, Dir: runtime.ProjectDir, ComposeFile: runtime.ComposeFile, EnvFile: runtime.EnvFile}
	return backend.functions.Rollback(ctx, project, request)
}

func (backend *Backend) DeleteFunction(ctx context.Context, request contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error) {
	if request.Slug == "" {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("server slug is required")
	}
	runtime, err := backend.projectFS.CurrentRuntimeFiles(request.Slug)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	project := compose.ProjectRef{Slug: request.Slug, Dir: runtime.ProjectDir, ComposeFile: runtime.ComposeFile, EnvFile: runtime.EnvFile}
	return backend.functions.Delete(ctx, project, request)
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
			return fmt.Errorf("server metadata is missing and Manager server name was not provided")
		}
		if request.ConfirmProjectName == "" || request.ConfirmProjectName != expectedName {
			return fmt.Errorf("exact server name confirmation is required")
		}
		if err := backend.runner.DownRuntime(ctx, project); err != nil {
			return err
		}
		if err := backend.proxy.Remove(ctx, request.Slug); err != nil {
			return fmt.Errorf("remove managed nginx site: %w", err)
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
