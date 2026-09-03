package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/proxy"
	"supabase-manager/apps/provisioner/internal/render"
	"supabase-manager/internal/contracts"
	"supabase-manager/internal/diagnostic"
)

// Reconcile renders and atomically applies one complete project configuration.
// The request contains the complete canonical state. Legacy revision fields
// are accepted for wire compatibility but are deliberately ignored: the
// Manager serializes writes and the runtime always applies the supplied state.
func (backend *Backend) Reconcile(ctx context.Context, request contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	slog.Info("runtime reconciliation entered", "project_id", request.ProjectID, "slug", request.Slug, "operation_id", request.OperationID)
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
		// Revisions/fences belonged to the removed desired-vs-last-good protocol.
		// Keep a monotonic diagnostic value in metadata for operators, but never
		// reject a complete canonical apply because an old client supplied a stale
		// number. This is what makes Retry deterministic after a Manager restart.
		applyRevision := metadata.Revision + 1
		if request.NextRevision > applyRevision {
			applyRevision = request.NextRevision
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
			outcome.Response = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: metadata.Revision, RolledBack: outcome.RollbackSucceeded, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "Server runtime reconciliation failed"}, Diagnostic: redactedReconcileDiagnostic(request, outcome.Cause), DiagnosticVersion: contracts.DiagnosticVersionCompleteRedaction}
			outcome.Response.RuntimeChanged = published
			result = outcome.Response
			encoded, _ := json.Marshal(result)
			metadata.Idempotency[request.IdempotencyKey] = encoded
			return outcome
		}
		previousConfig := metadata.Configuration
		previousProxyRoute, previousProxyManaged := routeForProxy(request.Slug, previousConfig, request.Secrets)
		previousServices := append([]string(nil), metadata.EnabledServices...)
		if len(previousServices) == 0 && metadata.Revision > 0 {
			previousServices = enabledServices(previousConfig)
		}
		previousRef, previousRefErr := backend.projectFS.CurrentRuntimeGeneration(request.Slug)
		previousProject := compose.ProjectRef{Slug: request.Slug, Dir: previousRef.ProjectDir, ComposeFile: previousRef.ComposeFile, EnvFile: previousRef.EnvFile}
		currentRef, _ := backend.projectFS.CurrentRuntimeFiles(request.Slug)
		currentProject := compose.ProjectRef{Slug: request.Slug, Dir: currentRef.ProjectDir, ComposeFile: currentRef.ComposeFile, EnvFile: currentRef.EnvFile}
		locationChangeGate := metadata.Revision > 0 && request.Configuration.Services.Storage && storageLocationChanged(previousConfig.Storage, request.Configuration.Storage)
		if locationChangeGate {
			if previousRefErr != nil {
				return fail(fmt.Errorf("resolve previous runtime generation: %w", previousRefErr))
			}
			for _, inputPath := range []string{previousProject.ComposeFile, previousProject.EnvFile} {
				if inputErr := validateRuntimeInput(inputPath); inputErr != nil {
					return fail(fmt.Errorf("resolve previous runtime generation: %w", inputErr))
				}
			}
			count, countErr := backend.runner.StorageObjectCount(ctx, previousProject)
			if countErr != nil {
				return fail(fmt.Errorf("Storage contains objects: unable to determine object count: %w", countErr))
			}
			if count != 0 {
				return fail(fmt.Errorf("Storage contains objects: count=%d", count))
			}
		}
		slog.Info("runtime reconciliation stage", "project_id", request.ProjectID, "slug", request.Slug, "operation_id", request.OperationID, "stage", "render")
		rendered, err := render.Project(render.Input{
			ProjectID: request.ProjectID, ProjectName: request.ProjectName, Slug: request.Slug, APIPort: request.APIPort,
			Configuration: request.Configuration, Secrets: request.Secrets, RuntimeSecrets: request.RuntimeSecrets,
		})
		if err != nil {
			return fail(reconcileFailure(err, false))
		}
		if request.ForceRecreate && metadata.Revision > 0 && previousConfig.Services.Database && request.Configuration.Services.Database {
			if previousRefErr != nil {
				return fail(fmt.Errorf("resolve current runtime for PostgreSQL upgrade check: %w", previousRefErr))
			}
			if err := rejectPostgresMajorUpgrade(previousProject.ComposeFile, rendered.Compose); err != nil {
				return fail(err)
			}
		}
		candidateRef, restore, commit, err := backend.projectFS.StageRuntimeFilesWithRef(request.Slug, projectfs.RuntimeFiles{
			Compose: []byte(rendered.Compose), Env: []byte(rendered.Env), FunctionsEnv: []byte(rendered.FunctionsEnv), MailerTemplates: rendered.MailerTemplates,
		})
		if err != nil {
			return fail(reconcileFailure(err, false))
		}
		candidateProject := compose.ProjectRef{Slug: request.Slug, Dir: candidateRef.ProjectDir, ComposeFile: candidateRef.ComposeFile, EnvFile: candidateRef.EnvFile}
		newServices := append([]string(nil), rendered.EnabledComposeServices...)
		var proxyChanged bool
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
			if published && len(previousServices) == 0 {
				// A first reconcile has no prior generation to restore. Remove the
				// entire scoped Compose runtime and its uncommitted database state
				// while the candidate env/compose files are still selected. A retry
				// must run PostgreSQL's official initialization from an empty PGDATA.
				cleanupErr = action(func(actionCtx context.Context) error {
					return backend.runner.DownRuntime(actionCtx, currentProject)
				})
				if cleanupErr != nil {
					cleanupErr = fmt.Errorf("remove candidate runtime during rollback: %w", cleanupErr)
				}
				if resetErr := backend.projectFS.ResetInitialDatabase(request.Slug); resetErr != nil {
					cleanupErr = errors.Join(cleanupErr, fmt.Errorf("reset initial database during rollback: %w", resetErr))
				}
				if resetErr := action(func(actionCtx context.Context) error {
					return backend.runner.ResetDatabaseConfig(actionCtx, currentProject)
				}); resetErr != nil {
					cleanupErr = errors.Join(cleanupErr, fmt.Errorf("reset database configuration during rollback: %w", resetErr))
				}
			} else if published && len(added) > 0 {
				// The current candidate is still selected while newly added
				// containers are removed; this leaves volumes intact before restoring
				// the previous pointer.
				cleanupErr = action(func(actionCtx context.Context) error {
					return backend.runner.RemoveStopped(actionCtx, currentProject, added...)
				})
				if cleanupErr != nil {
					cleanupErr = fmt.Errorf("remove candidate services during rollback: %w", cleanupErr)
				}
			}
			rollbackErr := restore()
			if rollbackErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.Join(cause, cleanupErr, fmt.Errorf("restore previous runtime during rollback: %w", rollbackErr)), RollbackSucceeded: false, RuntimeChanged: published}
			}
			if !published {
				return &contracts.ReconcileFailure{Cause: errors.Join(cause, cleanupErr), RollbackSucceeded: cleanupErr == nil}
			}
			rollbackServices := append(affectedServices(previousConfig, request.Configuration), disabled...)
			rollbackServices = intersect(unique(rollbackServices), previousServices)
			var recoveryErr error
			if request.ForceRecreate && len(previousServices) > 0 {
				if err := action(func(actionCtx context.Context) error {
					return backend.runner.RecreateRuntime(actionCtx, previousProject)
				}); err != nil {
					recoveryErr = fmt.Errorf("recreate previous runtime during rollback: %w", err)
				}
			} else if len(previousServices) > 0 && len(rollbackServices) > 0 {
				if err := action(func(actionCtx context.Context) error {
					return backend.runner.Recreate(actionCtx, previousProject, rollbackServices...)
				}); err != nil {
					recoveryErr = fmt.Errorf("recreate previous services during rollback: %w", err)
				}
			}
			if recoveryErr == nil && (request.ForceRecreate && len(previousServices) > 0 || len(rollbackServices) > 0) {
				if err := action(func(actionCtx context.Context) error {
					return backend.waitHealthy(actionCtx, request.Slug, rollbackServices)
				}); err != nil {
					recoveryErr = fmt.Errorf("check previous runtime health during rollback: %w", err)
				}
			}
			if cleanupErr != nil || recoveryErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.Join(cause, cleanupErr, recoveryErr), RollbackSucceeded: false, RuntimeChanged: published}
			}
			if proxyChanged {
				if previousProxyManaged {
					if err := action(func(actionCtx context.Context) error {
						return backend.proxy.Apply(actionCtx, previousProxyRoute)
					}); err != nil {
						recoveryErr = fmt.Errorf("restore managed nginx site during rollback: %w", err)
					}
				} else if err := action(func(actionCtx context.Context) error {
					return backend.proxy.Remove(actionCtx, request.Slug)
				}); err != nil {
					recoveryErr = fmt.Errorf("remove managed nginx site during rollback: %w", err)
				}
			}
			if recoveryErr != nil {
				return &contracts.ReconcileFailure{Cause: errors.Join(cause, recoveryErr), RollbackSucceeded: false, RuntimeChanged: published}
			}
			return &contracts.ReconcileFailure{Cause: cause, RollbackSucceeded: true, RuntimeChanged: published}
		}
		if err := pointFunctionsEnvAtCandidate(candidateRef, rendered.Compose); err != nil {
			return fail(rollback(err))
		}
		slog.Info("runtime reconciliation stage", "project_id", request.ProjectID, "slug", request.Slug, "operation_id", request.OperationID, "stage", "validate_compose")
		if err := backend.runner.Validate(ctx, candidateProject); err != nil {
			return fail(rollback(err))
		}
		if request.PullImages {
			slog.Info("runtime reconciliation stage", "project_id", request.ProjectID, "slug", request.Slug, "operation_id", request.OperationID, "stage", "pull_images")
			if err := backend.runner.Pull(ctx, candidateProject); err != nil {
				return fail(rollback(fmt.Errorf("pull official runtime images: %w", err)))
			}
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
			// A project with no published revision has never completed its first
			// bootstrap. Stop any orphaned attempt, remove its partial PGDATA,
			// and let the pinned Postgres image run its full official init path.
			slog.Info("runtime reconciliation stage", "project_id", request.ProjectID, "slug", request.Slug, "operation_id", request.OperationID, "stage", "bootstrap_database")
			if err := backend.runner.DownRuntime(ctx, currentProject); err != nil {
				return fail(rollback(err))
			}
			if err := backend.projectFS.ResetInitialDatabase(request.Slug); err != nil {
				return fail(rollback(err))
			}
			if err := backend.runner.ResetDatabaseConfig(ctx, currentProject); err != nil {
				return fail(rollback(err))
			}
			if err := backend.runner.UpDatabase(ctx, currentProject); err != nil {
				return fail(rollback(err))
			}
			if err := backend.runner.VerifyDatabaseBootstrap(ctx, currentProject); err != nil {
				return fail(rollback(err))
			}
			if err := backend.runner.SynchronizeDatabaseRolePasswords(ctx, currentProject); err != nil {
				return fail(rollback(err))
			}
			dependent := without(newServices, "db")
			if err := backend.runner.UpSelected(ctx, currentProject, dependent...); err != nil {
				return fail(rollback(err))
			}
		} else {
			affected := servicesToRecreate(previousConfig, request.Configuration, newServices, request.ForceRecreate)
			slog.Info("runtime reconciliation stage", "project_id", request.ProjectID, "slug", request.Slug, "operation_id", request.OperationID, "stage", "recreate_services", "services", affected)
			if request.ForceRecreate {
				if err := backend.runner.RecreateRuntime(ctx, currentProject); err != nil {
					return fail(rollback(err))
				}
			} else if err := backend.runner.Recreate(ctx, currentProject, affected...); err != nil {
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
		slog.Info("runtime reconciliation stage", "project_id", request.ProjectID, "slug", request.Slug, "operation_id", request.OperationID, "stage", "wait_healthy", "services", newServices)
		if err := backend.waitHealthy(ctx, request.Slug, newServices); err != nil {
			return fail(rollback(err))
		}
		if currentProxyRoute, managed := routeForProxy(request.Slug, request.Configuration, request.Secrets); managed {
			if err := backend.proxy.Apply(ctx, currentProxyRoute); err != nil {
				return fail(rollback(fmt.Errorf("apply managed nginx site: %w", err)))
			}
			proxyChanged = true
		} else if previousProxyManaged {
			if err := backend.proxy.Remove(ctx, request.Slug); err != nil {
				return fail(rollback(fmt.Errorf("remove managed nginx site: %w", err)))
			}
			proxyChanged = true
		}
		result = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: applyRevision, EnabledServices: newServices, RecreatedServices: servicesToRecreate(previousConfig, request.Configuration, newServices, request.ForceRecreate)}
		encoded, _ := json.Marshal(result)
		metadata.ProjectID, metadata.ProjectName, metadata.Revision = request.ProjectID, request.ProjectName, applyRevision
		if request.Fence > 0 {
			metadata.Fence = request.Fence
		}
		metadata.Configuration = request.Configuration
		metadata.Configuration.Revision = applyRevision
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
			result = contracts.ReconcileProjectResponse{OperationID: request.OperationID, ProjectID: request.ProjectID, Revision: request.ExpectedRevision, RolledBack: runtimeRollbackSucceeded, RuntimeChanged: runtimeChanged, Error: &contracts.APIError{Code: "RECONCILE_FAILED", Message: "Server runtime reconciliation failed"}, Diagnostic: redactedReconcileDiagnostic(request, err), DiagnosticVersion: contracts.DiagnosticVersionCompleteRedaction}
			err = &contracts.ReconcileFailure{Cause: err, Response: result, RollbackSucceeded: runtimeRollbackSucceeded, RuntimeChanged: runtimeChanged}
		}
		return result, err
	}
	slog.Info("runtime reconciliation completed", "project_id", request.ProjectID, "slug", request.Slug, "operation_id", request.OperationID, "revision", metadata.Revision, "recreated_services", result.RecreatedServices)
	result.Revision = metadata.Revision
	return result, nil
}

