package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/internal/contracts"
)

var ErrStaleConfiguration = errors.New("stale project configuration revision")
var ErrConfigurationConflict = errors.New("project configuration conflicts with another project")
var ErrConfigurationBusy = errors.New("project configuration operation is busy")
var ErrSecretSnapshotUnavailable = errors.New("encrypted secret snapshot is unavailable")

type ConfigurationSnapshot struct {
	ProjectID        string                         `json:"projectId"`
	Revision         int64                          `json:"revision"`
	LastGoodRevision int64                          `json:"lastGoodRevision"`
	Configuration    contracts.ProjectConfiguration `json:"configuration"`
	Fence            int64                          `json:"-"`
}

// SecretMutation is an already-encrypted change applied in the same transaction
// as its configuration revision.
type SecretMutation struct {
	Kind     string
	Envelope secrets.Envelope
	Delete   bool
}

type ConfigurationLease struct {
	Owner string
	Fence int64
}

type OperationCompensation struct {
	Phase string
	Key   string
}

func validateConfigurationOwner(owner string, fence int64) error {
	if strings.TrimSpace(owner) == "" {
		return fmt.Errorf("configuration operation owner is required")
	}
	if fence <= 0 {
		return fmt.Errorf("configuration operation fence must be positive")
	}
	return nil
}

