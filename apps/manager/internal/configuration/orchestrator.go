// Package configuration coordinates durable configuration revisions with the
// private Provisioner reconciliation API.
package configuration

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
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
	"supabase-manager/internal/authkeys"
	"supabase-manager/internal/contracts"
)

type Provisioner interface {
	Reconcile(context.Context, contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error)
}

type PasswordRotationProvisioner interface {
	RotateDatabasePassword(context.Context, contracts.RotateDatabasePasswordRequest) (contracts.RotateDatabasePasswordResponse, error)
}

type AuthKeysProvisioner interface {
	ReconcileAuthKeys(context.Context, contracts.AuthKeysReconcileRequest) (contracts.ReconcileProjectResponse, error)
}

// PasswordRotationRollbackProvisioner is an explicit compensation boundary
// used if Manager cannot publish the newly rotated encrypted secret.
type PasswordRotationRollbackProvisioner interface {
	RollbackDatabasePassword(context.Context, contracts.RotateDatabasePasswordRequest) error
}

type PasswordRotationPublicationProvisioner interface {
	ConfirmDatabasePasswordRotation(context.Context, contracts.ConfirmDatabasePasswordRotationRequest) error
}

type Orchestrator struct {
	store       *store.Store
	operations  *operation.Service
	configs     *project.ConfigurationService
	allocator   *ports.Allocator
	provisioner Provisioner
	cipher      *managersecrets.Cipher
	now         func() time.Time
	id          func() string
	locksMu     sync.Mutex
	locks       map[string]chan struct{}
	leaseMu     sync.Mutex
	leases      map[string]store.ConfigurationLease
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
	o := &Orchestrator{store: database, operations: operations, now: time.Now, id: randomID, locks: make(map[string]chan struct{}), leases: make(map[string]store.ConfigurationLease)}
	for _, arg := range args {
		switch value := arg.(type) {
		case *project.ConfigurationService:
			o.configs = value
		case *ports.Allocator:
			o.allocator = value
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

// QueuePatch validates and durably saves the canonical aggregate before the
// runtime operation is started. A per-project lock plus the durable active
// operation check serializes concurrent edits without client revision guards.
func (o *Orchestrator) QueuePatch(ctx context.Context, projectID string, patch contracts.ConfigurationPatch) (operation.Operation, store.ConfigurationSnapshot, error) {
	if o.configs == nil || o.operations == nil {
		return operation.Operation{}, store.ConfigurationSnapshot{}, errors.New("configuration orchestrator is unavailable")
	}
	if !o.tryAcquire(projectID) {
		return operation.Operation{}, store.ConfigurationSnapshot{}, store.ErrConfigurationBusy
	}
	// The client revision is advisory only. Read the canonical aggregate while
	// holding the per-project lock and let admission use that value. Older
	// dashboard bundles still send expectedRevision, but it must never block a
	// valid edit against the configuration currently stored by Manager.
	canonical, err := o.configs.Get(ctx, projectID)
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, err
	}
	if active, found, activeErr := o.store.FindActiveOperation(ctx, projectID, operation.TypeUpdateConfig); activeErr != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, activeErr
	} else if found {
		if o.now().Sub(active.CreatedAt) > time.Hour {
			// A worker that has been RUNNING for longer than the durable apply
			// budget cannot safely keep blocking edits (this is how old 70%
			// operations stranded every later update). Close it with an explicit
			// diagnostic, then admit the new canonical value.
			if active.Status == operation.Queued {
				_ = o.operations.Start(ctx, active.ID)
			}
			_ = o.operations.Fail(ctx, active.ID, "RESUME", errors.New("configuration apply worker exceeded one-hour deadline"))
		} else {
			// A Manager restart clears the in-process lock while the durable worker
			// may still be running. Reuse that operation rather than admitting a
			// second candidate and producing a queue of workers stuck at 70%.
			o.release(projectID)
			return active, canonical, nil
		}
	}
	patch.ExpectedRevision = canonical.Revision
	owner := ""
	cfg, err := o.configs.PreparePatch(ctx, projectID, patch)
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, err
	}
	if err := o.allocateUpdatePorts(ctx, projectID, &cfg); err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, err
	}
	mutations, err := o.configs.PrepareSecretMutations(ctx, projectID, &cfg)
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, err
	}
	queued, err := o.operations.NewQueuedOperation(projectID, operation.TypeUpdateConfig)
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, err
	}
	owner = queued.ID
	snapshot, lease, err := o.store.AdmitConfiguration(ctx, store.ConfigurationAdmission{Operation: queued, ProjectID: projectID, Owner: owner, ExpectedRevision: patch.ExpectedRevision, Configuration: cfg, OperationKind: "UPDATE_CONFIG", Mutations: mutations, Now: o.now()})
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, err
	}
	o.leaseMu.Lock()
	o.leases[projectID] = lease
	o.leaseMu.Unlock()
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

func (o *Orchestrator) tryAcquire(projectID string) bool {
	o.locksMu.Lock()
	lock := o.locks[projectID]
	if lock == nil {
		lock = make(chan struct{}, 1)
		o.locks[projectID] = lock
	}
	o.locksMu.Unlock()
	select {
	case lock <- struct{}{}:
		return true
	default:
		return false
	}
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

func (o *Orchestrator) releaseLease(ctx context.Context, projectID string) error {
	o.leaseMu.Lock()
	lease, ok := o.leases[projectID]
	if ok {
		delete(o.leases, projectID)
	}
	o.leaseMu.Unlock()
	if !ok {
		return nil
	}
	return o.store.ReleaseConfigurationLeaseOwned(ctx, projectID, lease.Owner, lease.Fence)
}

func (o *Orchestrator) releaseLeaseToken(ctx context.Context, projectID, owner string, fence int64) error {
	if owner == "" || fence == 0 || o.store == nil {
		return nil
	}
	o.leaseMu.Lock()
	if lease, ok := o.leases[projectID]; ok && lease.Owner == owner && lease.Fence == fence {
		delete(o.leases, projectID)
	}
	o.leaseMu.Unlock()
	return o.store.ReleaseConfigurationLeaseOwned(ctx, projectID, owner, fence)
}

func (o *Orchestrator) renewLease(ctx context.Context, projectID, owner string, fence int64) <-chan struct{} {
	lost := make(chan struct{})
	if owner == "" || fence == 0 {
		close(lost)
		return lost
	}
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()
		defer close(lost)
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if renewed, err := o.store.RenewConfigurationLease(ctx, projectID, owner, fence, now, 45*time.Minute); err != nil || !renewed {
					return
				}
			}
		}
	}()
	return lost
}

func (o *Orchestrator) currentFence(projectID string) int64 {
	o.leaseMu.Lock()
	defer o.leaseMu.Unlock()
	return o.leases[projectID].Fence
}
func (o *Orchestrator) Release(projectID string) {
	o.release(projectID)
	if o.store != nil {
		_ = o.releaseLease(context.Background(), projectID)
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
	if !o.tryAcquire(projectID) {
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", store.ErrConfigurationBusy
	}
	current, err := o.configs.GetDesired(ctx, projectID)
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	newPassword, err := randomSecret()
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	envelope, err := o.cipher.Encrypt(projectID, "operation.database-password", []byte(newPassword))
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	queued, err := o.operations.NewQueuedOperation(projectID, operation.TypeUpdateConfig)
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	next, lease, err := o.store.AdmitConfiguration(ctx, store.ConfigurationAdmission{Operation: queued, ProjectID: projectID, Owner: queued.ID, ExpectedRevision: current.Revision, Configuration: current.Configuration, OperationKind: "ROTATE_DATABASE_PASSWORD", OperationSecrets: map[string]managersecrets.Envelope{"database-password": envelope}, Now: o.now()})
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, "", err
	}
	o.leaseMu.Lock()
	o.leases[projectID] = lease
	o.leaseMu.Unlock()
	return queued, next, newPassword, nil
}

func randomSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

// QueueAuthKeysOperation admits a durable candidate without changing the
// active encrypted bundle. The candidate is encrypted in operation_secrets and
// is published only after Provisioner reconciliation succeeds.
func (o *Orchestrator) QueueAuthKeysOperation(ctx context.Context, projectID, kind string) (operation.Operation, store.ConfigurationSnapshot, contracts.AuthKeysCandidate, error) {
	if o.configs == nil || o.operations == nil || o.cipher == nil {
		return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, errors.New("configuration orchestrator is unavailable")
	}
	if !o.tryAcquire(projectID) {
		return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, store.ErrConfigurationBusy
	}
	current, err := o.configs.GetDesired(ctx, projectID)
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, err
	}
	legacy, _, err := o.hydrate(ctx, projectID, current.Configuration)
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, err
	}
	complete := legacy.SupabasePublishableKey != "" && legacy.SupabaseSecretKey != "" && legacy.AnonKeyAsymmetric != "" && legacy.ServiceRoleKeyAsymmetric != "" && legacy.JWTKeys != "" && legacy.JWTJWKS != ""
	if kind == "MIGRATE_AUTH_KEYS" && complete {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, errors.New("auth key bundle is already complete")
	}
	candidate := contracts.AuthKeysCandidate{SupabasePublishableKey: legacy.SupabasePublishableKey, SupabaseSecretKey: legacy.SupabaseSecretKey, AnonKeyAsymmetric: legacy.AnonKeyAsymmetric, ServiceRoleKeyAsymmetric: legacy.ServiceRoleKeyAsymmetric, JWTKeys: legacy.JWTKeys, JWTJWKS: legacy.JWTJWKS}
	bundle := authkeys.Bundle{SupabasePublishableKey: candidate.SupabasePublishableKey, SupabaseSecretKey: candidate.SupabaseSecretKey, AnonKeyAsymmetric: candidate.AnonKeyAsymmetric, ServiceRoleKeyAsymmetric: candidate.ServiceRoleKeyAsymmetric, JWTKeys: candidate.JWTKeys, JWTJWKS: candidate.JWTJWKS}
	if kind == "ROTATE_API_KEYS" {
		generated, e := authkeys.Generate(rand.Reader, legacy.JWTSecret)
		if e != nil {
			o.release(projectID)
			return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, e
		}
		candidate.SupabasePublishableKey, candidate.SupabaseSecretKey = generated.SupabasePublishableKey, generated.SupabaseSecretKey
	} else {
		generated, e := authkeys.Generate(rand.Reader, legacy.JWTSecret)
		if e != nil {
			o.release(projectID)
			return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, e
		}
		candidate = contracts.AuthKeysCandidate{SupabasePublishableKey: generated.SupabasePublishableKey, SupabaseSecretKey: generated.SupabaseSecretKey, AnonKeyAsymmetric: generated.AnonKeyAsymmetric, ServiceRoleKeyAsymmetric: generated.ServiceRoleKeyAsymmetric, JWTKeys: generated.JWTKeys, JWTJWKS: generated.JWTJWKS}
	}
	if kind == "ROTATE_API_KEYS" {
		if bundle.JWTKeys == "" || bundle.JWTJWKS == "" {
			o.release(projectID)
			return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, errors.New("API rotation requires an existing signing bundle")
		}
	}
	op, err := o.operations.NewQueuedOperation(projectID, operation.TypeUpdateConfig)
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, err
	}
	ops := map[string]managersecrets.Envelope{}
	fields := map[string]string{"publishable-api-key": candidate.SupabasePublishableKey, "secret-api-key": candidate.SupabaseSecretKey, "anon-key-asymmetric": candidate.AnonKeyAsymmetric, "service-role-key-asymmetric": candidate.ServiceRoleKeyAsymmetric, "jwt-keys": candidate.JWTKeys, "jwt-jwks": candidate.JWTJWKS}
	for field, value := range fields {
		env, e := o.cipher.Encrypt(projectID, "operation.auth-key."+field, []byte(value))
		if e != nil {
			o.release(projectID)
			return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, e
		}
		ops[field] = env
	}
	next, lease, err := o.store.AdmitConfiguration(ctx, store.ConfigurationAdmission{Operation: op, ProjectID: projectID, Owner: op.ID, ExpectedRevision: current.Revision, Configuration: current.Configuration, OperationKind: kind, OperationSecrets: ops, Now: o.now()})
	if err != nil {
		o.release(projectID)
		return operation.Operation{}, store.ConfigurationSnapshot{}, contracts.AuthKeysCandidate{}, err
	}
	o.leaseMu.Lock()
	o.leases[projectID] = lease
	o.leaseMu.Unlock()
	return op, next, candidate, nil
}

func (o *Orchestrator) RunAuthKeys(ctx context.Context, p contracts.Project, op operation.Operation, snapshot store.ConfigurationSnapshot, candidate contracts.AuthKeysCandidate) (operation.Operation, error) {
	retainDurableLease := false
	defer func() {
		o.release(p.ID)
		if !retainDurableLease {
			_ = o.releaseLease(context.Background(), p.ID)
		}
	}()
	if op.Status == operation.Queued {
		if err := o.operations.Start(ctx, op.ID); err != nil {
			return op, err
		}
	} else if op.Status != operation.Running {
		return op, errors.New("auth key operation is not runnable")
	}
	if ap := snapshot.Configuration.Network.APIPort; ap == 0 {
		snapshot.Configuration.Network.APIPort, _ = o.store.ReservedPort(ctx, p.ID, string(ports.KindAPI))
	}
	legacy, runtime, err := o.hydrate(ctx, p.ID, snapshot.Configuration)
	if err != nil {
		return o.fail(ctx, op, "RENDER_RUNTIME", err, false)
	}
	req := contracts.ReconcileProjectRequest{OperationID: op.ID, IdempotencyKey: op.ID + ":auth-keys", ProjectID: p.ID, ProjectName: p.Name, Slug: p.Slug, APIPort: snapshot.Configuration.Network.APIPort, Configuration: snapshot.Configuration, Secrets: legacy, RuntimeSecrets: runtime}
	if auth, ok := o.provisioner.(AuthKeysProvisioner); ok {
		result, e := auth.ReconcileAuthKeys(ctx, contracts.AuthKeysReconcileRequest{Request: req, Candidate: candidate})
		if e != nil {
			if shouldRestoreConfiguration(e) {
				if restoreErr := o.restoreConfigurationState(ctx, p.ID, op.ID, snapshot); restoreErr != nil {
					retainDurableLease = true
					return op, restoreErr
				}
			} else {
				// A transport timeout or typed unknown outcome may have applied the
				// candidate. Keep the durable operation and lease for Resume replay.
				if markerErr := o.store.MarkAuthKeysRecoverable(ctx, p.ID, op.ID, snapshot.Fence); markerErr != nil {
					return op, markerErr
				}
				retainDurableLease = true
				return op, e
			}
			return o.fail(ctx, op, "COMPOSE_UP", errors.New("auth key runtime reconciliation failed"), false)
		}
		if result.OperationID != op.ID || result.ProjectID != p.ID || !sameServices(result.EnabledServices, enabledServices(snapshot.Configuration)) {
			// The runtime may already be serving the candidate despite malformed
			// verification metadata. Keep the admitted snapshot and lease for
			// recovery; never delete a potentially active candidate speculatively.
			if markerErr := o.store.MarkAuthKeysRecoverable(ctx, p.ID, op.ID, snapshot.Fence); markerErr != nil {
				return op, markerErr
			}
			retainDurableLease = true
			return op, errors.New("auth key runtime verification failed")
		}
	} else {
		return o.fail(ctx, op, "COMPOSE_UP", errors.New("auth key provisioner is unavailable"), false)
	}
	values := map[string]managersecrets.Envelope{}
	fields := map[string]string{"publishable-api-key": candidate.SupabasePublishableKey, "secret-api-key": candidate.SupabaseSecretKey, "anon-key-asymmetric": candidate.AnonKeyAsymmetric, "service-role-key-asymmetric": candidate.ServiceRoleKeyAsymmetric, "jwt-keys": candidate.JWTKeys, "jwt-jwks": candidate.JWTJWKS}
	for field, value := range fields {
		env, e := o.cipher.Encrypt(p.ID, field, []byte(value))
		if e != nil {
			return o.fail(ctx, op, "PERSIST_CONFIGURATION", errors.New("auth key persistence failed"), false)
		}
		values[field] = env
	}
	if err := o.store.PublishSecretsAndMarkConfigurationGoodOwned(ctx, p.ID, snapshot.Revision, op.ID, snapshot.Fence, "COMMITTED", values, o.now()); err != nil {
		// Runtime reconciliation already succeeded; retain the durable candidate
		// and lease so Resume can retry publication without splitting state.
		if markerErr := o.store.MarkAuthKeysRecoverable(ctx, p.ID, op.ID, snapshot.Fence); markerErr != nil {
			return op, markerErr
		}
		retainDurableLease = true
		return op, err
	}
	if err := o.operations.Succeed(ctx, op.ID); err != nil {
		return op, err
	}
	return o.operations.Get(ctx, op.ID)
}