func routeForProxy(slug string, configuration contracts.ProjectConfiguration, secrets contracts.ProjectSecrets) (proxy.Route, bool) {
	if configuration.Network.HTTPSMode != contracts.HTTPSModeExternal || configuration.General.Domain == "" || configuration.Network.APIPort == 0 {
		return proxy.Route{}, false
	}
	studioUsername := strings.TrimSpace(configuration.General.StudioUsername)
	if studioUsername == "" {
		studioUsername = "supabase"
	}
	return proxy.Route{
		Slug:               slug,
		Domain:             configuration.General.Domain,
		APIPort:            configuration.Network.APIPort,
		StudioPort:         configuration.Network.StudioPort,
		StudioEnabled:      configuration.Services.Studio,
		StudioUsername:     studioUsername,
		StudioPassword:     secrets.DashboardPassword,
		CertificateFile:    managedTLSCertificateFile(configuration.Network.ManagedTLS),
		CertificateKeyFile: managedTLSPrivateKeyFile(configuration.Network.ManagedTLS),
	}, true
}

func storageLocationChanged(before, after contracts.StorageConfig) bool {
	return before.Backend != after.Backend || before.Bucket != after.Bucket || before.Region != after.Region || before.Endpoint != after.Endpoint || before.AccountID != after.AccountID || before.LocalPath != after.LocalPath
}