func (s *Store) SetOperationCompensation(ctx context.Context, operationID, phase, key string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE operations SET compensation_phase=?, compensation_idempotency_key=? WHERE id=?`, phase, key, operationID)
	return err
}

func (s *Store) GetOperationCompensation(ctx context.Context, operationID string) (OperationCompensation, error) {
	var state OperationCompensation
	err := s.db.QueryRowContext(ctx, `SELECT compensation_phase,compensation_idempotency_key FROM operations WHERE id=?`, operationID).Scan(&state.Phase, &state.Key)
	if errors.Is(err, sql.ErrNoRows) {
		return state, ErrNotFound
	}
	return state, err
}

// ConfigurationAdmission is the single durable admission boundary for a
// configuration command. The lease, revision, encrypted mutations, operation
// event and exact command payload are committed (or rolled back) together.
type ConfigurationAdmission struct {
	Operation        contracts.Operation
	ProjectID        string
	Owner            string
	ExpectedRevision int64
	Configuration    contracts.ProjectConfiguration
	OperationKind    string
	Mutations        []SecretMutation
	OperationSecrets map[string]secrets.Envelope
	Now              time.Time
	LeaseTTL         time.Duration
}

func (s *Store) AdmitConfiguration(ctx context.Context, input ConfigurationAdmission) (ConfigurationSnapshot, ConfigurationLease, error) {
	if err := validateConfigurationOwner(input.Owner, 1); err != nil {
		return ConfigurationSnapshot{}, ConfigurationLease{}, err
	}
	if input.Operation.ID == "" || input.Operation.ID != input.Owner {
		return ConfigurationSnapshot{}, ConfigurationLease{}, fmt.Errorf("configuration operation owner must match operation id")
	}
	if input.OperationKind == "" {
		input.OperationKind = "UPDATE_CONFIG"
	}
	if input.LeaseTTL <= 0 {
		input.LeaseTTL = 45 * time.Minute
	}
	if input.Now.IsZero() {
		input.Now = time.Now()
	}
	redacted := redactConfiguration(input.Configuration)
	redacted.Revision = input.ExpectedRevision + 1
	payload, err := json.Marshal(redacted)
	if err != nil {
		return ConfigurationSnapshot{}, ConfigurationLease{}, fmt.Errorf("encode configuration: %w", err)
	}
	var snapshot ConfigurationSnapshot
	var lease ConfigurationLease
	err = s.InTx(ctx, func(tx *sql.Tx) error {
		expires := input.Now.Add(input.LeaseTTL)
		// Admission owns the complete conflict check and reservation. Keeping
		// both in this write transaction prevents a racing project from passing
		// while the candidate is still pending (before owned publication).
		resources := configurationResources(input.Configuration)
		for kind, key := range resources {
			var conflictID string
			err := tx.QueryRowContext(ctx, `SELECT project_id FROM configuration_reservations WHERE resource_kind=? AND resource_key=? AND project_id<>? LIMIT 1`, kind, key, input.ProjectID).Scan(&conflictID)
			if errors.Is(err, sql.ErrNoRows) {
				if kind == "domain" {
					err = tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE domain=? AND id<>? LIMIT 1`, key, input.ProjectID).Scan(&conflictID)
				} else {
					err = tx.QueryRowContext(ctx, `SELECT project_id FROM port_allocations WHERE port=? AND project_id<>? LIMIT 1`, key, input.ProjectID).Scan(&conflictID)
				}
			}
			if err == nil {
				return fmt.Errorf("%w: %s", ErrConfigurationConflict, conflictID)
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("check configuration conflicts: %w", err)
			}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO project_configuration_leases(project_id, owner, fence, acquired_at, expires_at) VALUES (?, ?, 1, ?, ?) ON CONFLICT(project_id) DO UPDATE SET owner=excluded.owner, fence=project_configuration_leases.fence+1, acquired_at=excluded.acquired_at, expires_at=excluded.expires_at WHERE project_configuration_leases.expires_at <= excluded.acquired_at`, input.ProjectID, input.Owner, formatTime(input.Now), formatTime(expires))
		if err != nil {
			return fmt.Errorf("admit configuration lease: %w", err)
		}
		count, _ := res.RowsAffected()
		if count != 1 {
			return ErrConfigurationBusy
		}
		if err := tx.QueryRowContext(ctx, `SELECT fence FROM project_configuration_leases WHERE project_id=? AND owner=?`, input.ProjectID, input.Owner).Scan(&lease.Fence); err != nil {
			return err
		}
		lease.Owner = input.Owner
		var current int64
		if err := tx.QueryRowContext(ctx, `SELECT config_revision FROM projects WHERE id=?`, input.ProjectID).Scan(&current); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if current != input.ExpectedRevision {
			return fmt.Errorf("%w: expected %d, current %d", ErrStaleConfiguration, input.ExpectedRevision, current)
		}
		next := input.ExpectedRevision + 1
		if _, err := tx.ExecContext(ctx, `INSERT INTO operations(id, project_id, type, status, progress, created_at) VALUES (?, ?, ?, ?, 0, ?)`, input.Operation.ID, input.ProjectID, input.Operation.Type, input.Operation.Status, formatTime(input.Operation.CreatedAt)); err != nil {
			return fmt.Errorf("create admitted operation: %w", err)
		}
		if err := appendOperationEvent(ctx, tx, input.Operation.ID, "OPERATION_QUEUED", json.RawMessage(`{"status":"QUEUED"}`), input.Operation.CreatedAt); err != nil {
			return err
		}
		for kind, key := range resources {
			if _, err := tx.ExecContext(ctx, `INSERT INTO configuration_reservations(resource_kind,resource_key,project_id,operation_id,revision,created_at) VALUES(?,?,?,?,?,?)`, kind, key, input.ProjectID, input.Operation.ID, input.ExpectedRevision+1, formatTime(input.Now)); err != nil {
				if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "primary key") {
					return fmt.Errorf("%w: pending %s %s", ErrConfigurationConflict, kind, key)
				}
				return fmt.Errorf("reserve configuration %s %s: %w", kind, key, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET config_revision=?, updated_at=? WHERE id=? AND config_revision=?`, next, formatTime(input.Now), input.ProjectID, input.ExpectedRevision); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_configs(project_id,section,revision,config_json,created_at) VALUES(?, 'aggregate', ?, ?, ?)`, input.ProjectID, next, string(payload), formatTime(input.Now)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_secret_versions(project_id,revision,kind,envelope_version,nonce,ciphertext) SELECT project_id, ?, kind, envelope_version, nonce, ciphertext FROM project_secrets WHERE project_id=?`, next, input.ProjectID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_secret_snapshot_markers(project_id,revision,present) VALUES(?,?,CASE WHEN EXISTS(SELECT 1 FROM project_secrets WHERE project_id=?) THEN 1 ELSE 0 END)`, input.ProjectID, next, input.ProjectID); err != nil {
			return err
		}
		for _, mutation := range input.Mutations {
			if mutation.Delete {
				if _, err := tx.ExecContext(ctx, `DELETE FROM project_secrets WHERE project_id=? AND kind=?`, input.ProjectID, mutation.Kind); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO project_secrets(project_id,kind,envelope_version,nonce,ciphertext,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(project_id,kind) DO UPDATE SET envelope_version=excluded.envelope_version,nonce=excluded.nonce,ciphertext=excluded.ciphertext,updated_at=excluded.updated_at`, input.ProjectID, mutation.Kind, mutation.Envelope.Version, mutation.Envelope.Nonce, mutation.Envelope.Ciphertext, formatTime(input.Now)); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operation_configurations(operation_id,project_id,revision,config_json,operation_kind,fence,created_at) VALUES(?,?,?,?,?,?,?)`, input.Operation.ID, input.ProjectID, next, string(payload), input.OperationKind, lease.Fence, formatTime(input.Now)); err != nil {
			return err
		}
		for kind, envelope := range input.OperationSecrets {
			if _, err := tx.ExecContext(ctx, `INSERT INTO operation_secrets(operation_id,kind,envelope_version,nonce,ciphertext) VALUES(?,?,?,?,?)`, input.Operation.ID, kind, envelope.Version, envelope.Nonce, envelope.Ciphertext); err != nil {
				return err
			}
		}
		snapshot = ConfigurationSnapshot{ProjectID: input.ProjectID, Revision: next, LastGoodRevision: input.ExpectedRevision, Configuration: redacted, Fence: lease.Fence}
		return nil
	})
	if err != nil {
		return ConfigurationSnapshot{}, ConfigurationLease{}, err
	}
	return snapshot, lease, nil
}

// configurationResources is the authority for globally unique admission
// resources. Disabled service owners contribute no port reservation.
func configurationResources(cfg contracts.ProjectConfiguration) map[string]string {
	resources := make(map[string]string, 7)
	if cfg.General.Domain != "" {
		resources["domain"] = cfg.General.Domain
	}
	ports := []struct {
		port    int
		enabled bool
	}{
		{cfg.Network.APIPort, true},
		{cfg.Network.StudioPort, cfg.Services.Studio},
		{cfg.Network.DirectDatabasePort, cfg.Services.DirectDB},
		{cfg.Network.PoolerPort, cfg.Services.Supavisor},
		{cfg.Pooler.TransactionPort, cfg.Services.Supavisor},
		{cfg.Pooler.SessionPort, cfg.Services.Supavisor},
	}
	for _, candidate := range ports {
		if candidate.enabled && candidate.port > 0 {
			resources["port:"+strconv.Itoa(candidate.port)] = strconv.Itoa(candidate.port)
		}
	}
	return resources
}

func (s *Store) RenewConfigurationLease(ctx context.Context, projectID, owner string, fence int64, now time.Time, ttl time.Duration) (bool, error) {
	if err := validateConfigurationOwner(owner, fence); err != nil {
		return false, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE project_configuration_leases SET expires_at = ? WHERE project_id = ? AND owner = ? AND fence = ?`, formatTime(now.Add(ttl)), projectID, owner, fence)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (s *Store) ReleaseConfigurationLeaseOwned(ctx context.Context, projectID, owner string, fence int64) error {
	if err := validateConfigurationOwner(owner, fence); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM project_configuration_leases WHERE project_id = ? AND owner = ? AND fence = ?`, projectID, owner, fence)
	return err
}

func (s *Store) OwnsConfigurationLease(ctx context.Context, projectID, owner string, fence int64, now time.Time) (bool, error) {
	if err := validateConfigurationOwner(owner, fence); err != nil {
		return false, err
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_configuration_leases WHERE project_id=? AND owner=? AND fence=? AND expires_at > ?`, projectID, owner, fence, formatTime(now)).Scan(&count)
	return count == 1, err
}

