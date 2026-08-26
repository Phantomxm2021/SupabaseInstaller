package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

type PasswordRotator interface {
	RotateDatabasePassword(context.Context, compose.ProjectRef, string, string) error
}

// RotateDatabasePassword changes PostgreSQL first, then recreates and health
// checks every enabled dependent service. A failed health check attempts the
// inverse role change and dependent restart before reporting rollback status.
func (backend *Backend) RotateDatabasePassword(ctx context.Context, request contracts.RotateDatabasePasswordRequest) (contracts.RotateDatabasePasswordResponse, error) {
	var result contracts.RotateDatabasePasswordResponse
	metadata, err := backend.projectFS.UpdateMetadata(request.Slug, func(metadata *projectfs.Metadata) error {
		if raw, ok := metadata.Idempotency[request.IdempotencyKey]; ok {
			return json.Unmarshal(raw, &result)
		}
		if metadata.ProjectID != request.ProjectID {
			return fmt.Errorf("project metadata mismatch")
		}
		if metadata.Revision != request.ExpectedRevision || request.NextRevision <= request.ExpectedRevision || request.OldPassword == "" || request.NewPassword == "" || request.OldPassword == request.NewPassword {
			return contracts.ErrInvalidReconcileRevision
		}
		rotator, ok := backend.runner.(PasswordRotator)
		if !ok {
			return &contracts.ReconcileFailure{Cause: errors.New("rotation unavailable"), RollbackSucceeded: false}
		}
		current, err := backend.projectFS.CurrentRuntimeFiles(request.Slug)
		if err != nil {
			return err
		}
		ref := compose.ProjectRef{Slug: request.Slug, Dir: current.ProjectDir, ComposeFile: current.ComposeFile, EnvFile: current.EnvFile}
		services := without(enabledServices(metadata.Configuration), "db")
		if err := rotator.RotateDatabasePassword(ctx, ref, request.OldPassword, request.NewPassword); err != nil {
			return &contracts.ReconcileFailure{Cause: errors.New("database password update failed"), RollbackSucceeded: false}
		}
		rollback := func() error {
			if err := rotator.RotateDatabasePassword(ctx, ref, request.NewPassword, request.OldPassword); err != nil {
				return err
			}
			if err := backend.runner.Recreate(ctx, ref, services...); err != nil {
				return err
			}
			return backend.waitHealthy(ctx, request.Slug, enabledServices(metadata.Configuration))
		}
		if err := backend.runner.Recreate(ctx, ref, services...); err != nil {
			_ = rollback()
			return &contracts.ReconcileFailure{Cause: errors.New("dependent service restart failed"), RollbackSucceeded: false}
		}
		if err := backend.waitHealthy(ctx, request.Slug, enabledServices(metadata.Configuration)); err != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: false}
			}
			return &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: true}
		}
		result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.NextRevision}
		raw, _ := json.Marshal(result)
		metadata.Idempotency[request.IdempotencyKey] = raw
		return nil
	})
	if errors.Is(err, contracts.ErrStaleConfigRevision) || errors.Is(err, contracts.ErrInvalidReconcileRevision) {
		return contracts.RotateDatabasePasswordResponse{}, err
	}
	if err != nil {
		var failure *contracts.ReconcileFailure
		if errors.As(err, &failure) {
			result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: failure.RollbackSucceeded, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rotation failed"}}
			return result, failure
		}
		return result, err
	}
	_ = metadata
	return result, nil
}
