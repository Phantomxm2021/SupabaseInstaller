package operation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"supabase-manager/apps/manager/internal/store"
)

var ErrInvalidTransition = errors.New("invalid operation transition")

type Service struct {
	store *store.Store
	id    func() string
	now   func() time.Time
}

// NewQueuedOperation allocates the stable identity used by Store admission.
// It does not write anything; callers pass the returned value to the atomic
// admission transaction.
func (s *Service) NewQueuedOperation(projectID string, operationType Type) (Operation, error) {
	op := Operation{ID: s.id(), ProjectID: projectID, Type: operationType, Status: Queued, CreatedAt: s.now()}
	if op.ID == "" {
		return Operation{}, fmt.Errorf("operation ID generator returned an empty ID")
	}
	return op, nil
}

func NewService(store *store.Store, id func() string, now func() time.Time) *Service {
	return &Service{store: store, id: id, now: now}
}

func (s *Service) Create(ctx context.Context, projectID string, operationType Type) (Operation, error) {
	operation, err := s.NewQueuedOperation(projectID, operationType)
	if err != nil {
		return Operation{}, err
	}
	if err := s.store.CreateOperation(ctx, operation, "OPERATION_QUEUED", json.RawMessage(`{"status":"QUEUED"}`)); err != nil {
		return Operation{}, err
	}
	return operation, nil
}

func (s *Service) Start(ctx context.Context, id string) error {
	operation, err := s.requireStatus(ctx, id, Queued)
	if err != nil {
		return err
	}
	now := s.now()
	return s.update(ctx, operation, store.OperationUpdate{Status: Running, StartedAt: &now, EventType: "OPERATION_STARTED", EventPayload: json.RawMessage(`{"status":"RUNNING"}`), EventTime: now})
}

func (s *Service) StartStep(ctx context.Context, id, step string, progress int) error {
	operation, err := s.requireStatus(ctx, id, Running)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"step": step, "progress": progress, "status": "RUNNING"})
	return s.update(ctx, operation, store.OperationUpdate{Status: Running, CurrentStep: step, Progress: progress, EventType: "STEP_STARTED", EventPayload: payload, EventTime: s.now()})
}

func (s *Service) CompleteStep(ctx context.Context, id, step string, progress int) error {
	operation, err := s.requireStatus(ctx, id, Running)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"step": step, "progress": progress, "status": "SUCCEEDED"})
	return s.update(ctx, operation, store.OperationUpdate{Status: Running, CurrentStep: step, Progress: progress, EventType: "STEP_COMPLETED", EventPayload: payload, EventTime: s.now()})
}

func (s *Service) Fail(ctx context.Context, id, step string, cause error) error {
	operation, err := s.requireStatus(ctx, id, Running)
	if err != nil {
		return err
	}
	now := s.now()
	payload, _ := json.Marshal(map[string]any{"step": step, "code": "OPERATION_FAILED", "message": cause.Error()})
	return s.update(ctx, operation, store.OperationUpdate{Status: Failed, CurrentStep: step, Progress: operation.Progress, ErrorCode: "OPERATION_FAILED", ErrorMessage: cause.Error(), FinishedAt: &now, EventType: "OPERATION_FAILED", EventPayload: payload, EventTime: now})
}

func (s *Service) BeginRollback(ctx context.Context, id string) error {
	operation, err := s.requireStatus(ctx, id, Failed)
	if err != nil {
		return err
	}
	now := s.now()
	return s.update(ctx, operation, store.OperationUpdate{Status: RollingBack, CurrentStep: operation.CurrentStep, Progress: operation.Progress, ErrorCode: operation.ErrorCode, ErrorMessage: operation.ErrorMessage, StartedAt: operation.StartedAt, EventType: "ROLLBACK_STARTED", EventPayload: json.RawMessage(`{"status":"ROLLING_BACK"}`), EventTime: now})
}

func (s *Service) CompleteRollback(ctx context.Context, id string) error {
	operation, err := s.requireStatus(ctx, id, RollingBack)
	if err != nil {
		return err
	}
	now := s.now()
	return s.update(ctx, operation, store.OperationUpdate{Status: RolledBack, CurrentStep: operation.CurrentStep, Progress: operation.Progress, ErrorCode: operation.ErrorCode, ErrorMessage: operation.ErrorMessage, StartedAt: operation.StartedAt, FinishedAt: &now, EventType: "ROLLBACK_COMPLETED", EventPayload: json.RawMessage(`{"status":"ROLLED_BACK"}`), EventTime: now})
}

func (s *Service) Succeed(ctx context.Context, id string) error {
	operation, err := s.requireStatus(ctx, id, Running)
	if err != nil {
		return err
	}
	now := s.now()
	return s.update(ctx, operation, store.OperationUpdate{Status: Succeeded, CurrentStep: operation.CurrentStep, Progress: 100, StartedAt: operation.StartedAt, FinishedAt: &now, EventType: "OPERATION_SUCCEEDED", EventPayload: json.RawMessage(`{"status":"SUCCEEDED","progress":100}`), EventTime: now})
}

func (s *Service) EventsAfter(ctx context.Context, id string, sequence int64) ([]Event, error) {
	return s.store.OperationEventsAfter(ctx, id, sequence)
}

func (s *Service) Get(ctx context.Context, id string) (Operation, error) {
	return s.store.GetOperation(ctx, id)
}

func (s *Service) ListActive(ctx context.Context, typ Type) ([]Operation, error) {
	return s.store.ListActiveOperations(ctx, typ)
}

func (s *Service) requireStatus(ctx context.Context, id string, status Status) (Operation, error) {
	operation, err := s.store.GetOperation(ctx, id)
	if err != nil {
		return Operation{}, err
	}
	if operation.Status != status {
		return Operation{}, fmt.Errorf("%w: %s to requested state", ErrInvalidTransition, operation.Status)
	}
	return operation, nil
}

func (s *Service) update(ctx context.Context, previous Operation, update store.OperationUpdate) error {
	if update.StartedAt == nil {
		update.StartedAt = previous.StartedAt
	}
	return s.store.UpdateOperation(ctx, previous.ID, update)
}