// allocateUpdatePorts applies the same server-owned allocator used by install
// to installed-project updates. It only chooses candidate values. Canonical
// allocations are promoted or released by the owned publication transaction in the same
// transaction as the desired revision; a failed render therefore cannot
// change the last-good runtime's ports.
func (o *Orchestrator) allocateUpdatePorts(ctx context.Context, projectID string, cfg *contracts.ProjectConfiguration) error {
	if o.allocator == nil {
		return nil
	}
	if !cfg.Services.Studio {
		cfg.Network.StudioPort = 0
	}
	if !cfg.Services.DirectDB {
		cfg.Database.DirectPort = false
		cfg.Database.DirectPortNumber, cfg.Network.DirectDatabasePort = 0, 0
	}
	if !cfg.Services.Supavisor {
		cfg.Pooler.TransactionPort, cfg.Pooler.SessionPort, cfg.Network.PoolerPort = 0, 0, 0
	}
	kinds := []ports.Kind{ports.KindAPI}
	if cfg.Services.Studio {
		kinds = append(kinds, ports.KindStudio)
	}
	if cfg.Services.DirectDB {
		kinds = append(kinds, ports.KindDirectDB)
	}
	if cfg.Services.Supavisor {
		kinds = append(kinds, ports.KindPoolerTxn, ports.KindPoolerSes)
	}
	allocated, err := o.allocator.CandidateMany(ctx, projectID, kinds)
	if err != nil {
		return err
	}
	cfg.Network.APIPort = allocated[ports.KindAPI]
	if cfg.Services.Studio {
		cfg.Network.StudioPort = allocated[ports.KindStudio]
	}
	if cfg.Services.DirectDB {
		cfg.Database.DirectPort = true
		cfg.Database.DirectPortNumber = allocated[ports.KindDirectDB]
		cfg.Network.DirectDatabasePort = allocated[ports.KindDirectDB]
	}
	if cfg.Services.Supavisor {
		cfg.Pooler.TransactionPort = allocated[ports.KindPoolerTxn]
		cfg.Pooler.SessionPort = allocated[ports.KindPoolerSes]
		cfg.Network.PoolerPort = 0
	}
	return nil
}

