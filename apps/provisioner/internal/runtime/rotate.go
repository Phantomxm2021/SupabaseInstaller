package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/render"
	"supabase-manager/internal/contracts"
)

type PasswordRotator interface {
	RotateDatabasePassword(context.Context, compose.ProjectRef, string, string) error
}

// RollbackDatabasePassword compensates a successful runtime rotation when the
// Manager cannot publish its encrypted secret envelope.
func (backend *Backend) RollbackDatabasePassword(ctx context.Context, request contracts.RotateDatabasePasswordRequest) error {
	if request.OperationKind != "ROLLBACK_DATABASE_PASSWORD" || request.OldPassword == "" || request.NewPassword == "" {
		return contracts.ErrInvalidReconcileRevision
	}
	metadata, err := backend.projectFS.Metadata(request.Slug)
	if err != nil {
		return err
	}
	if metadata.ProjectID != request.ProjectID || metadata.Revision != request.NextRevision {
		return contracts.ErrStaleConfigRevision
	}
	rotator, ok := backend.runner.(PasswordRotator)
	if !ok {
		return errors.New("rotation rollback unavailable")
	}
	current, err := backend.projectFS.CurrentRuntimeFiles(request.Slug)
	if err != nil {
		return err
	}
	ref := compose.ProjectRef{Slug: request.Slug, Dir: current.ProjectDir, ComposeFile: current.ComposeFile, EnvFile: current.EnvFile}
	restoreRuntime := func() error { return nil }
	if request.Configuration.General.SupabaseVersion != "" {
		oldSecrets := request.Secrets
		oldSecrets.DatabasePassword = request.NewPassword
		rendered, renderErr := render.Project(render.Input{ProjectID: request.ProjectID, Slug: request.Slug, APIPort: request.Configuration.Network.APIPort, Configuration: request.Configuration, Secrets: oldSecrets, RuntimeSecrets: request.RuntimeSecrets})
		if renderErr != nil {
			return errors.New("rollback render failed")
		}
		candidate, restore, commit, stageErr := backend.projectFS.StageRuntimeFilesWithRef(request.Slug, projectfs.RuntimeFiles{Compose: []byte(rendered.Compose), Env: []byte(rendered.Env), FunctionsEnv: []byte(rendered.FunctionsEnv)})
		if stageErr == nil {
			stageErr = pointFunctionsEnvAtCandidate(candidate, rendered.Compose)
		}
		if stageErr == nil {
			stageErr = backend.runner.Validate(ctx, compose.ProjectRef{Slug: request.Slug, Dir: candidate.ProjectDir, ComposeFile: candidate.ComposeFile, EnvFile: candidate.EnvFile})
		}
		if stageErr != nil {
			_ = restore()
			return errors.New("rollback candidate validation failed")
		}
		if stageErr = commit(); stageErr != nil {
			_ = restore()
			return errors.New("rollback candidate commit failed")
		}
		restoreRuntime = restore
		current, err = backend.projectFS.CurrentRuntimeFiles(request.Slug)
		if err != nil {
			_ = restoreRuntime()
			return err
		}
		ref = compose.ProjectRef{Slug: request.Slug, Dir: current.ProjectDir, ComposeFile: current.ComposeFile, EnvFile: current.EnvFile}
	}
	services := without(enabledServices(metadata.Configuration), "db")
	if err := rotator.RotateDatabasePassword(ctx, ref, request.OldPassword, request.NewPassword); err != nil {
		_ = restoreRuntime()
		return errors.New("database password rollback failed")
	}
	if err := backend.runner.Recreate(ctx, ref, services...); err != nil {
		_ = restoreRuntime()
		return errors.New("dependent service rollback failed")
	}
	if err := backend.waitHealthy(ctx, request.Slug, enabledServices(metadata.Configuration)); err != nil {
		_ = restoreRuntime()
		return errors.New("database password rollback health check failed")
	}
	if err := restoreRuntime(); err != nil {
		return errors.New("rollback runtime publication failed")
	}
	return nil
}

