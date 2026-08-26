package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/render"
	"supabase-manager/internal/contracts"
)

// Reconcile renders and atomically applies one complete project configuration.
// Metadata, idempotency and the last-known-good revision are updated only after
// the candidate has passed Compose validation and service health checks.
func (backend *Backend) Reconcile(ctx context.Context, request contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	var result contracts.ReconcileProjectResponse
	var runtimeRollback func() error
	var runtimeRollbackSucceeded bool
	metadata, err := backend.projectFS.UpdateMetadataWithRollback(request.Slug, func(metadata *projectfs.Metadata) error {
		if stored, ok := metadata.Idempotency[request.IdempotencyKey]; ok {
			if err := json.Unmarshal(stored, &result); err != nil {
				return err
			}
			if result.Error != nil {
				return &contracts.ReconcileFailure{Response: result, RollbackSucceeded: result.RolledBack, RuntimeChanged: result.RuntimeChanged}
			}
			return nil
		}
		if metadata.Revision != request.ExpectedRevision {
			return contracts.ErrStaleConfigRevision
		}
		if request.Fence > 0 && metadata.Fence > request.Fence {
			return contracts.ErrStaleConfigRevision
		}
		if request.NextRevision <= metadata.Revision || request.Configuration.Revision != request.NextRevision {
			return contracts.ErrInvalidReconcileRevision
		}
		// Claim the highest fence durably before rendering or touching Docker.
		// The projectfs file lock serializes this claim across Provisioner
		// processes; every later metadata publication carries the same token.
		if request.Fence > 0 && metadata.Fence < request.Fence {
			metadata.Fence = request.Fence
			if err := backend.projectFS.WriteMetadataForPhase(request.Slug, *metadata); err != nil {
				return err
			}
		}
		published := false
		fail := func(err error) error {
			var outcome *contracts.ReconcileFailure
			if !errors.As(err, &outcome) {
				outcome = &contracts.ReconcileFailure{Cause: err}
			}
			outcome.RuntimeChanged = published
			outcome.Response = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: metadata.Revision, RolledBack: outcome.RollbackSucceeded, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "Project runtime reconciliation failed"}}
			outcome.Response.RuntimeChanged = published
			result = outcome.Response
			encoded, _ := json.Marshal(result)
			metadata.Idempotency[request.IdempotencyKey] = encoded
			return outcome
		}
		previousConfig := metadata.Configuration
		previousServices := append([]string(nil), metadata.EnabledServices...)
		if len(previousServices) == 0 && metadata.Revision > 0 {
			previousServices = enabledServices(previousConfig)
		}
		rendered, err := render.Project(render.Input{
			ProjectID: request.ProjectID, Slug: request.Slug, APIPort: request.APIPort,
			Configuration: request.Configuration, Secrets: request.Secrets, RuntimeSecrets: request.RuntimeSecrets,
		})
		if err != nil {
			return fail(reconcileFailure(err, false))
		}
		candidateRef, restore, commit, err := backend.projectFS.StageRuntimeFilesWithRef(request.Slug, projectfs.RuntimeFiles{
			Compose: []byte(rendered.Compose), Env: []byte(rendered.Env), FunctionsEnv: []byte(rendered.FunctionsEnv),
		})
		if err != nil {
			return fail(reconcileFailure(err, false))
		}
		candidateProject := compose.ProjectRef{Slug: request.Slug, Dir: candidateRef.ProjectDir, ComposeFile: candidateRef.ComposeFile, EnvFile: candidateRef.EnvFile}
		previousRef, _ := backend.projectFS.CurrentRuntimeGeneration(request.Slug)
		previousProject := compose.ProjectRef{Slug: request.Slug, Dir: previousRef.ProjectDir, ComposeFile: previousRef.ComposeFile, EnvFile: previousRef.EnvFile}
		currentRef, _ := backend.projectFS.CurrentRuntimeFiles(request.Slug)
		currentProject := compose.ProjectRef{Slug: request.Slug, Dir: currentRef.ProjectDir, ComposeFile: currentRef.ComposeFile, EnvFile: currentRef.EnvFile}
		newServices := append([]string(nil), rendered.EnabledComposeServices...)
		disabled := difference(previousServices, newServices)
		added := difference(newServices, previousServices)
		rollback := func(cause error) error {
			rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), rollbackBudget)
			defer cancelRollback()
			action := func(fn func(context.Context) error) error {
				actionCtx, cancelAction := context.WithTimeout(rollbackCtx, rollbackActionBudget)
				defer cancelAction()
				return fn(actionCtx)
			}
			var cleanupErr error
			if published && len(added) > 0 {
				// The current candidate is still selected while containers are
				// removed; this leaves volumes intact before restoring the pointer.
				cleanupErr = action(func(actionCtx context.Context) error {
					return backend.runner.RemoveStopped(actionCtx, currentProject, added...)
				})
			}
			rollbackErr := restore()
			if rollbackErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.Join(cause, cleanupErr, rollbackErr), RollbackSucceeded: false, RuntimeChanged: published}
			}
			if !published {
				return &contracts.ReconcileFailure{Cause: errors.Join(cause, cleanupErr), RollbackSucceeded: cleanupErr == nil}
			}
			rollbackServices := append(affectedServices(previousConfig, request.Configuration), disabled...)
			rollbackServices = intersect(unique(rollbackServices), previousServices)
			var recoveryErr error
			if len(previousServices) > 0 && len(rollbackServices) > 0 {
				if err := action(func(actionCtx context.Context) error {
					return backend.runner.Recreate(actionCtx, previousProject, rollbackServices...)
				}); err != nil {
					recoveryErr = err
				}
			}
			if recoveryErr == nil && len(rollbackServices) > 0 {
				if err := action(func(actionCtx context.Context) error {
					return backend.waitHealthy(actionCtx, request.Slug, rollbackServices)
				}); err != nil {
					recoveryErr = err
				}
			}
			if cleanupErr != nil || recoveryErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.Join(cause, cleanupErr, recoveryErr), RollbackSucceeded: false, RuntimeChanged: published}
			}
			return &contracts.ReconcileFailure{Cause: cause, RollbackSucceeded: true, RuntimeChanged: published}
		}
		if err := pointFunctionsEnvAtCandidate(candidateRef, rendered.Compose); err != nil {
			return fail(rollback(err))
		}
		if err := backend.runner.Validate(ctx, candidateProject); err != nil {
			return fail(rollback(err))
		}
		if err := writeCandidateCompose(candidateRef.ComposeFile, []byte(rendered.Compose)); err != nil {
			return fail(rollback(err))
		}
		if err := commit(); err != nil {
			return fail(rollback(err))
		}
		published = true
		if err := backend.runner.RemoveStopped(ctx, previousProject, disabled...); err != nil {
			return fail(rollback(err))
		}
		if metadata.Revision == 0 && len(previousServices) == 0 {
			if err := backend.runner.UpDatabase(ctx, currentProject); err != nil {
				return fail(rollback(err))
			}
			dependent := without(newServices, "db")
			if err := backend.runner.UpSelected(ctx, currentProject, dependent...); err != nil {
				return fail(rollback(err))
			}
		} else {
			affected := intersect(affectedServices(previousConfig, request.Configuration), newServices)
			if err := backend.runner.Recreate(ctx, currentProject, affected...); err != nil {
				return fail(rollback(err))
			}
			if err := backend.runner.UpSelected(ctx, currentProject, added...); err != nil {
				return fail(rollback(err))
			}
		}
		// The disposable acceptance harness injects one inspector failure after
		// candidate runtime apply, before normal candidate health publication.
		// The normal rollback path then restores the previous pointer and
		// recreates its affected services.
		if backend.acceptanceInspectorFailOnce.Load() && request.Configuration.Auth.OAuth != nil && backend.consumeAcceptanceInspectorFailure() {
			_, inspectErr := backend.inspector.Project(ctx, health.ProjectRef{Slug: request.Slug, Enabled: newServices})
			cause := errors.New("acceptance inspector failure")
			if inspectErr != nil {
				cause = errors.Join(cause, inspectErr)
			}
			return fail(rollback(cause))
		}
		if err := backend.waitHealthy(ctx, request.Slug, newServices); err != nil {
			return fail(rollback(err))
		}
		result = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.NextRevision, EnabledServices: newServices, RecreatedServices: intersect(affectedServices(previousConfig, request.Configuration), newServices)}
		encoded, _ := json.Marshal(result)
		metadata.ProjectID, metadata.ProjectName, metadata.Revision = request.ProjectID, request.ProjectName, request.NextRevision
		if request.Fence > 0 {
			metadata.Fence = request.Fence
		}
		metadata.Configuration = request.Configuration
		metadata.Configuration.Revision = request.NextRevision
		metadata.EnabledServices = append([]string(nil), newServices...)
		metadata.Idempotency[request.IdempotencyKey] = encoded
		runtimeRollback = func() error {
			err := rollback(fmt.Errorf("metadata publication failed"))
			var failure *contracts.ReconcileFailure
			runtimeRollbackSucceeded = errors.As(err, &failure) && failure.RollbackSucceeded
			return err
		}
		return nil
	}, func() error {
		if runtimeRollback == nil {
			return nil
		}
		return runtimeRollback()
	})
	if errors.Is(err, contracts.ErrStaleConfigRevision) {
		return contracts.ReconcileProjectResponse{}, err
	}
	if err != nil {
		if !errors.Is(err, contracts.ErrInvalidReconcileRevision) && !errors.Is(err, contracts.ErrStaleConfigRevision) && result.Error == nil {
			runtimeChanged := runtimeRollback != nil
			result = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: runtimeRollbackSucceeded, RuntimeChanged: runtimeChanged, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "Project runtime reconciliation failed"}}
			err = &contracts.ReconcileFailure{Cause: err, Response: result, RollbackSucceeded: runtimeRollbackSucceeded, RuntimeChanged: runtimeChanged}
		}
		return result, err
	}
	result.Revision = metadata.Revision
	return result, nil
}

