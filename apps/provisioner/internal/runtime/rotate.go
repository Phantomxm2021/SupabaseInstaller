package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/render"
	"supabase-manager/internal/contracts"
)

type PasswordRotator interface {
	RotateDatabasePassword(context.Context, compose.ProjectRef, string, string) error
}

// ConfirmDatabasePasswordRotation is the Manager publication boundary. It is
// separate from rotation so the journal cannot be closed before the encrypted
// secret transaction succeeds.
func (backend *Backend) ConfirmDatabasePasswordRotation(ctx context.Context, request contracts.ConfirmDatabasePasswordRotationRequest) error {
	if request.OperationID == "" || request.ProjectID == "" || request.Slug == "" || request.NextRevision <= request.ExpectedRevision {
		return contracts.ErrInvalidReconcileRevision
	}
	_, err := backend.projectFS.UpdateMetadata(request.Slug, func(metadata *projectfs.Metadata) error {
		if metadata.ProjectID != request.ProjectID || metadata.Revision != request.NextRevision {
			return contracts.ErrStaleConfigRevision
		}
		if request.Fence > 0 && metadata.Fence > request.Fence {
			return contracts.ErrStaleConfigRevision
		}
		if metadata.Rotation == nil {
			return nil // idempotent after the completed confirmation
		}
		if metadata.Rotation.OperationID != request.OperationID || metadata.Rotation.Phase != "provisioner-committed" || metadata.Rotation.NextRevision != request.NextRevision {
			return contracts.ErrStaleConfigRevision
		}
		metadata.Rotation = nil
		return nil
	})
	return err
}

// RollbackDatabasePassword compensates a successful runtime rotation when the
// Manager cannot publish its encrypted secret envelope.
func (backend *Backend) RollbackDatabasePassword(ctx context.Context, request contracts.RotateDatabasePasswordRequest) error {
	if request.Slug == "" {
		return contracts.ErrInvalidReconcileRevision
	}
	_, err := backend.projectFS.WithProjectMetadataLock(request.Slug, func(metadata *projectfs.Metadata) error {
		return backend.rollbackDatabasePasswordLocked(ctx, request, metadata)
	})
	return err
}