func (o *Orchestrator) RunDatabasePasswordRotation(ctx context.Context, currentProject contracts.Project, queued operation.Operation, snapshot store.ConfigurationSnapshot, newPassword string) (operation.Operation, error) {
	defer o.release(currentProject.ID)
	defer o.releaseLeaseToken(context.Background(), currentProject.ID, queued.ID, snapshot.Fence)
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	renewCtx, cancelRenew := context.WithCancel(context.Background())
	defer cancelRenew()
	lost := o.renewLease(renewCtx, currentProject.ID, queued.ID, snapshot.Fence)
	go func() {
		select {
		case <-lost:
			cancelWork()
		case <-workCtx.Done():
		}
	}()
	if queued.Status == operation.Queued {
		if err := o.operations.Start(ctx, queued.ID); err != nil {
			return queued, err
		}
	} else if queued.Status != operation.Running {
		return queued, errors.New("rotation operation is not runnable")
	}
	// A worker may have crashed after Manager published the new envelope and
	// while compensation was in flight. Re-enter through the same fenced,
	// idempotent rollback key before attempting any new rotation work.
	compensation, _ := o.store.GetOperationCompensation(ctx, queued.ID)
	if compensation.Phase == "COMMITTED" {
		if published, readErr := o.store.GetConfiguration(ctx, currentProject.ID); readErr == nil && published.LastGoodRevision >= snapshot.Revision {
			if err := o.operations.Succeed(ctx, queued.ID); err != nil {
				return queued, err
			}
			return o.operations.Get(ctx, queued.ID)
		}
	}
	if compensation.Phase == "STATE_RESTORED" {
		return o.fail(ctx, queued, queued.CurrentStep, errors.New("database password rotation compensated"), true)
	}
	if queued.CurrentStep == "MARK_CONFIGURATION_GOOD" && compensation.Phase == "ROLLBACK_CONFIRMED" {
		if err := o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot); err != nil {
			if published, readErr := o.store.GetConfiguration(ctx, currentProject.ID); readErr == nil && published.LastGoodRevision < snapshot.Revision {
				return queued, err
			}
			return o.fail(ctx, queued, queued.CurrentStep, errors.New("database password rotation compensated"), true)
		}
		return o.fail(ctx, queued, queued.CurrentStep, errors.New("database password rotation compensated"), true)
	}
	if queued.CurrentStep == "MARK_CONFIGURATION_GOOD" && (compensation.Phase == "ROLLBACK_PENDING" || compensation.Phase == "RUNTIME_COMMITTED") {
		if old, oldErr := o.rotationOldPassword(ctx, currentProject.ID, snapshot.Revision-1); oldErr == nil {
			request, requestErr := o.rotationRequestForRecovery(ctx, currentProject, queued, snapshot, old, newPassword)
			if requestErr == nil {
				if compensator, ok := o.provisioner.(PasswordRotationRollbackProvisioner); ok {
					request.OperationKind = "ROLLBACK_DATABASE_PASSWORD"
					request.IdempotencyKey = compensation.Key
					if request.IdempotencyKey == "" {
						request.IdempotencyKey = queued.ID + ":rollback"
					}
					request.OldPassword, request.NewPassword = newPassword, old
					if rollbackErr := compensator.RollbackDatabasePassword(ctx, request); rollbackErr == nil {
						if err := o.store.SetOperationCompensation(ctx, queued.ID, "ROLLBACK_CONFIRMED", request.IdempotencyKey); err != nil {
							return queued, err
						}
						if err := o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot); err != nil {
							return queued, err
						}
						return o.fail(ctx, queued, queued.CurrentStep, errors.New("database password rotation compensated"), true)
					} else {
						return queued, rollbackErr
					}
				}
			}
		}
	}
	// A crash after Manager publication but before OPERATION_SUCCEEDED is a
	// successful durable commit. Do not replay ALTER with old==new.
	if published, readErr := o.store.GetConfiguration(ctx, currentProject.ID); readErr == nil && published.LastGoodRevision >= snapshot.Revision {
		if err := o.operations.Succeed(ctx, queued.ID); err != nil {
			return queued, err
		}
		return o.operations.Get(ctx, queued.ID)
	}
	var rotationRequest contracts.RotateDatabasePasswordRequest
	for i, step := range []string{"VALIDATE_CONFIGURATION", "SAVE_CONFIGURATION", "RENDER_RUNTIME", "RECONCILE_SERVICES", "VERIFY_SERVICES", "MARK_CONFIGURATION_GOOD"} {
		progress := (i + 1) * 15
		if err := o.operations.StartStep(ctx, queued.ID, step, progress); err != nil {
			return queued, err
		}
		if i == 0 {
			if err := workCtx.Err(); err != nil {
				_ = o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot)
				return o.fail(ctx, queued, step, errors.New("configuration lease lost"), false)
			}
			if err := project.ValidateStoredConfiguration(snapshot.Configuration); err != nil {
				return o.fail(ctx, queued, step, err, false)
			}
		}
		if step == "RECONCILE_SERVICES" {
			if snapshot.Fence > 0 {
				if owned, _ := o.store.OwnsConfigurationLease(workCtx, currentProject.ID, queued.ID, snapshot.Fence, o.now()); !owned {
					return o.fail(ctx, queued, step, errors.New("configuration lease lost"), false)
				}
			}
			secrets, runtimeSecrets, err := o.hydrate(workCtx, currentProject.ID, snapshot.Configuration)
			if err != nil {
				return o.fail(ctx, queued, step, fmt.Errorf("database password rotation preparation failed: %w", err), false)
			}
			rotator, ok := o.provisioner.(PasswordRotationProvisioner)
			if !ok {
				return o.fail(ctx, queued, step, errors.New("database password rotation unavailable"), false)
			}
			oldPassword := secrets.DatabasePassword
			if revisionEnvelope, revisionErr := o.store.GetSecretAtRevision(workCtx, currentProject.ID, snapshot.Revision-1, "database-password"); revisionErr == nil {
				if plaintext, decryptErr := o.cipher.Decrypt(currentProject.ID, "database-password", revisionEnvelope); decryptErr == nil {
					oldPassword = string(plaintext)
				}
			}
			secrets.DatabasePassword = newPassword
			configuration := snapshot.Configuration
			if configuration.Network.APIPort == 0 {
				configuration.Network.APIPort, _ = o.store.ReservedPort(ctx, currentProject.ID, string(ports.KindAPI))
			}
			rotationRequest = contracts.RotateDatabasePasswordRequest{OperationKind: "ROTATE_DATABASE_PASSWORD", OperationID: queued.ID, IdempotencyKey: queued.ID + ":rotate", ProjectID: currentProject.ID, ProjectName: currentProject.Name, Slug: currentProject.Slug, ExpectedRevision: snapshot.Revision - 1, NextRevision: snapshot.Revision, OldPassword: oldPassword, NewPassword: newPassword, Configuration: configuration, Secrets: secrets, RuntimeSecrets: runtimeSecrets}
			rotationRequest.Fence = snapshot.Fence
			result, reconcileErr := rotator.RotateDatabasePassword(workCtx, rotationRequest)
			if reconcileErr != nil {
				if isRejectedRuntimeRevision(reconcileErr) {
					return o.failRejectedRuntimeRevision(ctx, queued, step, currentProject.ID, snapshot, reconcileErr)
				}
				if !runtimeOutcomeKnown(reconcileErr) {
					// A lost HTTP response is not a runtime failure. Keep the durable
					// operation active so Resume can replay its idempotency key.
					return queued, reconcileErr
				}
				rolledBack := reconcileRollback(reconcileErr)
				if shouldRestoreConfiguration(reconcileErr) {
					if restoreErr := o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot); restoreErr != nil {
						rolledBack = false
					}
				}
				return o.fail(ctx, queued, step, reconcileErr, rolledBack)
			}
			if result.OperationID != queued.ID || result.ProjectID != currentProject.ID || result.Revision != snapshot.Revision {
				return o.fail(ctx, queued, step, errors.New("runtime verification failed"), false)
			}
			if err := o.store.SetOperationCompensation(ctx, queued.ID, "RUNTIME_COMMITTED", queued.ID+":rollback"); err != nil {
				return queued, err
			}
		}
		if step == "MARK_CONFIGURATION_GOOD" {
			envelope, err := o.cipher.Encrypt(currentProject.ID, "database-password", []byte(newPassword))
			if err != nil {
				rollback := false
				if compensator, ok := o.provisioner.(PasswordRotationRollbackProvisioner); ok {
					if err := o.store.SetOperationCompensation(ctx, queued.ID, "ROLLBACK_PENDING", queued.ID+":rollback"); err != nil {
						return queued, err
					}
					rollbackRequest := rotationRequest
					rollbackRequest.OperationKind = "ROLLBACK_DATABASE_PASSWORD"
					rollbackRequest.IdempotencyKey = queued.ID + ":rollback"
					rollbackRequest.OldPassword, rollbackRequest.NewPassword = rollbackRequest.NewPassword, rollbackRequest.OldPassword
					rollbackErr := compensator.RollbackDatabasePassword(ctx, rollbackRequest)
					if rollbackErr != nil {
						return queued, rollbackErr
					}
					rollback = rollbackErr == nil
				}
				if rollback {
					if err := o.store.SetOperationCompensation(ctx, queued.ID, "ROLLBACK_CONFIRMED", queued.ID+":rollback"); err != nil {
						return queued, err
					}
					if restoreErr := o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot); restoreErr != nil {
						rollback = false
					}
				}
				return o.fail(ctx, queued, step, errors.New("database password rotation failed"), rollback)
			}
			if err := o.store.PublishConfigurationSecret(ctx, currentProject.ID, snapshot.Revision, "database-password", envelope, o.now()); err != nil {
				rollback := false
				if compensator, ok := o.provisioner.(PasswordRotationRollbackProvisioner); ok {
					if err := o.store.SetOperationCompensation(ctx, queued.ID, "ROLLBACK_PENDING", queued.ID+":rollback"); err != nil {
						return queued, err
					}
					rollbackRequest := rotationRequest
					rollbackRequest.OperationKind = "ROLLBACK_DATABASE_PASSWORD"
					rollbackRequest.IdempotencyKey = queued.ID + ":rollback"
					rollbackRequest.OldPassword, rollbackRequest.NewPassword = rollbackRequest.NewPassword, rollbackRequest.OldPassword
					rollbackErr := compensator.RollbackDatabasePassword(ctx, rollbackRequest)
					if rollbackErr != nil {
						return queued, rollbackErr
					}
					rollback = rollbackErr == nil
				}
				if rollback {
					if err := o.store.SetOperationCompensation(ctx, queued.ID, "ROLLBACK_CONFIRMED", queued.ID+":rollback"); err != nil {
						return queued, err
					}
					if restoreErr := o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot); restoreErr != nil {
						rollback = false
					}
				}
				return o.fail(ctx, queued, step, err, rollback)
			}
			if confirmer, ok := o.provisioner.(PasswordRotationPublicationProvisioner); ok {
				confirmation := contracts.ConfirmDatabasePasswordRotationRequest{OperationID: queued.ID, IdempotencyKey: queued.ID + ":confirm", ProjectID: currentProject.ID, Slug: currentProject.Slug, ExpectedRevision: snapshot.Revision - 1, NextRevision: snapshot.Revision, Fence: snapshot.Fence}
				if err := confirmer.ConfirmDatabasePasswordRotation(ctx, confirmation); err != nil {
					rollback := false
					if compensator, ok := o.provisioner.(PasswordRotationRollbackProvisioner); ok {
						if err := o.store.SetOperationCompensation(ctx, queued.ID, "ROLLBACK_PENDING", queued.ID+":rollback"); err != nil {
							return queued, err
						}
						rollbackRequest := rotationRequest
						rollbackRequest.OperationKind = "ROLLBACK_DATABASE_PASSWORD"
						rollbackRequest.IdempotencyKey = queued.ID + ":rollback"
						rollbackRequest.OldPassword, rollbackRequest.NewPassword = rollbackRequest.NewPassword, rollbackRequest.OldPassword
						rollbackErr := compensator.RollbackDatabasePassword(ctx, rollbackRequest)
						if rollbackErr != nil {
							return queued, rollbackErr
						}
						rollback = rollbackErr == nil
					}
					if rollback {
						if err := o.store.SetOperationCompensation(ctx, queued.ID, "ROLLBACK_CONFIRMED", queued.ID+":rollback"); err != nil {
							return queued, err
						}
						if restoreErr := o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot); restoreErr != nil {
							rollback = false
						}
					}
					return o.fail(ctx, queued, step, errors.New("database password rotation confirmation failed"), rollback)
				}
			}
			if err := o.store.MarkConfigurationGoodOwned(ctx, currentProject.ID, snapshot.Revision, queued.ID, snapshot.Fence, "COMMITTED", o.now()); err != nil {
				rollback := false
				if compensator, ok := o.provisioner.(PasswordRotationRollbackProvisioner); ok {
					if err := o.store.SetOperationCompensation(ctx, queued.ID, "ROLLBACK_PENDING", queued.ID+":rollback"); err != nil {
						return queued, err
					}
					rollbackRequest := rotationRequest
					rollbackRequest.OperationKind = "ROLLBACK_DATABASE_PASSWORD"
					rollbackRequest.IdempotencyKey = queued.ID + ":rollback"
					rollbackRequest.OldPassword, rollbackRequest.NewPassword = rollbackRequest.NewPassword, rollbackRequest.OldPassword
					rollbackErr := compensator.RollbackDatabasePassword(ctx, rollbackRequest)
					if rollbackErr != nil {
						return queued, rollbackErr
					}
					rollback = rollbackErr == nil
				}
				if rollback {
					if err := o.store.SetOperationCompensation(ctx, queued.ID, "ROLLBACK_CONFIRMED", queued.ID+":rollback"); err != nil {
						return queued, err
					}
					if restoreErr := o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot); restoreErr != nil {
						rollback = false
					}
				}
				return o.fail(ctx, queued, step, errors.New("configuration publication failed"), rollback)
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