// AcquireConfigurationLeaseForOperation resumes an operation with a fresh
// fencing generation and binds that generation to its durable command payload
// atomically. A resumed worker must never run a payload carrying an old fence.
func (s *Store) AcquireConfigurationLeaseForOperation(ctx context.Context, projectID, owner, operationID string, now time.Time, ttl time.Duration) (ConfigurationLease, bool, error) {
	if err := validateConfigurationOwner(owner, 1); err != nil {
		return ConfigurationLease{}, false, err
	}
	if operationID == "" || operationID != owner {
		return ConfigurationLease{}, false, fmt.Errorf("configuration lease owner must match operation id")
	}
	var lease ConfigurationLease
	err := s.InTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `INSERT INTO project_configuration_leases(project_id,owner,fence,acquired_at,expires_at) VALUES(?,?,1,?,?) ON CONFLICT(project_id) DO UPDATE SET owner=excluded.owner,fence=project_configuration_leases.fence+1,acquired_at=excluded.acquired_at,expires_at=excluded.expires_at WHERE project_configuration_leases.expires_at <= excluded.acquired_at`, projectID, owner, formatTime(now), formatTime(now.Add(ttl)))
		if err != nil {
			return err
		}
		count, _ := res.RowsAffected()
		if count != 1 {
			return ErrConfigurationBusy
		}
		if err := tx.QueryRowContext(ctx, `SELECT fence FROM project_configuration_leases WHERE project_id=? AND owner=?`, projectID, owner).Scan(&lease.Fence); err != nil {
			return err
		}
		updated, err := tx.ExecContext(ctx, `UPDATE operation_configurations SET fence=? WHERE operation_id=? AND project_id=?`, lease.Fence, operationID, projectID)
		if err != nil {
			return err
		}
		if count, err := updated.RowsAffected(); err != nil {
			return err
		} else if count != 1 {
			return ErrNotFound
		}
		lease.Owner = owner
		return nil
	})
	if errors.Is(err, ErrConfigurationBusy) {
		return ConfigurationLease{}, false, nil
	}
	if err != nil {
		return ConfigurationLease{}, false, err
	}
	return lease, true, nil
}