func (backend *Backend) rollbackDatabasePasswordLocked(ctx context.Context, request contracts.RotateDatabasePasswordRequest, metadata *projectfs.Metadata) error {
	if request.OperationKind != "ROLLBACK_DATABASE_PASSWORD" || request.OldPassword == "" || request.NewPassword == "" {
		return contracts.ErrInvalidReconcileRevision
	}
	var err error
	if stored, ok := metadata.Idempotency[request.IdempotencyKey]; ok {
		var prior contracts.RotateDatabasePasswordResponse
		if json.Unmarshal(stored, &prior) == nil && prior.RolledBack {
			return nil
		}
	}
	if metadata.ProjectID != request.ProjectID || metadata.Revision != request.NextRevision {
		return contracts.ErrStaleConfigRevision
	}
	if request.Fence > 0 && metadata.Fence > request.Fence {
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
	// The candidate is the compensated (old-password) generation. Once it is
	// committed it must remain current; invoking the staging restore closure
	// here would switch current back to the new-password generation.
	restoreRuntime := func() error { return nil }
	if metadata.Rotation != nil && metadata.Rotation.OldGeneration != "" {
		oldRef, refErr := backend.projectFS.RuntimeGeneration(request.Slug, metadata.Rotation.OldGeneration)
		if refErr != nil {
			return errors.New("rollback previous generation unavailable")
		}
		if err := backend.projectFS.SelectRuntimeGeneration(request.Slug, metadata.Rotation.OldGeneration); err != nil {
			return errors.New("rollback runtime publication failed")
		}
		ref = compose.ProjectRef{Slug: request.Slug, Dir: oldRef.ProjectDir, ComposeFile: oldRef.ComposeFile, EnvFile: oldRef.EnvFile}
	} else if request.Configuration.General.SupabaseVersion != "" {
		oldSecrets := request.Secrets
		oldSecrets.DatabasePassword = request.NewPassword
		rendered, renderErr := render.Project(render.Input{ProjectID: request.ProjectID, ProjectName: request.ProjectName, Slug: request.Slug, APIPort: request.Configuration.Network.APIPort, Configuration: request.Configuration, Secrets: oldSecrets, RuntimeSecrets: request.RuntimeSecrets})
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
		// Do not retain restore: external compensation's successful state is the
		// candidate itself, not the generation that preceded it.
		restoreRuntime = func() error { return nil }
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
	// Keep the old generation selected and make the metadata revision agree with
	// PostgreSQL and the running consumers. This is an idempotent durable commit.
	result := contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: true}
	encoded, _ := json.Marshal(result)
	if metadata.ProjectID != request.ProjectID || metadata.Revision != request.NextRevision {
		return contracts.ErrStaleConfigRevision
	}
	metadata.Revision = request.ExpectedRevision
	metadata.Configuration.Revision = request.ExpectedRevision
	metadata.Rotation = nil
	metadata.Idempotency[request.IdempotencyKey] = encoded
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
				result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: failure.RollbackSucceeded, RuntimeChanged: failure.RuntimeChanged, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rotation failed"}}
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
		if request.Fence > 0 && metadata.Fence > request.Fence {
			return contracts.ErrStaleConfigRevision
		}
		if metadata.Revision != request.ExpectedRevision || request.NextRevision <= request.ExpectedRevision || request.OldPassword == "" || request.NewPassword == "" || request.OldPassword == request.NewPassword {
			return contracts.ErrInvalidReconcileRevision
		}
		if request.Fence > 0 && metadata.Fence < request.Fence {
			metadata.Fence = request.Fence
			if err := backend.projectFS.WriteMetadataForPhase(request.Slug, *metadata); err != nil {
				return err
			}
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
		oldRuntimeRef, oldRefErr := backend.projectFS.CurrentRuntimeGeneration(request.Slug)
		if oldRefErr != nil {
			return oldRefErr
		}
		oldRef := compose.ProjectRef{Slug: request.Slug, Dir: oldRuntimeRef.ProjectDir, ComposeFile: oldRuntimeRef.ComposeFile, EnvFile: oldRuntimeRef.EnvFile}
		// A retry after a lost response must preserve the journal's original
		// generation. The current pointer may already be the new generation.
		if journal := metadata.Rotation; journal != nil && journal.OperationID == request.OperationID && journal.OldGeneration != "" {
			if journalRef, journalErr := backend.projectFS.RuntimeGeneration(request.Slug, journal.OldGeneration); journalErr == nil {
				oldRef = compose.ProjectRef{Slug: request.Slug, Dir: journalRef.ProjectDir, ComposeFile: journalRef.ComposeFile, EnvFile: journalRef.EnvFile}
			}
		}
		oldGeneration := filepath.Base(filepath.Dir(oldRef.ComposeFile))
		if metadata.Rotation == nil || metadata.Rotation.OperationID != request.OperationID {
			metadata.Rotation = &projectfs.RotationJournal{OperationID: request.OperationID, Phase: "prepared", OldGeneration: oldGeneration, ExpectedRevision: request.ExpectedRevision, NextRevision: request.NextRevision}
		} else {
			// Keep OldGeneration and any durable phase from the first attempt;
			// never overwrite the recovery anchor with the current pointer.
			metadata.Rotation.ExpectedRevision = request.ExpectedRevision
			metadata.Rotation.NextRevision = request.NextRevision
		}
		if err := backend.projectFS.WriteMetadataForPhase(request.Slug, *metadata); err != nil {
			return err
		}
		restoreRuntime := func() error { return nil }
		if request.Configuration.General.SupabaseVersion != "" {
			rendered, renderErr := render.Project(render.Input{ProjectID: request.ProjectID, ProjectName: request.ProjectName, Slug: request.Slug, APIPort: request.Configuration.Network.APIPort, Configuration: request.Configuration, Secrets: request.Secrets, RuntimeSecrets: request.RuntimeSecrets})
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
			publishedRef, _ := backend.projectFS.CurrentRuntimeGeneration(request.Slug)
			metadata.Rotation.NewGeneration = filepath.Base(filepath.Dir(publishedRef.ComposeFile))
			metadata.Rotation.OldGeneration = filepath.Base(filepath.Dir(oldRef.ComposeFile))
			metadata.Rotation.Phase = "runtime-published"
			if err := backend.projectFS.WriteMetadataForPhase(request.Slug, *metadata); err != nil {
				rollbackErr := restore()
				return &contracts.ReconcileFailure{Cause: err, RollbackSucceeded: rollbackErr == nil}
			}
		}
		services := without(enabledServices(metadata.Configuration), "db")
		if err := rotator.RotateDatabasePassword(ctx, ref, request.OldPassword, request.NewPassword); err != nil {
			_ = restoreRuntime()
			return &contracts.ReconcileFailure{Cause: errors.New("database password update failed"), RollbackSucceeded: false, RuntimeChanged: true}
		}
		metadata.Rotation.Phase = "db-rotated"
		if err := backend.projectFS.WriteMetadataForPhase(request.Slug, *metadata); err != nil {
			rollbackErr := rotator.RotateDatabasePassword(ctx, oldRef, request.NewPassword, request.OldPassword)
			restoreErr := restoreRuntime()
			return &contracts.ReconcileFailure{Cause: err, RollbackSucceeded: rollbackErr == nil && restoreErr == nil, RuntimeChanged: true}
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
			return &contracts.ReconcileFailure{Cause: errors.New("dependent service restart failed"), RollbackSucceeded: rollbackErr == nil && restoreErr == nil, RuntimeChanged: true}
		}
		if err := backend.waitHealthy(ctx, request.Slug, enabledServices(metadata.Configuration)); err != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				_ = restoreRuntime()
				return &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: false, RuntimeChanged: true}
			}
			if restoreErr := restoreRuntime(); restoreErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: false, RuntimeChanged: true}
			}
			return &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: true, RuntimeChanged: true}
		}
		metadata.Rotation.Phase = "services-verified"
		if err := backend.projectFS.WriteMetadataForPhase(request.Slug, *metadata); err != nil {
			rollbackErr := rollback()
			return &contracts.ReconcileFailure{Cause: err, RollbackSucceeded: rollbackErr == nil, RuntimeChanged: true}
		}
		metadata.Rotation.Phase = "provisioner-committed"
		if err := backend.projectFS.WriteMetadataForPhase(request.Slug, *metadata); err != nil {
			return err
		}
		result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.NextRevision}
		metadata.Revision = request.NextRevision
		if request.Fence > 0 {
			metadata.Fence = request.Fence
		}
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
		return result, &contracts.ReconcileFailure{Cause: errors.New("database password rotation failed"), RollbackSucceeded: result.RolledBack, RuntimeChanged: result.RuntimeChanged}
	}
	if errors.Is(err, contracts.ErrStaleConfigRevision) || errors.Is(err, contracts.ErrInvalidReconcileRevision) {
		return contracts.RotateDatabasePasswordResponse{}, err
	}
	if err != nil {
		var failure *contracts.ReconcileFailure
		if errors.As(err, &failure) {
			result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: failure.RollbackSucceeded, RuntimeChanged: failure.RuntimeChanged, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rotation failed"}}
			return result, failure
		}
		var publication *projectfs.MetadataPublicationError
		if errors.As(err, &publication) {
			result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: publication.RollbackSucceeded, RuntimeChanged: true, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rotation failed"}}
			return result, &contracts.ReconcileFailure{Cause: errors.New("rotation metadata publication failed"), RollbackSucceeded: publication.RollbackSucceeded, RuntimeChanged: true}
		}
		result = contracts.RotateDatabasePasswordResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rotation failed"}}
		return result, &contracts.ReconcileFailure{Cause: errors.New("rotation metadata publication failed"), RollbackSucceeded: false}
	}
	_ = metadata
	return result, nil
}