// rejectPostgresMajorUpgrade keeps a Compose refresh from mounting an existing
// database directory with a different PostgreSQL major. Supabase documents
// major migrations as a backup-and-migrate procedure, not a container
// recreation, so the operation must stop before it pulls or changes Docker.
func rejectPostgresMajorUpgrade(previousCompose, candidateCompose string) error {
	previous, err := os.ReadFile(previousCompose)
	if err != nil {
		return fmt.Errorf("read current runtime for PostgreSQL upgrade check: %w", err)
	}
	previousMajor, previousOK := postgresMajor(string(previous))
	candidateMajor, candidateOK := postgresMajor(candidateCompose)
	if !previousOK || !candidateOK || previousMajor == candidateMajor {
		return nil
	}
	return fmt.Errorf("manual PostgreSQL major upgrade required (%d → %d): back up this server and follow Supabase's PostgreSQL upgrade guide before syncing the official runtime", previousMajor, candidateMajor)
}

func postgresMajor(compose string) (int, bool) {
	for _, line := range strings.Split(compose, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "image:") || !strings.Contains(line, "supabase/postgres:") {
			continue
		}
		image := strings.TrimSpace(strings.TrimPrefix(line, "image:"))
		tag := strings.TrimPrefix(image, "supabase/postgres:")
		major, _, _ := strings.Cut(tag, ".")
		value, err := strconv.Atoi(major)
		return value, err == nil && value > 0
	}
	return 0, false
}