// shouldRestoreConfiguration closes the Manager-side admission transaction
// for failures before runtime publication, or after a confirmed rollback.
// Unknown transport outcomes are left active for recovery rather than guessed.
func shouldRestoreConfiguration(err error) bool {
	var failure *contracts.ReconcileFailure
	if errors.As(err, &failure) {
		return !failure.RuntimeChanged || failure.RollbackSucceeded
	}
	var outcome interface {
		RuntimeOutcomeKnown() bool
		RuntimeChanged() bool
		RollbackSucceeded() bool
	}
	if errors.As(err, &outcome) {
		return outcome.RuntimeOutcomeKnown() && (!outcome.RuntimeChanged() || outcome.RollbackSucceeded())
	}
	return false
}

func runtimeOutcomeKnown(err error) bool {
	var failure *contracts.ReconcileFailure
	if errors.As(err, &failure) {
		return true
	}
	var outcome interface{ RuntimeOutcomeKnown() bool }
	return errors.As(err, &outcome) && outcome.RuntimeOutcomeKnown()
}

// isRejectedRuntimeRevision identifies Provisioner rejections made before it
// can render, switch a runtime generation, or recreate a service. Unlike a
// transport failure, these responses are deterministic: preserving the
// operation as RUNNING would replay the same rejected request forever.
func isRejectedRuntimeRevision(err error) bool {
	return errors.Is(err, contracts.ErrStaleConfigRevision) || errors.Is(err, contracts.ErrInvalidReconcileRevision)
}

func (o *Orchestrator) failRejectedRuntimeRevision(ctx context.Context, queued operation.Operation, step, projectID string, snapshot store.ConfigurationSnapshot, reconcileErr error) (operation.Operation, error) {
	message := "runtime configuration revision is invalid"
	if errors.Is(reconcileErr, contracts.ErrStaleConfigRevision) {
		message = "runtime configuration revision conflict"
	}
	if restoreErr := o.restoreConfigurationState(ctx, projectID, queued.ID, snapshot); restoreErr != nil {
		return o.fail(ctx, queued, step, fmt.Errorf("%s: %w; candidate configuration could not be restored: %v", message, reconcileErr, restoreErr), false)
	}
	return o.fail(ctx, queued, step, fmt.Errorf("%s: %w; candidate configuration restored", message, reconcileErr), false)
}