// RotateDatabasePassword changes PostgreSQL first, then recreates and health
// checks every enabled dependent service. A failed health check attempts the
// inverse role change and dependent restart before reporting rollback status.
func (backend *Backend) RotateDatabasePassword(ctx context.Context, request contracts.RotateDatabasePasswordRequest) (contracts.RotateDatabasePasswordResponse, error) {
	var result contracts.RotateDatabasePasswordResponse
	var compensation func() error
	metadata, err := backend.projectFS.UpdateMetadataWithRollback(request.Slug, func(metadata *projectfs.Metadata) (callbackErr error) {
		defer func() {
			var failure *contracts.ReconcileFailure
			if errors.As(callbackErr, &failure) {
				result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: failure.RollbackSucceeded, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rotation failed"}}
				if raw, marshalErr := json.Marshal(result); marshalErr == nil {
					metadata.Idempotency[request.IdempotencyKey] = raw
				}
			}
		}()
		if request.OperationKind != "ROTATE_DATABASE_PASSWORD" {
			return contracts.ErrInvalidReconcileRevision
		}
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
		oldRef := ref
		restoreRuntime := func() error { return nil }
		if request.Configuration.General.SupabaseVersion != "" {
			rendered, renderErr := render.Project(render.Input{ProjectID: request.ProjectID, Slug: request.Slug, APIPort: request.Configuration.Network.APIPort, Configuration: request.Configuration, Secrets: request.Secrets, RuntimeSecrets: request.RuntimeSecrets})
			if renderErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.New("rotation render failed"), RollbackSucceeded: false}
			}
			candidate, restore, commit, stageErr := backend.projectFS.StageRuntimeFilesWithRef(request.Slug, projectfs.RuntimeFiles{Compose: []byte(rendered.Compose), Env: []byte(rendered.Env), FunctionsEnv: []byte(rendered.FunctionsEnv)})
			if stageErr == nil {
				stageErr = pointFunctionsEnvAtCandidate(candidate, rendered.Compose)
			}
			if stageErr == nil {
				stageErr = backend.runner.Validate(ctx, compose.ProjectRef{Slug: request.Slug, Dir: candidate.ProjectDir, ComposeFile: candidate.ComposeFile, EnvFile: candidate.EnvFile})
			}
			if stageErr != nil {
				_ = restore()
				return &contracts.ReconcileFailure{Cause: errors.New("rotation candidate validation failed"), RollbackSucceeded: false}
			}
			if stageErr = commit(); stageErr != nil {
				_ = restore()
				return &contracts.ReconcileFailure{Cause: errors.New("rotation candidate commit failed"), RollbackSucceeded: false}
			}
			restoreRuntime = restore
			current, _ = backend.projectFS.CurrentRuntimeFiles(request.Slug)
			ref = compose.ProjectRef{Slug: request.Slug, Dir: current.ProjectDir, ComposeFile: current.ComposeFile, EnvFile: current.EnvFile}
		}
		services := without(enabledServices(metadata.Configuration), "db")
		if err := rotator.RotateDatabasePassword(ctx, ref, request.OldPassword, request.NewPassword); err != nil {
			_ = restoreRuntime()
			return &contracts.ReconcileFailure{Cause: errors.New("database password update failed"), RollbackSucceeded: false}
		}
		rollback := func() error {
			if err := restoreRuntime(); err != nil {
				return err
			}
			if err := rotator.RotateDatabasePassword(ctx, oldRef, request.NewPassword, request.OldPassword); err != nil {
				return err
			}
			if err := backend.runner.Recreate(ctx, oldRef, services...); err != nil {
				return err
			}
			return backend.waitHealthy(ctx, request.Slug, enabledServices(metadata.Configuration))
		}
		compensation = rollback
		if err := backend.runner.Recreate(ctx, ref, services...); err != nil {
			rollbackErr := rollback()
			restoreErr := restoreRuntime()
			return &contracts.ReconcileFailure{Cause: errors.New("dependent service restart failed"), RollbackSucceeded: rollbackErr == nil && restoreErr == nil}
		}
		if err := backend.waitHealthy(ctx, request.Slug, enabledServices(metadata.Configuration)); err != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				_ = restoreRuntime()
				return &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: false}
			}
			if restoreErr := restoreRuntime(); restoreErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: false}
			}
			return &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: true}
		}
		result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.NextRevision}
		metadata.Revision = request.NextRevision
		metadata.Configuration = request.Configuration
		metadata.EnabledServices = enabledServices(request.Configuration)
		raw, _ := json.Marshal(result)
		metadata.Idempotency[request.IdempotencyKey] = raw
		return nil
	}, func() error {
		if compensation == nil {
			return errors.New("rotation compensation unavailable")
		}
		return compensation()
	})
	if err == nil && result.Error != nil {
		return result, &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: result.RolledBack}
	}
	if errors.Is(err, contracts.ErrStaleConfigRevision) || errors.Is(err, contracts.ErrInvalidReconcileRevision) {
		return contracts.RotateDatabasePasswordResponse{}, err
	}
	if err != nil {
		var failure *contracts.ReconcileFailure
		if errors.As(err, &failure) {
			result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: failure.RollbackSucceeded, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rotation failed"}}
			return result, failure
		}
		result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rotation failed"}}
		return result, &contracts.ReconcileFailure{Cause: errors.New("rotation metadata publication failed"), RollbackSucceeded: false}
	}
	_ = metadata
	return result, nil
}