func validateRuntimeInput(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("runtime input %q is not a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return file.Close()
}

func managedTLSCertificateFile(config *contracts.ManagedTLSConfig) string {
	if config == nil {
		return ""
	}
	return config.CertificateFile
}

func managedTLSPrivateKeyFile(config *contracts.ManagedTLSConfig) string {
	if config == nil {
		return ""
	}
	return config.PrivateKeyFile
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

var reconcileHealthInitialPoll = time.Second
var reconcileHealthMaxPoll = 5 * time.Second

func (backend *Backend) waitHealthy(ctx context.Context, slug string, enabled []string) error {
	if len(enabled) == 0 {
		return nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, reconcileHealthTimeout)
	defer cancel()
	var lastReport health.Report
	var lastProbeErr error
	pollDelay := reconcileHealthInitialPoll
	for {
		report, err := backend.inspector.Project(checkCtx, health.ProjectRef{Slug: slug, Enabled: enabled})
		if err != nil {
			if !retryableHealthProbeError(err) {
				return err
			}
			lastProbeErr = err
		} else {
			lastReport = report
			lastProbeErr = nil
			if report.Health == contracts.HealthHealthy {
				return nil
			}
			if !reportHasTransientService(report) {
				return healthFailureError(report)
			}
		}
		if err := waitForHealthPoll(checkCtx, pollDelay); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return healthTimeoutError(lastReport, lastProbeErr)
			}
			return err
		}
		pollDelay = nextHealthPollDelay(pollDelay)
	}
}