func (s *Store) GetConfiguration(ctx context.Context, projectID string) (ConfigurationSnapshot, error) {
	var snapshot ConfigurationSnapshot
	var raw string
	err := s.db.QueryRowContext(ctx, `
SELECT p.id, p.config_revision, p.last_good_revision, c.config_json
FROM projects AS p
JOIN project_configs AS c ON c.project_id = p.id AND c.section = 'aggregate' AND c.revision = p.last_good_revision
WHERE p.id = ?`, projectID).Scan(&snapshot.ProjectID, &snapshot.Revision, &snapshot.LastGoodRevision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ConfigurationSnapshot{}, ErrNotFound
	}
	if err != nil {
		return ConfigurationSnapshot{}, fmt.Errorf("get configuration: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &snapshot.Configuration); err != nil {
		return ConfigurationSnapshot{}, fmt.Errorf("decode configuration: %w", err)
	}
	snapshot.Configuration = redactConfiguration(snapshot.Configuration)
	// Revision remains the current desired revision for optimistic PATCH
	// checks, while the body is projected from the last-known-good snapshot.
	snapshot.Configuration.Revision = snapshot.Revision
	return snapshot, nil
}

// GetDesiredConfiguration returns the latest aggregate, including revisions
// that are not yet last-known-good. It is used solely as the merge base for a
// subsequent optimistic PATCH.
func (s *Store) GetDesiredConfiguration(ctx context.Context, projectID string) (ConfigurationSnapshot, error) {
	var snapshot ConfigurationSnapshot
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT p.id, p.config_revision, p.last_good_revision, c.config_json FROM projects p JOIN project_configs c ON c.project_id=p.id AND c.section='aggregate' AND c.revision=p.config_revision WHERE p.id=?`, projectID).Scan(&snapshot.ProjectID, &snapshot.Revision, &snapshot.LastGoodRevision, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ConfigurationSnapshot{}, ErrNotFound
	}
	if err != nil {
		return ConfigurationSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(raw), &snapshot.Configuration); err != nil {
		return ConfigurationSnapshot{}, err
	}
	snapshot.Configuration = redactConfiguration(snapshot.Configuration)
	snapshot.Configuration.Revision = snapshot.Revision
	return snapshot, nil
}

// PersistAllocatedConfiguration records server-owned ports without creating a
// user configuration revision. Allocation happens during installation before
// the first desired revision is rendered, so the immutable create revision
// must contain the exact ports that were reserved.
func (s *Store) PersistAllocatedConfiguration(ctx context.Context, projectID string, cfg contracts.ProjectConfiguration, now time.Time) error {
	return s.InTx(ctx, func(tx *sql.Tx) error {
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT config_revision FROM projects WHERE id=?`, projectID).Scan(&revision); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("read configuration revision for allocation: %w", err)
		}
		cfg.Revision = revision
		payload, err := json.Marshal(redactConfiguration(cfg))
		if err != nil {
			return fmt.Errorf("encode allocated configuration: %w", err)
		}
		result, err := tx.ExecContext(ctx, `UPDATE project_configs SET config_json=? WHERE project_id=? AND section='aggregate' AND revision=?`, string(payload), projectID, revision)
		if err != nil {
			return fmt.Errorf("persist allocated configuration: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("read allocated configuration update: %w", err)
		}
		if count == 0 {
			return ErrNotFound
		}
		// Keep the query projections (services and server-owned ports) in sync
		// with the allocated aggregate. This is intentionally in the same
		// transaction as the aggregate update so a retry cannot expose a mixed
		// configuration to the API or the provisioner.
		return updateCanonicalProjectionTx(ctx, tx, projectID, cfg, now)
	})
}

