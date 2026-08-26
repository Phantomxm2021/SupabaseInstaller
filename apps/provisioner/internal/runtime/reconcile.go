package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

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
	metadata, err := backend.projectFS.UpdateMetadata(request.Slug, func(metadata *projectfs.Metadata) error {
		if stored, ok := metadata.Idempotency[request.IdempotencyKey]; ok {
			return json.Unmarshal(stored, &result)
		}
		if metadata.Revision != request.ExpectedRevision {
			return contracts.ErrStaleConfigRevision
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
			return reconcileFailure(err, false)
		}
		restore, commit, err := backend.projectFS.StageRuntimeFiles(request.Slug, projectfs.RuntimeFiles{
			Compose: []byte(rendered.Compose), Env: []byte(rendered.Env), FunctionsEnv: []byte(rendered.FunctionsEnv),
		})
		if err != nil {
			return reconcileFailure(err, false)
		}
		project := compose.ProjectRef{Slug: request.Slug, Dir: mustRuntimeDir(backend.projectFS, request.Slug), ComposeFile: mustRuntimeCompose(backend.projectFS, request.Slug), EnvFile: mustRuntimeEnv(backend.projectFS, request.Slug)}
		rollback := func(cause error) error {
			rollbackErr := restore()
			if rollbackErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.Join(cause, rollbackErr), RollbackSucceeded: false}
			}
			rollbackServices := affectedServices(previousConfig, request.Configuration)
			if len(rollbackServices) > 0 {
				if err := backend.runner.Recreate(ctx, project, rollbackServices...); err != nil {
					return &contracts.ReconcileFailure{Cause: errors.Join(cause, err), RollbackSucceeded: false}
				}
			}
			if len(previousServices) > 0 {
				report, err := backend.inspector.Project(ctx, health.ProjectRef{Slug: request.Slug, Enabled: previousServices})
				if err != nil || report.Health != contracts.HealthHealthy {
					if err == nil {
						err = fmt.Errorf("previous runtime health is %s", report.Health)
					}
					return &contracts.ReconcileFailure{Cause: errors.Join(cause, err), RollbackSucceeded: false}
				}
			}
			return &contracts.ReconcileFailure{Cause: cause, RollbackSucceeded: true}
		}
		if err := backend.runner.Validate(ctx, project); err != nil {
			return rollback(err)
		}
		if err := commit(); err != nil {
			return rollback(err)
		}
		newServices := append([]string(nil), rendered.EnabledComposeServices...)
		disabled := difference(previousServices, newServices)
		if err := backend.runner.RemoveStopped(ctx, project, disabled...); err != nil {
			return rollback(err)
		}
		if metadata.Revision == 0 && len(previousServices) == 0 {
			if err := backend.runner.UpDatabase(ctx, project); err != nil {
				return rollback(err)
			}
			dependent := without(newServices, "db")
			if err := backend.runner.UpSelected(ctx, project, dependent...); err != nil {
				return rollback(err)
			}
		} else {
			affected := intersect(affectedServices(previousConfig, request.Configuration), newServices)
			if err := backend.runner.Recreate(ctx, project, affected...); err != nil {
				return rollback(err)
			}
			added := difference(newServices, previousServices)
			if err := backend.runner.UpSelected(ctx, project, added...); err != nil {
				return rollback(err)
			}
		}
		report, err := backend.inspector.Project(ctx, health.ProjectRef{Slug: request.Slug, Enabled: newServices})
		if err != nil {
			return rollback(err)
		}
		if report.Health != contracts.HealthHealthy {
			return rollback(fmt.Errorf("runtime health is %s", report.Health))
		}
		result = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.NextRevision, EnabledServices: newServices, RecreatedServices: intersect(affectedServices(previousConfig, request.Configuration), newServices)}
		encoded, _ := json.Marshal(result)
		metadata.ProjectID, metadata.ProjectName, metadata.Revision = request.ProjectID, request.ProjectName, request.NextRevision
		metadata.Configuration = request.Configuration
		metadata.Configuration.Revision = request.NextRevision
		metadata.EnabledServices = append([]string(nil), newServices...)
		metadata.Idempotency[request.IdempotencyKey] = encoded
		return nil
	})
	if errors.Is(err, contracts.ErrStaleConfigRevision) {
		return contracts.ReconcileProjectResponse{}, err
	}
	if err != nil {
		return contracts.ReconcileProjectResponse{}, err
	}
	result.Revision = metadata.Revision
	return result, nil
}

func reconcileFailure(cause error, rolledBack bool) error {
	return &contracts.ReconcileFailure{Cause: cause, RollbackSucceeded: rolledBack}
}

func mustRuntimeDir(root *projectfs.Root, slug string) string {
	ref, _ := root.CurrentRuntimeFiles(slug)
	return ref.ProjectDir
}
func mustRuntimeCompose(root *projectfs.Root, slug string) string {
	ref, _ := root.CurrentRuntimeFiles(slug)
	return ref.ComposeFile
}
func mustRuntimeEnv(root *projectfs.Root, slug string) string {
	ref, _ := root.CurrentRuntimeFiles(slug)
	return ref.EnvFile
}

func affectedServices(before, after contracts.ProjectConfiguration) []string {
	set := map[string]bool{}
	if before.General.SiteURL != after.General.SiteURL {
		set["auth"], set["studio"], set["api-gw"] = true, true, true
	}
	if before.Auth.SMTP != after.Auth.SMTP || !reflect.DeepEqual(before.Auth, after.Auth) {
		set["auth"] = true
	}
	if !reflect.DeepEqual(before.Functions, after.Functions) {
		set["functions"] = true
	}
	if !reflect.DeepEqual(before.Storage, after.Storage) {
		set["storage"] = true
	}
	if !reflect.DeepEqual(before.Database, after.Database) {
		set["db"] = true
	}
	if !reflect.DeepEqual(before.Pooler, after.Pooler) {
		set["supavisor"] = true
	}
	if before.Network != after.Network {
		set["api-gw"] = true
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
