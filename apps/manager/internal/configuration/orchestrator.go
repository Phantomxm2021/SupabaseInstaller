// Package configuration coordinates durable configuration revisions with the
// private Provisioner reconciliation API.
package configuration

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/ports"
	"supabase-manager/apps/manager/internal/project"
	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type Provisioner interface {
	Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error)
}

type PasswordRotationProvisioner interface {
	RotateDatabasePassword(context.Context, contracts.RotateDatabasePasswordRequest) (contracts.RotateDatabasePasswordResponse, error)
}

// PasswordRotationRollbackProvisioner is an explicit compensation boundary
// used if Manager cannot publish the newly rotated encrypted secret.
type PasswordRotationRollbackProvisioner interface {
	RollbackDatabasePassword(context.Context, contracts.RotateDatabasePasswordRequest) error
}

type Orchestrator struct {
	store       *store.Store
	operations  *operation.Service
	configs     *project.ConfigurationService
	provisioner Provisioner
	cipher      *managersecrets.Cipher
	now         func() time.Time
	id          func() string
	locksMu     sync.Mutex
	locks       map[string]chan struct{}
}

func (o *Orchestrator) Get(ctx context.Context, projectID string) (store.ConfigurationSnapshot, error) {
	if o.configs == nil {
		return store.ConfigurationSnapshot{}, errors.New("configuration orchestrator is unavailable")
	}
	return o.configs.Get(ctx, projectID)
}

// NewOrchestrator accepts the dependencies in any order to keep the manager's
// composition root and small integration fakes source-compatible. Supported
// arguments are *ConfigurationService, Provisioner, *Cipher, func() time.Time,
// and func() string.
func NewOrchestrator(database *store.Store, operations *operation.Service, args ...any) *Orchestrator {
	o := &Orchestrator{store: database, operations: operations, now: time.Now, id: randomID, locks: make(map[string]chan struct{})}
	for _, arg := range args {
		switch value := arg.(type) {
		case *project.ConfigurationService:
			o.configs = value
		case Provisioner:
			o.provisioner = value
		case *managersecrets.Cipher:
			o.cipher = value
		case func() time.Time:
			o.now = value
		case func() string:
			o.id = value
		}
	}
	if o.configs == nil && database != nil {
		o.configs = project.NewConfigurationService(database, o.cipher, o.now)
	}
	return o
}

func NewConfigurationOrchestrator(database *store.Store, operations *operation.Service, args ...any) *Orchestrator {
	return NewOrchestrator(database, operations, args...)
}

func randomID() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// QueuePatch validates and durably saves the desired aggregate before the
// runtime operation is started. The revision guard makes concurrent section
// edits fail atomically.
func (o *Orchestrator) QueuePatch(ctx context.Context, projectID string, patch contracts.ConfigurationPatch) (operation.Operation, store.ConfigurationSnapshot, error) {
	if o.configs == nil || o.operations == nil {
		return operation.Operation{}, store.ConfigurationSnapshot{}, errors.New("configuration orchestrator is unavailable")
	}
	o.acquire(projectID)
	lease, err := o.store.AcquireConfigurationLease(ctx, projectID, o.id(), o.now(), 45*time.Minute)
	if err != nil || !lease {
		o.release(projectID)
		if err != nil {
			return operation.Operation{}, store.ConfigurationSnapshot{}, err
		}
		return operation.Operation{}, store.ConfigurationSnapshot{}, errors.New("configuration operation already active")
	}
	snapshot, err := o.configs.Patch(ctx, projectID, patch)
	if err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, err
	}
	queued, err := o.operations.Create(ctx, projectID, operation.TypeUpdateConfig)
	if err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, err
	}
	if err := o.store.BindOperationConfiguration(ctx, queued.ID, projectID, snapshot, o.now()); err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, err
	}
	return queued, snapshot, nil
}

