package functions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/provisioner"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type Provisioner interface {
	provisioner.FunctionsClient
}

type Service struct {
	store       *store.Store
	operations  *operation.Service
	spool       *Spool
	provisioner Provisioner
	now         func() time.Time
	locks       sync.Map
}

func NewService(database *store.Store, operations *operation.Service, spool *Spool, client Provisioner, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: database, operations: operations, spool: spool, provisioner: client, now: now}
}

func (s *Service) QueueDeploy(ctx context.Context, p contracts.Project, name string, archive io.Reader) (operation.Operation, error) {
	if err := contracts.ValidateFunctionName(name); err != nil {
		return operation.Operation{}, err
	}
	if !p.Services.Functions {
		return operation.Operation{}, errors.New("functions service is disabled")
	}
	if s.spool == nil || s.store == nil || s.operations == nil {
		return operation.Operation{}, errors.New("functions service is unavailable")
	}
	op, err := s.operations.NewQueuedOperation(p.ID, operation.Type(contracts.OperationDeployFunction))
	if err != nil {
		return operation.Operation{}, err
	}
	path, hash, err := s.spool.Store(op.ID, archive)
	if err != nil {
		return operation.Operation{}, err
	}
	_, _ = path, hash
	admitted, err := s.store.AdmitFunctionOperation(ctx, op, name, hash)
	if err != nil {
		_ = s.spool.Remove(op.ID)
		return operation.Operation{}, err
	}
	if admitted.ID != op.ID {
		_ = s.spool.Remove(op.ID)
		return admitted, nil
	}
	return op, nil
}

func (s *Service) QueueRollback(ctx context.Context, p contracts.Project, name string) (operation.Operation, error) {
	if !p.Services.Functions {
		return operation.Operation{}, errors.New("functions service is disabled")
	}
	return s.queueAction(ctx, p, name, contracts.OperationRollbackFunction)
}

func (s *Service) QueueDelete(ctx context.Context, p contracts.Project, name, confirmation string) (operation.Operation, error) {
	if !p.Services.Functions {
		return operation.Operation{}, errors.New("functions service is disabled")
	}
	if confirmation != name {
		return operation.Operation{}, errors.New("exact function name confirmation is required")
	}
	return s.queueAction(ctx, p, name, contracts.OperationDeleteFunction)
}

func (s *Service) List(ctx context.Context, p contracts.Project) ([]contracts.FunctionSummary, error) {
	if s.provisioner == nil {
		return nil, errors.New("functions service is unavailable")
	}
	return s.provisioner.ListFunctions(ctx, p.Slug)
}

func (s *Service) queueAction(ctx context.Context, p contracts.Project, name string, typ contracts.OperationType) (operation.Operation, error) {
	if err := contracts.ValidateFunctionName(name); err != nil {
		return operation.Operation{}, err
	}
	if s.store == nil || s.operations == nil {
		return operation.Operation{}, errors.New("functions service is unavailable")
	}
	op, err := s.operations.NewQueuedOperation(p.ID, operation.Type(typ))
	if err != nil {
		return operation.Operation{}, err
	}
	admitted, err := s.store.AdmitFunctionOperation(ctx, op, name, "")
	if err != nil {
		return operation.Operation{}, err
	}
	return admitted, nil
}

func (s *Service) Run(ctx context.Context, p contracts.Project, queued operation.Operation) (operation.Operation, error) {
	if s.operations == nil || s.provisioner == nil || s.spool == nil {
		return queued, errors.New("functions service is unavailable")
	}
	command, err := s.store.GetFunctionCommand(ctx, queued.ID)
	if err != nil {
		return queued, err
	}
	if queued.Status == operation.Queued {
		if err := s.operations.Start(ctx, queued.ID); err != nil {
			return queued, err
		}
	}
	_ = s.operations.StartStep(ctx, queued.ID, "VALIDATING_ARCHIVE", 20)
	_ = s.operations.CompleteStep(ctx, queued.ID, "VALIDATING_ARCHIVE", 20)
	_ = s.operations.StartStep(ctx, queued.ID, "STAGING_RELEASE", 45)
	var runErr error
	var rolledBack bool
	switch queued.Type {
	case operation.Type(contracts.OperationDeployFunction):
		file, openErr := s.spool.Open(queued.ID)
		if openErr != nil {
			runErr = openErr
		} else {
			_ = s.operations.CompleteStep(ctx, queued.ID, "STAGING_RELEASE", 45)
			_ = s.operations.StartStep(ctx, queued.ID, "ACTIVATING_RELEASE", 70)
			_, runErr = s.provisioner.DeployFunction(ctx, p.Slug, command.Name, queued.ID, file)
			_ = file.Close()
			if runErr == nil {
				_ = s.operations.CompleteStep(ctx, queued.ID, "ACTIVATING_RELEASE", 70)
				_ = s.operations.StartStep(ctx, queued.ID, "RESTARTING_FUNCTIONS", 90)
			}
		}
	case operation.Type(contracts.OperationRollbackFunction):
		_ = s.operations.CompleteStep(ctx, queued.ID, "STAGING_RELEASE", 45)
		_ = s.operations.StartStep(ctx, queued.ID, "RESTARTING_FUNCTIONS", 90)
		_, runErr = s.provisioner.RollbackFunction(ctx, p.Slug, command.Name, queued.ID)
	case operation.Type(contracts.OperationDeleteFunction):
		_ = s.operations.CompleteStep(ctx, queued.ID, "STAGING_RELEASE", 45)
		_ = s.operations.StartStep(ctx, queued.ID, "RESTARTING_FUNCTIONS", 90)
		_, runErr = s.provisioner.DeleteFunction(ctx, p.Slug, command.Name, queued.ID)
	default:
		runErr = fmt.Errorf("unsupported function operation %q", queued.Type)
	}
	if s.spool != nil {
		_ = s.spool.Remove(queued.ID)
	}
	if runErr != nil {
		var outcome interface{ RollbackSucceeded() bool }
		if errors.As(runErr, &outcome) {
			rolledBack = outcome.RollbackSucceeded()
		}
		if failErr := s.operations.Fail(ctx, queued.ID, "DEPLOY_FUNCTION", runErr); failErr != nil {
			return queued, failErr
		}
		if rolledBack && s.operations.BeginRollback(ctx, queued.ID) == nil {
			_ = s.operations.CompleteRollback(ctx, queued.ID)
		}
		current, _ := s.operations.Get(ctx, queued.ID)
		return current, runErr
	}
	_ = s.operations.CompleteStep(ctx, queued.ID, "DEPLOY_FUNCTION", 100)
	if err := s.operations.Succeed(ctx, queued.ID); err != nil {
		return queued, err
	}
	current, _ := s.operations.Get(ctx, queued.ID)
	return current, nil
}

func (s *Service) Resume(ctx context.Context, getProject func(context.Context, string) (contracts.Project, error)) error {
	if s.operations == nil {
		return nil
	}
	for _, typ := range []operation.Type{operation.Type(contracts.OperationDeployFunction), operation.Type(contracts.OperationRollbackFunction), operation.Type(contracts.OperationDeleteFunction)} {
		items, err := s.operations.ListActive(ctx, typ)
		if err != nil {
			return err
		}
		for _, item := range items {
			p, err := getProject(ctx, item.ProjectID)
			if err != nil {
				continue
			}
			if _, err := s.Run(ctx, p, item); err != nil { /* durable operation records the failure */
			}
		}
	}
	return nil
}