type serviceProjection struct {
	name    string
	enabled bool
}

func projectServiceProjection(services contracts.Services) []serviceProjection {
	return []serviceProjection{
		{"database", services.Database}, {"gateway", services.Gateway}, {"auth", services.Auth},
		{"rest", services.REST}, {"studio", services.Studio}, {"postgresMeta", services.PostgresMeta},
		{"realtime", services.Realtime}, {"storage", services.Storage}, {"imgproxy", services.Imgproxy},
		{"functions", services.Functions}, {"supavisor", services.Supavisor}, {"logs", services.Logs},
		{"vector", services.Vector}, {"directDb", services.DirectDB},
	}
}

// RestoreConfigurationStateOwned is the only production compensation path.
// It uses CAS on desired revision and the operation's lease/fence before
// touching secrets, so a stale worker cannot erase a successor revision.
func (s *Store) RestoreConfigurationStateOwned(ctx context.Context, projectID string, failedRevision int64, owner string, fence int64, now time.Time) error {
	if err := validateConfigurationOwner(owner, fence); err != nil {
		return err
	}
	return s.InTx(ctx, func(tx *sql.Tx) error {
		var current, lastGood int64
		if err := tx.QueryRowContext(ctx, `SELECT config_revision,last_good_revision FROM projects WHERE id=?`, projectID).Scan(&current, &lastGood); err != nil {
			return err
		}
		if current != failedRevision {
			if current == lastGood {
				var phase string
				if err := tx.QueryRowContext(ctx, `SELECT compensation_phase FROM operations WHERE id=?`, owner).Scan(&phase); err == nil && phase == "STATE_RESTORED" {
					return nil
				}
			}
			return fmt.Errorf("%w: expected failed revision %d, current %d", ErrStaleConfiguration, failedRevision, current)
		}
		var leaseOwner string
		var leaseFence int64
		if err := tx.QueryRowContext(ctx, `SELECT owner,fence FROM project_configuration_leases WHERE project_id=?`, projectID).Scan(&leaseOwner, &leaseFence); err != nil {
			return err
		}
		// An expired lease is recoverable by its original fenced owner. The
		// current-revision CAS above proves that no successor has admitted;
		// rejecting solely on expiry would strand candidate rows forever after
		// a process pause. A different owner/fence is still stale.
		if leaseOwner != owner || leaseFence != fence {
			return fmt.Errorf("%w: configuration lease is no longer owned", ErrStaleConfiguration)
		}
		var boundRevision, boundFence int64
		if err := tx.QueryRowContext(ctx, `SELECT revision,fence FROM operation_configurations WHERE operation_id=? AND project_id=?`, owner, projectID).Scan(&boundRevision, &boundFence); err != nil {
			return err
		}
		if boundRevision != failedRevision || boundFence != fence {
			return fmt.Errorf("%w: operation fence changed", ErrStaleConfiguration)
		}
		var raw string
		if err := tx.QueryRowContext(ctx, `SELECT config_json FROM project_configs WHERE project_id=? AND section='aggregate' AND revision=?`, projectID, lastGood).Scan(&raw); err != nil {
			return err
		}
		var cfg contracts.ProjectConfiguration
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_secrets WHERE project_id=?`, projectID); err != nil {
			return err
		}
		var present int
		if err := tx.QueryRowContext(ctx, `SELECT present FROM project_secret_snapshot_markers WHERE project_id=? AND revision=?`, projectID, failedRevision).Scan(&present); err != nil {
			return err
		}
		if present != 0 {
			var rows int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_secret_versions WHERE project_id=? AND revision=?`, projectID, failedRevision).Scan(&rows); err != nil {
				return err
			}
			if rows == 0 {
				return ErrSecretSnapshotUnavailable
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO project_secrets(project_id,kind,envelope_version,nonce,ciphertext,updated_at) SELECT project_id,kind,envelope_version,nonce,ciphertext,? FROM project_secret_versions WHERE project_id=? AND revision=?`, formatTime(time.Now()), projectID, failedRevision); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET config_revision=?, updated_at=? WHERE id=?`, lastGood, formatTime(time.Now()), projectID); err != nil {
			return err
		}
		if err := updateCanonicalProjectionTx(ctx, tx, projectID, cfg, time.Now()); err != nil {
			return err
		}
		// The failed desired revision is not an active audit record: the
		// operation row/event remains the audit trail. Remove its candidate
		// snapshots so the next optimistic update can safely reuse the revision
		// after the CAS above has established that no successor won the project.
		for table := range map[string]struct{}{"project_configs": {}, "project_secret_versions": {}, "project_secret_snapshot_markers": {}} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE project_id=? AND revision=?`, projectID, failedRevision); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM configuration_reservations WHERE project_id=? AND revision=?`, projectID, failedRevision); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_configuration_leases WHERE project_id=? AND owner=? AND fence=?`, projectID, owner, fence); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE operations SET compensation_phase='STATE_RESTORED' WHERE id=?`, owner); err != nil {
			return err
		}
		return nil
	})
}