// Run applies the canonical configuration directly. The legacy revisioned
// implementation remains below only as an isolated compatibility helper for
// password-rotation recovery; normal Dashboard updates never enter it.
func (o *Orchestrator) Run(ctx context.Context, currentProject contracts.Project, queued operation.Operation, _ store.ConfigurationSnapshot) (operation.Operation, error) {
	if o.configs == nil {
		return queued, errors.New("configuration orchestrator is unavailable")
	}
	snapshot, err := o.configs.Get(ctx, currentProject.ID)
	if err != nil {
		return queued, err
	}
	defer o.release(currentProject.ID)
	// QueuePatch keeps the lease in the orchestrator's in-memory registry. The
	// canonical snapshot intentionally omits fencing metadata, so releasing by
	// snapshot.Fence would leak the durable lease after every successful update.
	defer o.releaseLease(context.Background(), currentProject.ID)
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
	steps := []struct {
		name     string
		progress int
	}{
		{"VALIDATE_CONFIGURATION", 10},
		{"PERSIST_CONFIGURATION", 25},
		{"RENDER_RUNTIME", 40},
	}
	for _, step := range steps {
		if err := o.operations.StartStep(ctx, queued.ID, step.name, step.progress); err != nil {
			return queued, err
		}
		if step.name == "VALIDATE_CONFIGURATION" {
			if err := project.ValidateStoredConfiguration(snapshot.Configuration); err != nil {
				return o.fail(ctx, queued, step.name, err, false)
			}
		}
		if err := o.operations.CompleteStep(ctx, queued.ID, step.name, step.progress); err != nil {
			return queued, err
		}
	}
	secrets, runtimeSecrets, err := o.hydrate(ctx, currentProject.ID, snapshot.Configuration)
	if err != nil {
		return o.fail(ctx, queued, "RENDER_RUNTIME", err, false)
	}
	if err := o.operations.StartStep(ctx, queued.ID, "COMPOSE_UP", 70); err != nil {
		return queued, err
	}
	request := contracts.ReconcileProjectRequest{
		OperationID: queued.ID, IdempotencyKey: queued.ID + ":reconcile", ProjectID: currentProject.ID,
		ProjectName: currentProject.Name, Slug: currentProject.Slug, APIPort: snapshot.Configuration.Network.APIPort,
		Configuration: snapshot.Configuration, Secrets: secrets, RuntimeSecrets: runtimeSecrets,
	}
	if request.APIPort == 0 {
		request.APIPort, _ = o.store.ReservedPort(ctx, currentProject.ID, string(ports.KindAPI))
		request.Configuration.Network.APIPort = request.APIPort
	}
	result, reconcileErr := o.provisioner.Reconcile(ctx, request)
	if reconcileErr != nil {
		return o.fail(ctx, queued, "COMPOSE_UP", reconcileErr, reconcileRollback(reconcileErr))
	}
	if err := o.operations.CompleteStep(ctx, queued.ID, "COMPOSE_UP", 70); err != nil {
		return queued, err
	}
	if err := o.operations.StartStep(ctx, queued.ID, "VERIFY_SERVICES", 90); err != nil {
		return queued, err
	}
	if result.OperationID != queued.ID || result.ProjectID != currentProject.ID || !sameServices(result.EnabledServices, enabledServices(snapshot.Configuration)) {
		// Revision numbers belong to the retired optimistic-concurrency protocol.
		// Runtime metadata may advance independently (for example after a manual
		// Docker restart); identity and enabled-service health are the only
		// verification invariants for a canonical apply.
		return o.fail(ctx, queued, "VERIFY_SERVICES", runtimeVerificationError(result, queued.ID, currentProject.ID, -1, snapshot.Configuration), false)
	}
	if err := o.operations.CompleteStep(ctx, queued.ID, "VERIFY_SERVICES", 90); err != nil {
		return queued, err
	}
	if err := o.store.CommitCanonicalConfiguration(ctx, currentProject.ID, o.now()); err != nil {
		return o.fail(ctx, queued, "VERIFY_SERVICES", err, false)
	}
	if err := o.operations.Succeed(ctx, queued.ID); err != nil {
		return queued, err
	}
	return o.operations.Get(ctx, queued.ID)
}

