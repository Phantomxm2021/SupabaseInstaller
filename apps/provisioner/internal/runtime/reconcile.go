package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
				return &contracts.ReconcileFailure{Response: result, RollbackSucceeded: result.RolledBack}
			}
			return nil
		}
		if metadata.Revision != request.ExpectedRevision {
			return contracts.ErrStaleConfigRevision
		}
		if request.NextRevision <= metadata.Revision || request.Configuration.Revision != request.NextRevision {
			return contracts.ErrInvalidReconcileRevision
		}
		fail := func(err error) error {
			var outcome *contracts.ReconcileFailure
			if !errors.As(err, &outcome) {
				outcome = &contracts.ReconcileFailure{Cause: err}
			}
			outcome.Response = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: metadata.Revision, RolledBack: outcome.RollbackSucceeded, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "Project runtime reconciliation failed"}}
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
		previousRef, _ := backend.projectFS.CurrentRuntimeFiles(request.Slug)
		previousProject := compose.ProjectRef{Slug: request.Slug, Dir: previousRef.ProjectDir, ComposeFile: previousRef.ComposeFile, EnvFile: previousRef.EnvFile}
		currentProject := previousProject
		currentProject.Slug = request.Slug
		newServices := append([]string(nil), rendered.EnabledComposeServices...)
		disabled := difference(previousServices, newServices)
		published := false
		rollback := func(cause error) error {
			if published && len(previousServices) == 0 && len(newServices) > 0 {
				// The current candidate is still selected while containers are
				// removed; this leaves volumes intact before restoring the pointer.
				if err := backend.runner.RemoveStopped(ctx, currentProject, newServices...); err != nil {
					return &contracts.ReconcileFailure{Cause: errors.Join(cause, err), RollbackSucceeded: false}
				}
			}
			rollbackErr := restore()
			if rollbackErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.Join(cause, rollbackErr), RollbackSucceeded: false}
			}
			rollbackServices := append(affectedServices(previousConfig, request.Configuration), disabled...)
			rollbackServices = unique(rollbackServices)
			if len(previousServices) > 0 && len(rollbackServices) > 0 {
				if err := backend.runner.Recreate(ctx, previousProject, rollbackServices...); err != nil {
					return &contracts.ReconcileFailure{Cause: errors.Join(cause, err), RollbackSucceeded: false}
				}
			}
			if len(previousServices) > 0 {
				if err := backend.waitHealthy(ctx, request.Slug, previousServices); err != nil {
					return &contracts.ReconcileFailure{Cause: errors.Join(cause, err), RollbackSucceeded: false}
				}
			}
			return &contracts.ReconcileFailure{Cause: cause, RollbackSucceeded: true}
		}
		if err := backend.runner.Validate(ctx, candidateProject); err != nil {
			return fail(rollback(err))
		}
		if err := backend.runner.RemoveStopped(ctx, previousProject, disabled...); err != nil {
			return fail(rollback(err))
		}
		if err := commit(); err != nil {
			return fail(rollback(err))
		}
		published = true
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
			added := difference(newServices, previousServices)
			if err := backend.runner.UpSelected(ctx, currentProject, added...); err != nil {
				return fail(rollback(err))
			}
		}
		if err := backend.waitHealthy(ctx, request.Slug, newServices); err != nil {
			return fail(rollback(err))
		}
		result = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.NextRevision, EnabledServices: newServices, RecreatedServices: intersect(affectedServices(previousConfig, request.Configuration), newServices)}
		encoded, _ := json.Marshal(result)
		metadata.ProjectID, metadata.ProjectName, metadata.Revision = request.ProjectID, request.ProjectName, request.NextRevision
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
			result = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: runtimeRollbackSucceeded, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "Project runtime reconciliation failed"}}
			err = &contracts.ReconcileFailure{Cause: err, Response: result, RollbackSucceeded: runtimeRollbackSucceeded}
		}
		return result, err
	}
	result.Revision = metadata.Revision
	return result, nil
}

const reconcileHealthTimeout = 30 * time.Second
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
		if report.Health != contracts.HealthStarting {
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
	if before.Network.Gateway != after.Network.Gateway || before.Network.HTTPSMode != after.Network.HTTPSMode || before.Network.InternalGatewayPort != after.Network.InternalGatewayPort || before.Network.APIPort != after.Network.APIPort || before.Network.Certificate != after.Network.Certificate {
		set["api-gw"] = true
	}
	if before.Network.DirectDatabasePort != after.Network.DirectDatabasePort {
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