func pointFunctionsEnvAtCandidate(ref projectfs.RuntimeRef, original string) error {
	path := filepath.ToSlash(filepath.Join(".", ".manager-runtime", "current", ".env.functions"))
	candidate, err := filepath.Rel(ref.ProjectDir, ref.FunctionsFile)
	if err != nil {
		return err
	}
	candidate = filepath.ToSlash(filepath.Join(".", candidate))
	patched := bytes.Replace([]byte(original), []byte(path), []byte(candidate), 1)
	if bytes.Equal(patched, []byte(original)) {
		return nil
	}
	return writeCandidateCompose(ref.ComposeFile, patched)
}

func writeCandidateCompose(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// Compose service replacement can legitimately take several minutes while
// dependent databases and migrations settle. This deadline is independent of
// the HTTP request lifecycle; the Manager runs reconciliation in a durable
// background operation.
const reconcileHealthTimeout = 5 * time.Minute
const reconcileHealthPoll = 50 * time.Millisecond

func (backend *Backend) waitHealthy(ctx context.Context, slug string, enabled []string) error {
	if len(enabled) == 0 {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, reconcileHealthTimeout)
	defer cancel()
	for {
		report, err := backend.inspector.Project(checkCtx, health.ProjectRef{Slug: slug, Enabled: enabled})
		if err != nil {
			return err
		}
		if report.Health == contracts.HealthHealthy {
			return nil
		}
		if !reportHasTransientService(report) {
			return fmt.Errorf("runtime health is %s", report.Health)
		}
		timer := time.NewTimer(reconcileHealthPoll)
		select {
		case <-checkCtx.Done():
			if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf("runtime health did not become healthy before deadline")
			}
			return checkCtx.Err()
		case <-timer.C:
		}
	}
}