func (s *Store) GetOperationKind(ctx context.Context, operationID string) (string, error) {
	var kind string
	err := s.db.QueryRowContext(ctx, `SELECT operation_kind FROM operation_configurations WHERE operation_id = ?`, operationID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return kind, err
}

func (s *Store) GetOperationConfiguration(ctx context.Context, operationID string) (ConfigurationSnapshot, error) {
	var snapshot ConfigurationSnapshot
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT project_id, revision, config_json, fence FROM operation_configurations WHERE operation_id = ?`, operationID).Scan(&snapshot.ProjectID, &snapshot.Revision, &raw, &snapshot.Fence)
	if errors.Is(err, sql.ErrNoRows) {
		return ConfigurationSnapshot{}, ErrNotFound
	}
	if err != nil {
		return snapshot, err
	}
	if err := json.Unmarshal([]byte(raw), &snapshot.Configuration); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func (s *Store) PutOperationSecret(ctx context.Context, operationID, kind string, envelope secrets.Envelope) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO operation_secrets(operation_id,kind,envelope_version,nonce,ciphertext) VALUES(?,?,?,?,?)`, operationID, kind, envelope.Version, envelope.Nonce, envelope.Ciphertext)
	return err
}

func (s *Store) GetOperationSecret(ctx context.Context, operationID, kind string) (secrets.Envelope, error) {
	var envelope secrets.Envelope
	err := s.db.QueryRowContext(ctx, `SELECT envelope_version,nonce,ciphertext FROM operation_secrets WHERE operation_id=? AND kind=?`, operationID, kind).Scan(&envelope.Version, &envelope.Nonce, &envelope.Ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return envelope, ErrNotFound
	}
	return envelope, err
}