// runLegacy is retained solely for source compatibility with password
// rotation recovery while existing installations migrate. It is not called
// by normal configuration PATCH operations.
func (o *Orchestrator) runLegacy(ctx context.Context, currentProject contracts.Project, queued operation.Operation, snapshot store.ConfigurationSnapshot) (operation.Operation, error) {
	defer o.release(currentProject.ID)
	defer o.releaseLeaseToken(context.Background(), currentProject.ID, queued.ID, snapshot.Fence)
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	renewCtx, cancelRenew := context.WithCancel(context.Background())
	defer cancelRenew()
	lost := o.renewLease(renewCtx, currentProject.ID, queued.ID, snapshot.Fence)
	go func() {
		select {
		case <-lost:
			cancelWork()
		case <-workCtx.Done():
		}
	}()
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
	compensation, _ := o.store.GetOperationCompensation(ctx, queued.ID)
	if compensation.Phase == "COMMITTED" {
		if published, readErr := o.store.GetConfiguration(ctx, currentProject.ID); readErr == nil && published.LastGoodRevision >= snapshot.Revision {
			if err := o.operations.Succeed(ctx, queued.ID); err != nil {
				return queued, err
			}
			return o.operations.Get(ctx, queued.ID)
		}
	}
	if compensation.Phase == "STATE_RESTORED" {
		return o.fail(ctx, queued, queued.CurrentStep, errors.New("configuration candidate restored"), true)
	}
	steps := []string{"VALIDATE_CONFIGURATION", "SAVE_CONFIGURATION", "RENDER_RUNTIME", "RECONCILE_SERVICES", "VERIFY_SERVICES", "MARK_CONFIGURATION_GOOD"}
	for i, name := range steps[:3] {
		if err := o.operations.StartStep(ctx, queued.ID, name, (i+1)*10); err != nil {
			return queued, err
		}
		if i == 0 {
			if err := project.ValidateStoredConfiguration(snapshot.Configuration); err != nil {
				_ = o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot)
				return o.fail(ctx, queued, name, err, false)
			}
		}
		if err := o.operations.CompleteStep(ctx, queued.ID, name, (i+1)*10); err != nil {
			return queued, err
		}
	}
	if err := workCtx.Err(); err != nil {
		_ = o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot)
		return o.fail(ctx, queued, "RENDER_RUNTIME", errors.New("configuration lease lost"), false)
	}
	secrets, runtimeSecrets, err := o.hydrate(workCtx, currentProject.ID, snapshot.Configuration)
	if err != nil {
		_ = o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot)
		return o.fail(ctx, queued, "RENDER_RUNTIME", err, false)
	}
	request := contracts.ReconcileProjectRequest{
		OperationID: queued.ID, IdempotencyKey: queued.ID + ":reconcile", ProjectID: currentProject.ID,
		ProjectName: currentProject.Name, Slug: currentProject.Slug, ExpectedRevision: snapshot.Revision - 1,
		NextRevision: snapshot.Revision, APIPort: snapshot.Configuration.Network.APIPort,
		Configuration: snapshot.Configuration, Secrets: secrets, RuntimeSecrets: runtimeSecrets,
	}
	request.Fence = snapshot.Fence
	if request.APIPort == 0 {
		request.APIPort, _ = o.store.ReservedPort(ctx, currentProject.ID, string(ports.KindAPI))
		request.Configuration.Network.APIPort = request.APIPort
	}
	if err := o.operations.StartStep(ctx, queued.ID, "RECONCILE_SERVICES", 70); err != nil {
		return queued, err
	}
	slog.Info("configuration reconciliation requested", "project_id", currentProject.ID, "slug", currentProject.Slug, "operation_id", queued.ID, "revision", snapshot.Revision)
	result, reconcileErr := o.provisioner.Reconcile(workCtx, request)
	if reconcileErr != nil {
		slog.Error("configuration reconciliation returned an error", "project_id", currentProject.ID, "slug", currentProject.Slug, "operation_id", queued.ID, "error", reconcileErr)
		if isRejectedRuntimeRevision(reconcileErr) {
			return o.failRejectedRuntimeRevision(ctx, queued, "RECONCILE_SERVICES", currentProject.ID, snapshot, reconcileErr)
		}
		if !runtimeOutcomeKnown(reconcileErr) {
			// Preserve RUNNING for scheduler/startup recovery when the private
			// request may have completed but its response was lost.
			return queued, reconcileErr
		}
		rolledBack := reconcileRollback(reconcileErr)
		if shouldRestoreConfiguration(reconcileErr) {
			if restoreErr := o.restoreConfigurationState(ctx, currentProject.ID, queued.ID, snapshot); restoreErr != nil {
				rolledBack = false
			}
		}
		return o.fail(ctx, queued, "RECONCILE_SERVICES", reconcileErr, rolledBack)
	}
	slog.Info("configuration reconciliation completed", "project_id", currentProject.ID, "slug", currentProject.Slug, "operation_id", queued.ID, "revision", result.Revision, "recreated_services", result.RecreatedServices)
	if err := o.operations.CompleteStep(ctx, queued.ID, "RECONCILE_SERVICES", 70); err != nil {
		return queued, err
	}
	if err := o.operations.StartStep(ctx, queued.ID, "VERIFY_SERVICES", 85); err != nil {
		return queued, err
	}
	if result.ProjectID != "" && result.ProjectID != currentProject.ID {
		return o.fail(ctx, queued, "VERIFY_SERVICES", runtimeVerificationError(result, queued.ID, currentProject.ID, snapshot.Revision, snapshot.Configuration), false)
	}
	if result.OperationID != queued.ID || result.ProjectID != currentProject.ID || result.Revision != snapshot.Revision || !sameServices(result.EnabledServices, enabledServices(snapshot.Configuration)) {
		return o.fail(ctx, queued, "VERIFY_SERVICES", runtimeVerificationError(result, queued.ID, currentProject.ID, snapshot.Revision, snapshot.Configuration), false)
	}
	if err := o.operations.CompleteStep(ctx, queued.ID, "VERIFY_SERVICES", 85); err != nil {
		return queued, err
	}
	if err := o.operations.StartStep(ctx, queued.ID, "MARK_CONFIGURATION_GOOD", 95); err != nil {
		return queued, err
	}
	if snapshot.Fence > 0 {
		if owned, _ := o.store.OwnsConfigurationLease(workCtx, currentProject.ID, queued.ID, snapshot.Fence, o.now()); !owned {
			return o.fail(ctx, queued, "MARK_CONFIGURATION_GOOD", errors.New("configuration lease lost"), false)
		}
	}
	if err := o.store.MarkConfigurationGoodOwned(ctx, currentProject.ID, snapshot.Revision, queued.ID, snapshot.Fence, "COMMITTED", o.now()); err != nil {
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
			_ = o.operations.Fail(ctx, queued.ID, "RESUME", errors.New("Server unavailable during operation resume"))
			continue
		}
		snapshot, err := o.store.GetOperationConfiguration(ctx, queued.ID)
		if err != nil {
			if queued.Status == operation.Queued {
				_ = o.operations.Start(ctx, queued.ID)
			}
			_ = o.operations.Fail(ctx, queued.ID, "RESUME", errors.New("configuration unavailable during operation resume"))
			continue
		}
		kind, kindErr := o.store.GetOperationKind(ctx, queued.ID)
		if kindErr != nil {
			if queued.Status == operation.Queued {
				_ = o.operations.Start(ctx, queued.ID)
			}
			_ = o.operations.Fail(ctx, queued.ID, "RESUME", errors.New("operation command is unavailable"))
			continue
		}
		if !o.tryAcquire(queued.ProjectID) {
			continue
		}
		lease, acquired, leaseErr := o.store.AcquireConfigurationLeaseForOperation(ctx, queued.ProjectID, queued.ID, queued.ID, o.now(), 45*time.Minute)
		if leaseErr != nil || !acquired {
			o.release(queued.ProjectID)
			continue
		}
		snapshot.Fence = lease.Fence
		o.leaseMu.Lock()
		o.leases[queued.ProjectID] = lease
		o.leaseMu.Unlock()
		go func(op operation.Operation, project contracts.Project, snap store.ConfigurationSnapshot, commandKind string, leaseFence int64) {
			// Resume owns the admission resources before payload decoding. Keep a
			// single scope around every exit, including corrupt/missing payloads.
			defer o.release(project.ID)
			if commandKind != "MIGRATE_AUTH_KEYS" && commandKind != "ROTATE_API_KEYS" && commandKind != "ROTATE_SIGNING_KEYS" {
				defer o.releaseLeaseToken(context.Background(), project.ID, op.ID, leaseFence)
			}
			if commandKind != "UPDATE_CONFIG" && commandKind != "ROTATE_DATABASE_PASSWORD" && commandKind != "MIGRATE_AUTH_KEYS" && commandKind != "ROTATE_API_KEYS" && commandKind != "ROTATE_SIGNING_KEYS" {
				if op.Status == operation.Queued {
					_ = o.operations.Start(context.Background(), op.ID)
				}
				_ = o.operations.Fail(context.Background(), op.ID, "RESUME", errors.New("unsupported operation command"))
				return
			}
			if commandKind == "ROTATE_DATABASE_PASSWORD" {
				if o.cipher == nil {
					if op.Status == operation.Queued {
						_ = o.operations.Start(context.Background(), op.ID)
					}
					_ = o.operations.Fail(context.Background(), op.ID, "RESUME", errors.New("rotation command payload is invalid"))
					return
				}
				envelope, secretErr := o.store.GetOperationSecret(context.Background(), op.ID, "database-password")
				if secretErr != nil {
					if op.Status == operation.Queued {
						_ = o.operations.Start(context.Background(), op.ID)
					}
					_ = o.operations.Fail(context.Background(), op.ID, "RESUME", errors.New("rotation command payload is unavailable"))
					return
				}
				plain, decryptErr := o.cipher.Decrypt(project.ID, "operation.database-password", envelope)
				if decryptErr != nil {
					if op.Status == operation.Queued {
						_ = o.operations.Start(context.Background(), op.ID)
					}
					_ = o.operations.Fail(context.Background(), op.ID, "RESUME", errors.New("rotation command payload is invalid"))
					return
				}
				_, _ = o.RunDatabasePasswordRotation(context.Background(), project, op, snap, string(plain))
				return
			}
			if commandKind == "MIGRATE_AUTH_KEYS" || commandKind == "ROTATE_API_KEYS" || commandKind == "ROTATE_SIGNING_KEYS" {
				candidate, decodeErr := o.authKeysOperationCandidate(context.Background(), project.ID, op.ID)
				if decodeErr != nil {
					if op.Status == operation.Queued {
						_ = o.operations.Start(context.Background(), op.ID)
					}
					_ = o.operations.Fail(context.Background(), op.ID, "RESUME", errors.New("auth key command payload is unavailable"))
					_ = o.releaseLeaseToken(context.Background(), project.ID, op.ID, lease.Fence)
					return
				}
				_, _ = o.RunAuthKeys(context.Background(), project, op, snap, candidate)
				return
			}
			_, _ = o.Run(context.Background(), project, op, snap)
		}(queued, p, snapshot, kind, lease.Fence)
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
		if isRendererHelperService(value) {
			continue
		}
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

// Renderer helper services are Compose implementation details rather than
// project capabilities. They are created as dependencies of Auth, Functions,
// Supavisor, and Logs, so they must not make the Manager reject an otherwise
// healthy configuration revision.
func isRendererHelperService(name string) bool {
	switch name {
	case "auth-templates", "deno-cache", "db-config", "logflare":
		return true
	default:
		return false
	}
}