func reportHasTransientService(report health.Report) bool {
	for _, service := range report.Services {
		if service.Health == contracts.HealthUnhealthy {
			return false
		}
		if service.Health == contracts.HealthStarting {
			return true
		}
	}
	return report.Health == contracts.HealthStarting
}

func reconcileFailure(cause error, rolledBack bool) error {
	return &contracts.ReconcileFailure{Cause: cause, RollbackSucceeded: rolledBack}
}

func affectedServices(before, after contracts.ProjectConfiguration) []string {
	set := map[string]bool{}
	if before.General.SiteURL != after.General.SiteURL || before.General.Domain != after.General.Domain {
		set["auth"], set["studio"], set["api-gw"] = true, true, true
	}
	if before.Auth.SMTP != after.Auth.SMTP || !reflect.DeepEqual(before.Auth, after.Auth) {
		set["auth"] = true
	}
	if !reflect.DeepEqual(before.Functions, after.Functions) {
		set["functions"] = true
	}
	if !reflect.DeepEqual(before.Realtime, after.Realtime) || before.Services.Realtime != after.Services.Realtime {
		set["realtime"] = true
	}
	if !reflect.DeepEqual(before.Storage, after.Storage) {
		set["storage"] = true
	}
	if before.Services.Imgproxy != after.Services.Imgproxy && before.Services.Storage && after.Services.Storage {
		set["storage"] = true
	}
	if !reflect.DeepEqual(before.Database, after.Database) {
		set["db"] = true
	}
	if !reflect.DeepEqual(before.Pooler, after.Pooler) {
		set["supavisor"] = true
	}
	if before.Network.Gateway != after.Network.Gateway || before.Network.HTTPSMode != after.Network.HTTPSMode || before.Network.InternalGatewayPort != after.Network.InternalGatewayPort || before.Network.APIPort != after.Network.APIPort {
		set["api-gw"] = true
	}
	if before.Network.DirectDatabasePort != after.Network.DirectDatabasePort {
		set["db"] = true
	}
	if before.Services.DirectDB != after.Services.DirectDB {
		set["db"] = true
	}
	if before.Network.PoolerPort != after.Network.PoolerPort {
		set["supavisor"] = true
	}
	if before.Network.StudioPort != after.Network.StudioPort {
		set["studio"] = true
	}
	if before.Services.Gateway != after.Services.Gateway {
		set["api-gw"] = true
	}
	if before.Services.Database != after.Services.Database {
		set["db"] = true
	}
	if before.Services.Auth != after.Services.Auth {
		set["auth"] = true
	}
	if before.Services.REST != after.Services.REST {
		set["rest"] = true
	}
	if before.Services.PostgresMeta != after.Services.PostgresMeta {
		set["meta"] = true
	}
	if before.Services.Storage != after.Services.Storage {
		set["storage"] = true
	}
	if before.Services.Functions != after.Services.Functions {
		set["functions"] = true
	}
	if before.Services.Supavisor != after.Services.Supavisor {
		set["supavisor"] = true
	}
	if before.Services.Logs != after.Services.Logs {
		set["analytics"], set["logflare"] = true, true
		set["vector"] = true
	}
	if before.Services.Vector != after.Services.Vector {
		set["vector"] = true
	}
	if before.Services.Studio != after.Services.Studio {
		set["studio"] = true
	}
	if before.Services.Logs != after.Services.Logs && before.Services.Vector && after.Services.Vector {
		set["vector"] = true
	}
	order := []string{"db", "auth", "rest", "meta", "studio", "api-gw", "realtime", "storage", "imgproxy", "functions", "supavisor", "analytics", "logflare", "vector"}
	result := make([]string, 0, len(set))
	for _, name := range order {
		if set[name] {
			result = append(result, name)
		}
	}
	return result
}