// GetSecretAtRevision reads the immutable encrypted snapshot captured before a
// desired revision was admitted. It is used by recovery to reconstruct the
// old credential without trusting a possibly already-published successor.
func (s *Store) GetSecretAtRevision(ctx context.Context, projectID string, revision int64, kind string) (secrets.Envelope, error) {
	var envelope secrets.Envelope
	err := s.db.QueryRowContext(ctx, `SELECT envelope_version,nonce,ciphertext FROM project_secret_versions WHERE project_id=? AND revision=? AND kind=?`, projectID, revision, kind).Scan(&envelope.Version, &envelope.Nonce, &envelope.Ciphertext)
	if errors.Is(err, sql.ErrNoRows) {
		return envelope, ErrNotFound
	}
	return envelope, err
}

// MarkConfigurationGoodOwned atomically publishes a candidate revision and
// records its durable commit phase. Recovery can therefore distinguish a
// crash after publication from a runtime candidate that still needs rollback.
func (s *Store) MarkConfigurationGoodOwned(ctx context.Context, projectID string, revision int64, owner string, fence int64, phase string, now time.Time) error {
	if revision <= 0 {
		return ErrStaleConfiguration
	}
	if err := validateConfigurationOwner(owner, fence); err != nil {
		return err
	}
	if strings.TrimSpace(phase) == "" {
		return fmt.Errorf("configuration publication phase is required")
	}
	return s.InTx(ctx, func(tx *sql.Tx) error {
		var current, lastGood int64
		if err := tx.QueryRowContext(ctx, `SELECT config_revision, last_good_revision FROM projects WHERE id = ?`, projectID).Scan(&current, &lastGood); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("read configuration revision: %w", err)
		}
		if revision != current {
			return fmt.Errorf("%w: expected current revision %d, got %d", ErrStaleConfiguration, current, revision)
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM project_configs WHERE project_id = ? AND section = 'aggregate' AND revision = ?`, projectID, revision).Scan(&exists); err != nil {
			return fmt.Errorf("verify configuration snapshot: %w", err)
		}
		if exists != 1 {
			return fmt.Errorf("%w: aggregate revision %d does not exist", ErrStaleConfiguration, revision)
		}
		if revision < lastGood {
			return fmt.Errorf("%w: last good revision is %d", ErrStaleConfiguration, lastGood)
		}
		var leaseOwner string
		var leaseFence int64
		if err := tx.QueryRowContext(ctx, `SELECT owner,fence FROM project_configuration_leases WHERE project_id=?`, projectID).Scan(&leaseOwner, &leaseFence); err != nil {
			return err
		}
		if leaseOwner != owner || leaseFence != fence {
			return fmt.Errorf("%w: configuration lease is no longer owned", ErrStaleConfiguration)
		}
		var boundRevision, boundFence int64
		if err := tx.QueryRowContext(ctx, `SELECT revision,fence FROM operation_configurations WHERE operation_id=? AND project_id=?`, owner, projectID).Scan(&boundRevision, &boundFence); err != nil {
			return err
		}
		if boundRevision != revision || boundFence != fence {
			return fmt.Errorf("%w: operation fence changed", ErrStaleConfiguration)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET last_good_revision = ? WHERE id = ?`, revision, projectID); err != nil {
			return fmt.Errorf("mark configuration good: %w", err)
		}
		var raw string
		if err := tx.QueryRowContext(ctx, `SELECT config_json FROM project_configs WHERE project_id=? AND section='aggregate' AND revision=?`, projectID, revision).Scan(&raw); err != nil {
			return err
		}
		var cfg contracts.ProjectConfiguration
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return err
		}
		if err := updateCanonicalProjectionTx(ctx, tx, projectID, cfg, time.Now()); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM configuration_reservations WHERE project_id=? AND revision=?`, projectID, revision); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE operations SET compensation_phase=? WHERE id=?`, phase, owner); err != nil {
			return err
		}
		return nil
	})
}

