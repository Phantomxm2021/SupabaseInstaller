package lifecycle

import (
	"context"
	"fmt"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type Action string

const (
	ActionStart         Action = "START"
	ActionStop          Action = "STOP"
	ActionRestart       Action = "RESTART"
	ActionDeleteRuntime Action = "DELETE_RUNTIME"
	ActionDeleteData    Action = "DELETE_DATA"
)

type Provisioner interface {
	Lifecycle(ctx context.Context, request contracts.LifecycleRequest) error
	Inspect(ctx context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error)
}

type Service struct {
	store       *store.Store
	operations  *operation.Service
	provisioner Provisioner
}

func NewService(store *store.Store, operations *operation.Service, provisioner Provisioner) *Service {
	return &Service{store: store, operations: operations, provisioner: provisioner}
}

func (service *Service) Queue(ctx context.Context, project contracts.Project, action Action, confirmation string) (operation.Operation, error) {
	if action == ActionDeleteData && confirmation != project.Name {
		return operation.Operation{}, fmt.Errorf("exact project name confirmation is required")
	}
	var operationType operation.Type
	switch action {
	case ActionStart:
		operationType = operation.TypeStart
	case ActionStop:
		operationType = operation.TypeStop
	case ActionRestart:
		operationType = operation.TypeRestart
	case ActionDeleteRuntime, ActionDeleteData:
		operationType = operation.TypeDelete
	default:
		return operation.Operation{}, fmt.Errorf("unsupported lifecycle action %q", action)
	}
	return service.operations.Create(ctx, project.ID, operationType)
}

// ForceDelete performs the destructive delete inline. Unlike normal lifecycle
// actions, deletion must not return an accepted background operation while the
// project metadata is still present: callers only receive success after the
// provisioner has removed the runtime/data and the Manager has removed the
// project record.
func (service *Service) ForceDelete(ctx context.Context, project contracts.Project, action Action, confirmation string) error {
	if action != ActionDeleteRuntime && action != ActionDeleteData {
		return fmt.Errorf("unsupported delete action %q", action)
	}
	if action == ActionDeleteData && confirmation != project.Name {
		return fmt.Errorf("exact project name confirmation is required")
	}
	operationID := fmt.Sprintf("force-delete-%s-%d", project.ID, time.Now().UnixNano())
	request := contracts.LifecycleRequest{
		OperationID: operationID, IdempotencyKey: operationID + ":" + string(action), ProjectID: project.ID,
		ProjectName: project.Name, Slug: project.Slug, Action: provisionerAction(action), ConfirmProjectName: confirmation,
	}
	if err := service.provisioner.Lifecycle(ctx, request); err != nil {
		return err
	}
	if action == ActionDeleteData {
		return service.store.DeleteProject(ctx, project.ID)
	}
	return service.store.UpdateProjectStatus(ctx, project.ID, contracts.ProjectStatusStopped, contracts.HealthStopped)
}

func (service *Service) Run(ctx context.Context, project contracts.Project, action Action, queued operation.Operation) (operation.Operation, error) {
	if err := service.operations.Start(ctx, queued.ID); err != nil {
		return queued, err
	}
	_ = service.operations.StartStep(ctx, queued.ID, string(action), 25)
	request := contracts.LifecycleRequest{
		OperationID: queued.ID, IdempotencyKey: queued.ID + ":" + string(action), ProjectID: project.ID,
		ProjectName: project.Name, Slug: project.Slug, Action: provisionerAction(action), ConfirmProjectName: project.Name,
	}
	if err := service.provisioner.Lifecycle(ctx, request); err != nil {
		_ = service.operations.Fail(ctx, queued.ID, string(action), err)
		latest, _ := service.operations.Get(ctx, queued.ID)
		return latest, err
	}
	_ = service.operations.CompleteStep(ctx, queued.ID, string(action), 90)
	status := contracts.ProjectStatusRunning
	healthStatus := contracts.HealthHealthy
	switch action {
	case ActionStop, ActionDeleteRuntime:
		status = contracts.ProjectStatusStopped
		healthStatus = contracts.HealthStopped
	case ActionDeleteData:
		if err := service.store.DeleteProject(ctx, project.ID); err != nil {
			_ = service.operations.Fail(ctx, queued.ID, "DELETE_METADATA", err)
			return queued, err
		}
	default:
		inspection, err := service.provisioner.Inspect(ctx, contracts.InspectProjectRequest{ProjectID: project.ID, Slug: project.Slug})
		if err != nil {
			_ = service.operations.Fail(ctx, queued.ID, "HEALTH_CHECK", err)
			return queued, err
		}
		healthStatus = inspection.Health
	}
	if action != ActionDeleteData {
		if err := service.store.UpdateProjectStatus(ctx, project.ID, status, healthStatus); err != nil {
			_ = service.operations.Fail(ctx, queued.ID, "UPDATE_PROJECT", err)
			return queued, err
		}
	}
	if err := service.operations.Succeed(ctx, queued.ID); err != nil {
		return queued, err
	}
	return service.operations.Get(ctx, queued.ID)
}

func provisionerAction(action Action) contracts.LifecycleAction {
	switch action {
	case ActionStart:
		return contracts.LifecycleStart
	case ActionStop:
		return contracts.LifecycleStop
	case ActionRestart:
		return contracts.LifecycleRestart
	case ActionDeleteRuntime:
		return contracts.LifecycleDeleteRuntime
	case ActionDeleteData:
		return contracts.LifecycleDeleteData
	default:
		return ""
	}
}