func (o *Orchestrator) acquire(projectID string) {
	o.locksMu.Lock()
	lock := o.locks[projectID]
	if lock == nil {
		lock = make(chan struct{}, 1)
		o.locks[projectID] = lock
	}
	o.locksMu.Unlock()
	lock <- struct{}{}
}
func (o *Orchestrator) release(projectID string) {
	o.locksMu.Lock()
	lock := o.locks[projectID]
	o.locksMu.Unlock()
	if lock != nil {
		select {
		case <-lock:
		default:
		}
	}
}
func (o *Orchestrator) Release(projectID string) {
	o.release(projectID)
	if o.store != nil {
		_ = o.store.ReleaseConfigurationLease(context.Background(), projectID)
	}
}

// Queue is the concise command-style alias used by API adapters.
func (o *Orchestrator) Queue(ctx context.Context, projectID string, patch contracts.ConfigurationPatch) (operation.Operation, store.ConfigurationSnapshot, error) {
	return o.QueuePatch(ctx, projectID, patch)
}

func (o *Orchestrator) QueueDatabasePasswordRotation(ctx context.Context, projectID string) (operation.Operation, store.ConfigurationSnapshot, string, error) {
	if o.configs == nil || o.operations == nil || o.cipher == nil {
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", errors.New("configuration orchestrator is unavailable")
	}
	o.acquire(projectID)
	lease, err := o.store.AcquireConfigurationLease(ctx, projectID, o.id(), o.now(), 45*time.Minute)
	if err != nil || !lease {
		o.release(projectID)
		if err != nil {
			return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
		}
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", errors.New("configuration operation already active")
	}
	current, err := o.configs.GetDesired(ctx, projectID)
	if err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	// Rotating the runtime password is represented by a durable desired revision,
	// but the encrypted row is intentionally replaced only after Reconcile has
	// successfully updated PostgreSQL and its dependents.
	next, err := o.configs.Save(ctx, projectID, current.Revision, current.Configuration)
	if err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	newPassword, err := randomSecret()
	if err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	queued, err := o.operations.Create(ctx, projectID, operation.TypeUpdateConfig)
	if err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	if err := o.store.BindOperationConfiguration(ctx, queued.ID, projectID, next, o.now()); err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	envelope, err := o.cipher.Encrypt(projectID, "operation.database-password", []byte(newPassword))
	if err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	if err := o.store.PutOperationSecret(ctx, queued.ID, "database-password", envelope); err != nil {
		_ = o.store.ReleaseConfigurationLease(ctx, projectID)
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	return queued, next, newPassword, nil
}

func randomSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (o *Orchestrator) RunDatabasePasswordRotation(ctx context.Context, currentProject contracts.Project, queued operation.Operation, snapshot store.ConfigurationSnapshot, newPassword string) (operation.Operation, error) {
	defer o.release(currentProject.ID)
	defer o.store.ReleaseConfigurationLease(context.Background(), currentProject.ID)
	if queued.Status == operation.Queued {
		if err := o.operations.Start(ctx, queued.ID); err != nil {
			return queued, err
		}
	} else if queued.Status != operation.Running {
		return queued, errors.New("rotation operation is not runnable")
	}
	var rotationRequest contracts.RotateDatabasePasswordRequest
	for i, step := range []string{"VALIDATE_CONFIGURATION", "SAVE_CONFIGURATION", "RENDER_RUNTIME", "RECONCILE_SERVICES", "VERIFY_SERVICES", "MARK_CONFIGURATION_GOOD"} {
		progress := (i + 1) * 15
		if err := o.operations.StartStep(ctx, queued.ID, step, progress); err != nil {
			return queued, err
		}
		if i == 0 {
			if err := project.ValidateConfiguration(snapshot.Configuration); err != nil {
				return o.fail(ctx, queued, step, err, false)
			}
		}
		if step == "RECONCILE_SERVICES" {
			secrets, runtimeSecrets, err := o.hydrate(ctx, currentProject.ID, snapshot.Configuration)
			if err != nil {
				return o.fail(ctx, queued, step, errors.New("runtime reconciliation failed"), false)
			}
			rotator, ok := o.provisioner.(PasswordRotationProvisioner)
			if !ok {
				return o.fail(ctx, queued, step, errors.New("database password rotation unavailable"), false)
			}
			oldPassword := secrets.DatabasePassword
			secrets.DatabasePassword = newPassword
			configuration := snapshot.Configuration
			if configuration.Network.APIPort == 0 {
				configuration.Network.APIPort, _ = o.store.ReservedPort(ctx, currentProject.ID, string(ports.KindAPI))
			}
			rotationRequest := contracts.RotateDatabasePasswordRequest{OperationKind: "ROTATE_DATABASE_PASSWORD", OperationID: queued.ID, IdempotencyKey: queued.ID + ":rotate", ProjectID: currentProject.ID, ProjectName: currentProject.Name, Slug: currentProject.Slug, ExpectedRevision: snapshot.Revision - 1, NextRevision: snapshot.Revision, OldPassword: oldPassword, NewPassword: newPassword, Configuration: configuration, Secrets: secrets, RuntimeSecrets: runtimeSecrets}
			result, reconcileErr := rotator.RotateDatabasePassword(ctx, rotationRequest)
			if reconcileErr != nil {
				return o.fail(ctx, queued, step, errors.New("runtime reconciliation failed"), reconcileRollback(reconcileErr))
			}
			if result.OperationID != queued.ID || result.ProjectID != currentProject.ID || result.Revision != snapshot.Revision {
				return o.fail(ctx, queued, step, errors.New("runtime verification failed"), false)
			}
		}
		if step == "MARK_CONFIGURATION_GOOD" {
			envelope, err := o.cipher.Encrypt(currentProject.ID, "database-password", []byte(newPassword))
			if err != nil {
				return o.fail(ctx, queued, step, errors.New("database password rotation failed"), true)
			}
			if err := o.store.PublishConfigurationSecret(ctx, currentProject.ID, snapshot.Revision, "database-password", envelope, o.now()); err != nil {
				rollback := false
				if compensator, ok := o.provisioner.(PasswordRotationRollbackProvisioner); ok {
					rollbackRequest := rotationRequest
					rollbackRequest.OperationKind = "ROLLBACK_DATABASE_PASSWORD"
					rollbackRequest.IdempotencyKey = queued.ID + ":rollback"
					rollbackRequest.OldPassword, rollbackRequest.NewPassword = rollbackRequest.NewPassword, rollbackRequest.OldPassword
					rollback = compensator.RollbackDatabasePassword(ctx, rollbackRequest) == nil
				}
				return o.fail(ctx, queued, step, err, rollback)
			}
		}
		if err := o.operations.CompleteStep(ctx, queued.ID, step, progress); err != nil {
			return queued, err
		}
	}
	if err := o.operations.Succeed(ctx, queued.ID); err != nil {
		return queued, err
	}
	return o.operations.Get(ctx, queued.ID)
}

func reconcileRollback(err error) bool {
	var failure *contracts.ReconcileFailure
	if errors.As(err, &failure) {
		return failure.RollbackSucceeded
	}
	var reporter interface{ RollbackSucceeded() bool }
	return errors.As(err, &reporter) && reporter.RollbackSucceeded()
}

// Run executes the six durable update steps. Runtime failures are intentionally
// reported with a generic message; the Provisioner response is typed and
// redacted, and operation events must never contain rendered environment data.
func (o *Orchestrator) Run(ctx context.Context, currentProject contracts.Project, queued operation.Operation, snapshot store.ConfigurationSnapshot) (operation.Operation, error) {
	defer o.release(currentProject.ID)
	defer o.store.ReleaseConfigurationLease(context.Background(), currentProject.ID)
	if o.provisioner == nil {
		return queued, errors.New("configuration provisioner is unavailable")
	}
	if queued.Status == operation.Queued {
		if err := o.operations.Start(ctx, queued.ID); err != nil {
			return queued, err
		}
	} else if queued.Status != operation.Running {
		return queued, errors.New("configuration operation is not runnable")
	}
	steps := []string{"VALIDATE_CONFIGURATION", "SAVE_CONFIGURATION", "RENDER_RUNTIME", "RECONCILE_SERVICES", "VERIFY_SERVICES", "MARK_CONFIGURATION_GOOD"}
	for i, name := range steps[:3] {
		if err := o.operations.StartStep(ctx, queued.ID, name, (i+1)*10); err != nil {
			return queued, err
		}
		if i == 0 {
			if err := project.ValidateConfiguration(snapshot.Configuration); err != nil {
				return o.fail(ctx, queued, name, err, false)
			}
		}
		if err := o.operations.CompleteStep(ctx, queued.ID, name, (i+1)*10); err != nil {
			return queued, err
		}
	}
	secrets, runtimeSecrets, err := o.hydrate(ctx, currentProject.ID, snapshot.Configuration)
	if err != nil {
		return o.fail(ctx, queued, "RENDER_RUNTIME", err, false)
	}
	request := contracts.ReconcileProjectRequest{
		OperationID: queued.ID, IdempotencyKey: queued.ID + ":reconcile", ProjectID: currentProject.ID,
		ProjectName: currentProject.Name, Slug: currentProject.Slug, ExpectedRevision: snapshot.Revision - 1,
		NextRevision: snapshot.Revision, APIPort: snapshot.Configuration.Network.APIPort,
		Configuration: snapshot.Configuration, Secrets: secrets, RuntimeSecrets: runtimeSecrets,
	}
	if request.APIPort == 0 {
		request.APIPort, _ = o.store.ReservedPort(ctx, currentProject.ID, string(ports.KindAPI))
		request.Configuration.Network.APIPort = request.APIPort
	}
	if err := o.operations.StartStep(ctx, queued.ID, "RECONCILE_SERVICES", 70); err != nil {
		return queued, err
	}
	result, reconcileErr := o.provisioner.Reconcile(ctx, request)
	if reconcileErr != nil {
		rolledBack := reconcileRollback(reconcileErr)
		if rolledBack {
			if restoreErr := o.store.RestoreSecretsRevision(ctx, currentProject.ID, snapshot.Revision); restoreErr != nil {
				rolledBack = false
			}
		}
		return o.fail(ctx, queued, "RECONCILE_SERVICES", errors.New("runtime reconciliation failed"), rolledBack)
	}
	if err := o.operations.CompleteStep(ctx, queued.ID, "RECONCILE_SERVICES", 70); err != nil {
		return queued, err
	}
	if err := o.operations.StartStep(ctx, queued.ID, "VERIFY_SERVICES", 85); err != nil {
		return queued, err
	}
	if result.ProjectID != "" && result.ProjectID != currentProject.ID {
		return o.fail(ctx, queued, "VERIFY_SERVICES", errors.New("runtime verification failed"), false)
	}
	if result.OperationID != queued.ID || result.ProjectID != currentProject.ID || result.Revision != snapshot.Revision || !sameServices(result.EnabledServices, enabledServices(snapshot.Configuration)) {
		return o.fail(ctx, queued, "VERIFY_SERVICES", errors.New("runtime verification failed"), false)
	}
	if err := o.operations.CompleteStep(ctx, queued.ID, "VERIFY_SERVICES", 85); err != nil {
		return queued, err
	}
	if err := o.operations.StartStep(ctx, queued.ID, "MARK_CONFIGURATION_GOOD", 95); err != nil {
		return queued, err
	}
	if err := o.store.MarkConfigurationGood(ctx, currentProject.ID, snapshot.Revision); err != nil {
		return o.fail(ctx, queued, "MARK_CONFIGURATION_GOOD", err, false)
	}
	if err := o.operations.CompleteStep(ctx, queued.ID, "MARK_CONFIGURATION_GOOD", 95); err != nil {
		return queued, err
	}
	if err := o.operations.Succeed(ctx, queued.ID); err != nil {
		return queued, err
	}
	return o.operations.Get(ctx, queued.ID)
}

// Resume restarts queued/running UPDATE_CONFIG operations after a Manager
// restart. The callback keeps project lookup outside this package boundary.
func (o *Orchestrator) Resume(ctx context.Context, lookup func(context.Context, string) (contracts.Project, error)) error {
	active, err := o.operations.ListActive(ctx, operation.TypeUpdateConfig)
	if err != nil {
		return err
	}
	for _, queued := range active {
		p, err := lookup(ctx, queued.ProjectID)
		if err != nil {
			if queued.Status == operation.Queued {
				_ = o.operations.Start(ctx, queued.ID)
			}
			_ = o.operations.Fail(ctx, queued.ID, "RESUME", errors.New("project unavailable during operation resume"))
			continue
		}
		snapshot, err := o.store.GetOperationConfiguration(ctx, queued.ID)
		if err != nil {
			snapshot, err = o.store.GetDesiredConfiguration(ctx, queued.ProjectID)
		}
		if err != nil {
			if queued.Status == operation.Queued {
				_ = o.operations.Start(ctx, queued.ID)
			}
			_ = o.operations.Fail(ctx, queued.ID, "RESUME", errors.New("configuration unavailable during operation resume"))
			continue
		}
		o.acquire(queued.ProjectID)
		lease, leaseErr := o.store.AcquireConfigurationLease(ctx, queued.ProjectID, queued.ID, o.now(), 45*time.Minute)
		if leaseErr != nil || !lease {
			o.release(queued.ProjectID)
			continue
		}
		go func(op operation.Operation, project contracts.Project, snap store.ConfigurationSnapshot) {
			if envelope, secretErr := o.store.GetOperationSecret(context.Background(), op.ID, "database-password"); secretErr == nil {
				if plain, decryptErr := o.cipher.Decrypt(project.ID, "operation.database-password", envelope); decryptErr == nil {
					_, _ = o.RunDatabasePasswordRotation(context.Background(), project, op, snap, string(plain))
					return
				}
			}
			_, _ = o.Run(context.Background(), project, op, snap)
		}(queued, p, snapshot)
	}
	return nil
}

func enabledServices(cfg contracts.ProjectConfiguration) []string {
	result := []string{}
	add := func(name string, enabled bool) {
		if enabled {
			result = append(result, name)
		}
	}
	add("db", cfg.Services.Database)
	add("api-gw", cfg.Services.Gateway)
	add("auth", cfg.Services.Auth)
	add("rest", cfg.Services.REST)
	add("meta", cfg.Services.PostgresMeta)
	add("studio", cfg.Services.Studio)
	add("realtime", cfg.Services.Realtime)
	add("storage", cfg.Services.Storage)
	add("imgproxy", cfg.Services.Imgproxy)
	add("functions", cfg.Services.Functions)
	add("supavisor", cfg.Services.Supavisor)
	add("analytics", cfg.Services.Logs)
	add("vector", cfg.Services.Vector)
	sort.Strings(result)
	return result
}

func sameServices(left, right []string) bool {
	left = canonicalServices(left)
	right = canonicalServices(right)
	a, b := append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

func canonicalServices(values []string) []string {
	result := make([]string, 0, len(values))
	gateway := false
	for _, value := range values {
		if value == "api-gw" || value == "envoy" || value == "kong" || value == "caddy" {
			gateway = true
		} else {
			result = append(result, value)
		}
	}
	if gateway {
		result = append(result, "api-gw")
	}
	return result
}

func (o *Orchestrator) fail(ctx context.Context, queued operation.Operation, step string, cause error, rollback bool) (operation.Operation, error) {
	if !errors.Is(cause, contracts.ErrStaleConfigRevision) && cause.Error() != "runtime reconciliation failed" {
		_ = o.operations.Fail(ctx, queued.ID, step, cause)
	} else {
		_ = o.operations.Fail(ctx, queued.ID, step, errors.New("runtime reconciliation failed"))
	}
	if rollback {
		if o.operations.BeginRollback(ctx, queued.ID) == nil {
			_ = o.operations.CompleteRollback(ctx, queued.ID)
		}
	}
	latest, _ := o.operations.Get(ctx, queued.ID)
	return latest, cause
}

// hydrate decrypts only secrets consumed by the selected configuration. The
// generated internal credentials are included for optional services so a
// manager restart can reconcile an existing project without regenerating them.
func (o *Orchestrator) hydrate(ctx context.Context, projectID string, cfg contracts.ProjectConfiguration) (contracts.ProjectSecrets, map[string]string, error) {
	if o.cipher == nil {
		return contracts.ProjectSecrets{}, nil, errors.New("secret cipher is unavailable")
	}
	baseKinds := []string{"database-password", "jwt-secret", "anon-key", "service-role-key", "dashboard-password", "secret-key-base", "vault-encryption-key"}
	if cfg.Services.Realtime {
		baseKinds = append(baseKinds, "realtime-db-encryption-key")
	}
	if cfg.Services.Logs {
		baseKinds = append(baseKinds, "logflare-public-access-token", "logflare-private-access-token")
	}
	if cfg.Services.Supavisor {
		baseKinds = append(baseKinds, "pooler-tenant-id")
	}
	if cfg.Storage.S3CompatibleAPI {
		baseKinds = append(baseKinds, "s3-protocol-access-key-id", "s3-protocol-access-key-secret")
	}
	if cfg.Auth.SMTP.Enabled && cfg.Auth.SMTP.PasswordSet {
		baseKinds = append(baseKinds, "smtp.password")
	}
	if cfg.Auth.Phone.Enabled && cfg.Auth.Phone.SecretSet {
		baseKinds = append(baseKinds, "phone.secret")
	}
	for name, provider := range cfg.Auth.OAuth {
		if provider.Enabled && provider.SecretSet {
			baseKinds = append(baseKinds, "oauth."+name+".secret")
		}
	}
	if cfg.Storage.SecretAccessKeySet {
		baseKinds = append(baseKinds, "storage.secretAccessKey")
	}
	for _, variable := range cfg.Functions.Variables {
		if cfg.Services.Functions && variable.ValueSet {
			baseKinds = append(baseKinds, "functions."+variable.Name)
		}
	}
	var out contracts.ProjectSecrets
	runtime := make(map[string]string)
	seen := map[string]struct{}{}
	for _, kind := range baseKinds {
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		envelope, err := o.store.GetSecret(ctx, projectID, kind)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return contracts.ProjectSecrets{}, nil, err
		}
		plain, err := o.cipher.Decrypt(projectID, kind, envelope)
		if err != nil {
			return contracts.ProjectSecrets{}, nil, fmt.Errorf("decrypt required secret: %w", err)
		}
		value := string(plain)
		switch kind {
		case "database-password":
			out.DatabasePassword = value
		case "jwt-secret":
			out.JWTSecret = value
		case "anon-key":
			out.AnonKey = value
		case "service-role-key":
			out.ServiceRoleKey = value
		case "dashboard-password":
			out.DashboardPassword = value
		case "secret-key-base":
			out.SecretKeyBase = value
		case "vault-encryption-key":
			out.VaultEncryptionKey = value
		case "realtime-db-encryption-key":
			out.RealtimeDBEncryptionKey = value
			runtime["realtime.dbEncryptionKey"] = value
		case "logflare-public-access-token":
			out.LogflarePublicAccessToken = value
			runtime["logs.publicAccessToken"] = value
		case "logflare-private-access-token":
			out.LogflarePrivateAccessToken = value
			runtime["logs.privateAccessToken"] = value
		case "s3-protocol-access-key-id":
			out.S3ProtocolAccessKeyID = value
			runtime["storage.s3Protocol.accessKey"] = value
		case "s3-protocol-access-key-secret":
			out.S3ProtocolAccessKeySecret = value
			runtime["storage.s3Protocol.secret"] = value
		case "pooler-tenant-id":
			out.PoolerTenantID = value
		default:
			runtime[kind] = value
		}
	}
	return out, runtime, nil
}

func (o *Orchestrator) Reveal(ctx context.Context, projectID, kind string) (string, error) {
	allowed := map[string]string{"anonKey": "anon-key", "serviceRoleKey": "service-role-key", "jwtSecret": "jwt-secret", "databasePassword": "database-password"}
	stored, ok := allowed[strings.TrimSpace(kind)]
	if !ok {
		return "", fmt.Errorf("unsupported secret kind")
	}
	envelope, err := o.store.GetSecret(ctx, projectID, stored)
	if err != nil {
		return "", err
	}
	plain, err := o.cipher.Decrypt(projectID, stored, envelope)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