func retryableHealthProbeError(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var timeout net.Error
	return errors.As(err, &timeout) && timeout.Timeout()
}

// dockerControlPlaneUnavailableError means the runtime could not be observed
// during its verification window. It is deliberately distinct from an
// unhealthy service: callers must not turn an observation outage into another
// Docker/Compose mutation by attempting compensation immediately.
type dockerControlPlaneUnavailableError struct {
	cause error
}

func (e *dockerControlPlaneUnavailableError) Error() string {
	return fmt.Sprintf("Docker control plane unavailable while verifying runtime: %v", e.cause)
}

func (e *dockerControlPlaneUnavailableError) Unwrap() error { return e.cause }

func waitForHealthPoll(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextHealthPollDelay(current time.Duration) time.Duration {
	if current >= reconcileHealthMaxPoll {
		return reconcileHealthMaxPoll
	}
	next := current * 2
	if next > reconcileHealthMaxPoll {
		return reconcileHealthMaxPoll
	}
	return next
}

// healthFailureError preserves the per-service Docker state that caused a
// reconcile to fail. The candidate is intentionally removed during rollback,
// so this error is the only durable, secret-free diagnostic available to the
// Manager operation and Provisioner log.
func healthFailureError(report health.Report) error {
	failed := make([]string, 0)
	for _, service := range report.Services {
		if service.Health == contracts.HealthHealthy {
			continue
		}
		label := service.Name
		if service.Status != "" || service.Health != "" {
			label += fmt.Sprintf(" (%s, %s)", service.Status, service.Health)
		}
		failed = append(failed, label)
	}
	if len(failed) == 0 {
		return fmt.Errorf("runtime health is %s", report.Health)
	}
	return fmt.Errorf("runtime health is %s; services: %s", report.Health, strings.Join(failed, ", "))
}

func healthTimeoutError(report health.Report, probeErr ...error) error {
	if len(probeErr) > 0 && probeErr[0] != nil {
		return &dockerControlPlaneUnavailableError{cause: probeErr[0]}
	}
	starting := make([]string, 0)
	for _, service := range report.Services {
		if service.Health != contracts.HealthStarting {
			continue
		}
		label := service.Name
		if service.Status != "" {
			label += " (" + service.Status + ")"
		}
		starting = append(starting, label)
	}
	if len(starting) == 0 {
		return fmt.Errorf("runtime health did not become healthy before deadline")
	}
	return fmt.Errorf("runtime health did not become healthy before deadline; still starting: %s", strings.Join(starting, ", "))
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

func redactedReconcileDiagnostic(request contracts.ReconcileProjectRequest, cause error) string {
	if cause == nil {
		return "Server runtime reconciliation failed"
	}
	var failure *contracts.ReconcileFailure
	if errors.As(cause, &failure) && failure.Cause != nil {
		cause = failure.Cause
	}
	secrets := request.Secrets
	values := []string{
		secrets.DatabasePassword, secrets.JWTSecret, secrets.AnonKey, secrets.ServiceRoleKey,
		secrets.DashboardPassword, secrets.SecretKeyBase, secrets.VaultEncryptionKey,
		secrets.RealtimeDBEncryptionKey, secrets.LogflarePublicAccessToken,
		secrets.LogflarePrivateAccessToken, secrets.S3ProtocolAccessKeyID,
		secrets.S3ProtocolAccessKeySecret, secrets.PoolerTenantID,
		secrets.SupabasePublishableKey, secrets.SupabaseSecretKey, secrets.AnonKeyAsymmetric,
		secrets.ServiceRoleKeyAsymmetric, secrets.JWTKeys, secrets.JWTJWKS,
	}
	for _, value := range request.RuntimeSecrets {
		values = append(values, value)
	}
	values = append(values, diagnostic.ConfigurationSecretValues(request.Configuration)...)
	return diagnostic.Sanitize(cause.Error(), values)
}

func affectedServices(before, after contracts.ProjectConfiguration) []string {
	set := map[string]bool{}
	if before.General.SiteURL != after.General.SiteURL || before.General.Domain != after.General.Domain {
		set["auth"], set["studio"], set["api-gw"] = true, true, true
	}
	if before.General.AuthSiteURL != after.General.AuthSiteURL {
		set["auth"] = true
	}
	if before.Auth.SMTP != after.Auth.SMTP || !reflect.DeepEqual(before.Auth, after.Auth) {
		set["auth"] = true
		set["auth-templates"] = true
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

func servicesToRecreate(before, after contracts.ProjectConfiguration, enabled []string, force bool) []string {
	if force {
		return append([]string(nil), enabled...)
	}
	return intersect(affectedServices(before, after), enabled)
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
	add("auth-templates", config.Services.Auth)
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