func runtimeVerificationError(result contracts.ReconcileProjectResponse, operationID, projectID string, revision int64, cfg contracts.ProjectConfiguration) error {
	mismatches := make([]string, 0, 4)
	if result.OperationID != operationID {
		mismatches = append(mismatches, fmt.Sprintf("operation ID received=%q expected=%q", result.OperationID, operationID))
	}
	if result.ProjectID != projectID {
		mismatches = append(mismatches, fmt.Sprintf("server ID received=%q expected=%q", result.ProjectID, projectID))
	}
	if revision >= 0 && result.Revision != revision {
		mismatches = append(mismatches, fmt.Sprintf("revision received=%d expected=%d", result.Revision, revision))
	}
	expected := enabledServices(cfg)
	if !sameServices(result.EnabledServices, expected) {
		actual := canonicalServices(result.EnabledServices)
		sort.Strings(actual)
		sort.Strings(expected)
		mismatches = append(mismatches, fmt.Sprintf("enabled services mismatch: received=%v expected=%v", actual, expected))
	}
	if len(mismatches) == 0 {
		return errors.New("runtime verification failed: provisioner returned an inconsistent result")
	}
	return fmt.Errorf("runtime verification failed: %s", strings.Join(mismatches, "; "))
}

func (o *Orchestrator) fail(ctx context.Context, queued operation.Operation, step string, cause error, rollback bool) (operation.Operation, error) {
	_ = o.operations.Fail(ctx, queued.ID, step, cause)
	if rollback {
		if o.operations.BeginRollback(ctx, queued.ID) == nil {
			_ = o.operations.CompleteRollback(ctx, queued.ID)
		}
	}
	latest, _ := o.operations.Get(ctx, queued.ID)
	return latest, cause
}

func (o *Orchestrator) restoreConfigurationState(ctx context.Context, projectID, owner string, snapshot store.ConfigurationSnapshot) error {
	return o.store.RestoreConfigurationStateOwned(ctx, projectID, snapshot.Revision, owner, snapshot.Fence, o.now())
}

func (o *Orchestrator) rotationOldPassword(ctx context.Context, projectID string, revision int64) (string, error) {
	envelope, err := o.store.GetSecretAtRevision(ctx, projectID, revision, "database-password")
	if err != nil {
		return "", err
	}
	plain, err := o.cipher.Decrypt(projectID, "database-password", envelope)
	return string(plain), err
}

func (o *Orchestrator) rotationRequestForRecovery(ctx context.Context, currentProject contracts.Project, queued operation.Operation, snapshot store.ConfigurationSnapshot, oldPassword, newPassword string) (contracts.RotateDatabasePasswordRequest, error) {
	secrets, runtimeSecrets, err := o.hydrate(ctx, currentProject.ID, snapshot.Configuration)
	if err != nil {
		return contracts.RotateDatabasePasswordRequest{}, err
	}
	return contracts.RotateDatabasePasswordRequest{
		OperationID: queued.ID, ProjectID: currentProject.ID, ProjectName: currentProject.Name, Slug: currentProject.Slug,
		ExpectedRevision: snapshot.Revision - 1, NextRevision: snapshot.Revision, OldPassword: oldPassword, NewPassword: newPassword,
		Configuration: snapshot.Configuration, Secrets: secrets, RuntimeSecrets: runtimeSecrets, Fence: snapshot.Fence,
	}, nil
}

// hydrate decrypts only secrets consumed by the selected configuration. The
// generated internal credentials are included for optional services so a
// manager restart can reconcile an existing project without regenerating them.
func (o *Orchestrator) hydrate(ctx context.Context, projectID string, cfg contracts.ProjectConfiguration) (contracts.ProjectSecrets, map[string]string, error) {
	if o.cipher == nil {
		return contracts.ProjectSecrets{}, nil, errors.New("secret cipher is unavailable")
	}
	baseKinds := []string{"database-password", "jwt-secret", "anon-key", "service-role-key", "dashboard-password", "secret-key-base", "vault-encryption-key", "publishable-api-key", "secret-api-key", "anon-key-asymmetric", "service-role-key-asymmetric", "jwt-keys", "jwt-jwks"}
	if cfg.General.StudioPasswordSet {
		baseKinds = append(baseKinds, "studio.password")
	}
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
	if cfg.Services.Auth && cfg.Auth.SMTP.Enabled && cfg.Auth.SMTP.PasswordSet {
		baseKinds = append(baseKinds, "smtp.password")
	}
	if cfg.Services.Auth && cfg.Auth.Phone.Enabled && cfg.Auth.Phone.SecretSet {
		baseKinds = append(baseKinds, "phone.secret")
	}
	for name, provider := range cfg.Auth.OAuth {
		if cfg.Services.Auth && provider.Enabled && provider.SecretSet {
			baseKinds = append(baseKinds, "oauth."+name+".secret")
		}
	}
	if cfg.Services.Storage && cfg.Storage.SecretAccessKeySet {
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
			if kind == "studio.password" && out.DashboardPassword == "" {
				return contracts.ProjectSecrets{}, nil, errors.New("configured Studio password is unavailable")
			}
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
		case "studio.password":
			out.DashboardPassword = value
		case "secret-key-base":
			out.SecretKeyBase = value
		case "vault-encryption-key":
			out.VaultEncryptionKey = value
		case "publishable-api-key":
			out.SupabasePublishableKey = value
		case "secret-api-key":
			out.SupabaseSecretKey = value
		case "anon-key-asymmetric":
			out.AnonKeyAsymmetric = value
		case "service-role-key-asymmetric":
			out.ServiceRoleKeyAsymmetric = value
		case "jwt-keys":
			out.JWTKeys = value
		case "jwt-jwks":
			out.JWTJWKS = value
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
	bundle := authkeys.Bundle{SupabasePublishableKey: out.SupabasePublishableKey, SupabaseSecretKey: out.SupabaseSecretKey, AnonKeyAsymmetric: out.AnonKeyAsymmetric, ServiceRoleKeyAsymmetric: out.ServiceRoleKeyAsymmetric, JWTKeys: out.JWTKeys, JWTJWKS: out.JWTJWKS}
	count := 0
	for _, value := range []string{bundle.SupabasePublishableKey, bundle.SupabaseSecretKey, bundle.AnonKeyAsymmetric, bundle.ServiceRoleKeyAsymmetric, bundle.JWTKeys, bundle.JWTJWKS} {
		if value != "" {
			count++
		}
	}
	if count != 0 && (count != 6 || bundle.Validate(out.JWTSecret) != nil) {
		return contracts.ProjectSecrets{}, nil, errors.New("invalid asymmetric auth key bundle")
	}
	return out, runtime, nil
}

func (o *Orchestrator) authKeysOperationCandidate(ctx context.Context, projectID, operationID string) (contracts.AuthKeysCandidate, error) {
	var out contracts.AuthKeysCandidate
	fields := map[string]*string{"publishable-api-key": &out.SupabasePublishableKey, "secret-api-key": &out.SupabaseSecretKey, "anon-key-asymmetric": &out.AnonKeyAsymmetric, "service-role-key-asymmetric": &out.ServiceRoleKeyAsymmetric, "jwt-keys": &out.JWTKeys, "jwt-jwks": &out.JWTJWKS}
	for kind, target := range fields {
		envelope, err := o.store.GetOperationSecret(ctx, operationID, kind)
		if err != nil {
			return out, err
		}
		plain, err := o.cipher.Decrypt(projectID, "operation.auth-key."+kind, envelope)
		if err != nil {
			return out, err
		}
		*target = string(plain)
	}
	return out, nil
}

func (o *Orchestrator) Reveal(ctx context.Context, projectID, kind string) (string, error) {
	allowed := map[string]string{
		"anonKey":             "anon-key",
		"serviceRoleKey":      "service-role-key",
		"jwtSecret":           "jwt-secret",
		"databasePassword":    "database-password",
		"publishable-api-key": "publishable-api-key",
		"secret-api-key":      "secret-api-key",
	}
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