func enabledServices(config contracts.ProjectConfiguration) []string {
	result := []string{}
	add := func(name string, enabled bool) {
		if enabled {
			result = append(result, name)
		}
	}
	add("db", config.Services.Database)
	add("api-gw", config.Services.Gateway)
	add("auth", config.Services.Auth)
	add("rest", config.Services.REST)
	add("meta", config.Services.PostgresMeta)
	add("studio", config.Services.Studio)
	add("realtime", config.Services.Realtime)
	add("storage", config.Services.Storage)
	add("imgproxy", config.Services.Imgproxy)
	add("functions", config.Services.Functions)
	add("supavisor", config.Services.Supavisor)
	add("analytics", config.Services.Logs)
	add("vector", config.Services.Vector)
	return result
}

func difference(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	out := []string{}
	for _, x := range a {
		if !set[x] {
			out = append(out, x)
		}
	}
	return out
}
func intersect(a, b []string) []string {
	set := map[string]bool{}
	for _, x := range b {
		set[x] = true
	}
	out := []string{}
	for _, x := range a {
		if set[x] {
			out = append(out, x)
		}
	}
	return out
}
func without(a []string, name string) []string {
	out := []string{}
	for _, x := range a {
		if x != name {
			out = append(out, x)
		}
	}
	return out
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