func updateCanonicalProjectionTx(ctx context.Context, tx *sql.Tx, projectID string, cfg contracts.ProjectConfiguration, now time.Time) error {
	servicesJSON, err := json.Marshal(cfg.Services)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE projects SET domain=?, site_url=?, supabase_version=?, services_json=?, updated_at=? WHERE id=?`, cfg.General.Domain, cfg.General.SiteURL, cfg.General.SupabaseVersion, string(servicesJSON), formatTime(now), projectID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_services WHERE project_id=?`, projectID); err != nil {
		return err
	}
	for _, service := range projectServiceProjection(cfg.Services) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_services(project_id,service,enabled,status) VALUES(?,?,?,'UNKNOWN')`, projectID, service.name, service.enabled); err != nil {
			return err
		}
	}
	for _, port := range []struct {
		kind    string
		port    int
		enabled bool
	}{{"API", cfg.Network.APIPort, true}, {"STUDIO", cfg.Network.StudioPort, cfg.Services.Studio}, {"DATABASE", cfg.Network.DirectDatabasePort, cfg.Services.DirectDB}, {"POOLER", cfg.Network.PoolerPort, cfg.Services.Supavisor}, {"POOLER_TRANSACTION", cfg.Pooler.TransactionPort, cfg.Services.Supavisor}, {"POOLER_SESSION", cfg.Pooler.SessionPort, cfg.Services.Supavisor}} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM port_allocations WHERE project_id=? AND kind=?`, projectID, port.kind); err != nil {
			return err
		}
		if !port.enabled || port.port == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO port_allocations(port,project_id,kind,created_at) VALUES(?,?,?,?)`, port.port, projectID, port.kind, formatTime(now)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PublishConfigurationSecret(ctx context.Context, projectID string, revision int64, kind string, envelope secrets.Envelope, now time.Time) error {
	return s.InTx(ctx, func(tx *sql.Tx) error {
		var current int64
		if err := tx.QueryRowContext(ctx, `SELECT config_revision FROM projects WHERE id=?`, projectID).Scan(&current); err != nil {
			return err
		}
		if current != revision {
			return fmt.Errorf("%w: revision changed", ErrStaleConfiguration)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_secrets(project_id,kind,envelope_version,nonce,ciphertext,updated_at) VALUES(?,?,?,?,?,?) ON CONFLICT(project_id,kind) DO UPDATE SET envelope_version=excluded.envelope_version,nonce=excluded.nonce,ciphertext=excluded.ciphertext,updated_at=excluded.updated_at`, projectID, kind, envelope.Version, envelope.Nonce, envelope.Ciphertext, formatTime(now)); err != nil {
			return err
		}
		// Secret publication is only one side of a runtime rotation. The
		// revision becomes last-good after the Provisioner confirms its durable
		// journal, so compensation can still restore the previous snapshot.
		return nil
	})
}

func redactConfiguration(cfg contracts.ProjectConfiguration) contracts.ProjectConfiguration {
	// Clone every reference-bearing field before removing secrets. This keeps
	// persistence and read-boundary redaction from mutating caller-owned maps or
	// slices.
	if cfg.Auth.RedirectURLs != nil {
		cfg.Auth.RedirectURLs = append([]string(nil), cfg.Auth.RedirectURLs...)
	}
	if cfg.Auth.Phone.Fields != nil {
		cfg.Auth.Phone.Fields = cloneStringMap(cfg.Auth.Phone.Fields)
	}
	if cfg.Auth.OAuth != nil {
		oauthCopy := make(map[string]contracts.OAuthProviderConfig, len(cfg.Auth.OAuth))
		for provider, oauth := range cfg.Auth.OAuth {
			if oauth.Fields != nil {
				oauth.Fields = cloneStringMap(oauth.Fields)
			}
			oauthCopy[provider] = oauth
		}
		cfg.Auth.OAuth = oauthCopy
	}
	if cfg.Functions.Variables != nil {
		cfg.Functions.Variables = append([]contracts.FunctionVariable(nil), cfg.Functions.Variables...)
	}
	if cfg.Database.Extensions != nil {
		cfg.Database.Extensions = append([]string(nil), cfg.Database.Extensions...)
	}
	cfg.Auth.SMTP.Password = contracts.SecretInput{}
	cfg.Auth.Phone.Secret = contracts.SecretInput{}
	for provider, oauth := range cfg.Auth.OAuth {
		oauth.Secret = contracts.SecretInput{}
		cfg.Auth.OAuth[provider] = oauth
	}
	cfg.Storage.SecretAccessKey = contracts.SecretInput{}
	for index := range cfg.Functions.Variables {
		cfg.Functions.Variables[index].Value = contracts.SecretInput{}
	}
	return cfg
}

func cloneStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
