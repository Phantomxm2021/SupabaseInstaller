package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/internal/contracts"
)

var ErrStaleConfiguration = errors.New("stale project configuration revision")
var ErrConfigurationConflict = errors.New("project configuration conflicts with another project")

type ConfigurationSnapshot struct {
	ProjectID        string                         `json:"projectId"`
	Revision         int64                          `json:"revision"`
	LastGoodRevision int64                          `json:"lastGoodRevision"`
	Configuration    contracts.ProjectConfiguration `json:"configuration"`
}

// SecretMutation is an already-encrypted change applied in the same transaction
// as its configuration revision.
type SecretMutation struct {
	Kind     string
	Envelope secrets.Envelope
	Delete   bool
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

func (s *Store) SaveConfiguration(ctx context.Context, projectID string, expected int64, cfg contracts.ProjectConfiguration, now time.Time) (ConfigurationSnapshot, error) {
	return s.saveConfiguration(ctx, projectID, expected, cfg, now, nil)
}

func (s *Store) SaveConfigurationWithSecrets(ctx context.Context, projectID string, expected int64, cfg contracts.ProjectConfiguration, now time.Time, mutations []SecretMutation) (ConfigurationSnapshot, error) {
	return s.saveConfiguration(ctx, projectID, expected, cfg, now, mutations)
}

func (s *Store) saveConfiguration(ctx context.Context, projectID string, expected int64, cfg contracts.ProjectConfiguration, now time.Time, mutations []SecretMutation) (ConfigurationSnapshot, error) {
	redacted := redactConfiguration(cfg)
	next := expected + 1
	redacted.Revision = next
	payload, err := json.Marshal(redacted)
	if err != nil {
		return ConfigurationSnapshot{}, fmt.Errorf("encode configuration: %w", err)
	}
	servicesJSON, err := json.Marshal(cfg.Services)
	if err != nil {
		return ConfigurationSnapshot{}, fmt.Errorf("encode services: %w", err)
	}
	var result ConfigurationSnapshot
	err = s.InTx(ctx, func(tx *sql.Tx) error {
		var conflictID string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE (domain = ? OR (? > 0 AND id IN (SELECT project_id FROM port_allocations WHERE port = ?))) AND id <> ? LIMIT 1`, cfg.General.Domain, cfg.Network.APIPort, cfg.Network.APIPort, projectID).Scan(&conflictID); err == nil {
			return fmt.Errorf("%w: %s", ErrConfigurationConflict, conflictID)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check configuration conflicts: %w", err)
		}
		var lastGood int64
		if err := tx.QueryRowContext(ctx, `SELECT last_good_revision FROM projects WHERE id = ?`, projectID).Scan(&lastGood); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("read last good configuration revision: %w", err)
		}
		updated, err := tx.ExecContext(ctx, `UPDATE projects SET config_revision = ?, updated_at = ? WHERE id = ? AND config_revision = ?`, next, formatTime(now), projectID, expected)
		if err != nil {
			return fmt.Errorf("advance configuration revision: %w", err)
		}
		count, err := updated.RowsAffected()
		if err != nil {
			return fmt.Errorf("read configuration revision update: %w", err)
		}
		if count == 0 {
			var current int64
			err := tx.QueryRowContext(ctx, `SELECT config_revision FROM projects WHERE id = ?`, projectID).Scan(&current)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return fmt.Errorf("read current configuration revision: %w", err)
			}
			return fmt.Errorf("%w: expected %d, current %d", ErrStaleConfiguration, expected, current)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_configs(project_id, section, revision, config_json, created_at) VALUES (?, 'aggregate', ?, ?, ?)`, projectID, next, string(payload), formatTime(now)); err != nil {
			return fmt.Errorf("insert configuration revision: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_secret_versions(project_id, revision, kind, envelope_version, nonce, ciphertext) SELECT project_id, ?, kind, envelope_version, nonce, ciphertext FROM project_secrets WHERE project_id = ?`, next, projectID); err != nil {
			return fmt.Errorf("snapshot encrypted secrets: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET domain = ?, site_url = ?, supabase_version = ?, services_json = ?, updated_at = ? WHERE id = ?`, cfg.General.Domain, cfg.General.SiteURL, cfg.General.SupabaseVersion, string(servicesJSON), formatTime(now), projectID); err != nil {
			return fmt.Errorf("update project projection: %w", err)
		}
		for _, mutation := range mutations {
			if mutation.Delete {
				if _, err := tx.ExecContext(ctx, `DELETE FROM project_secrets WHERE project_id = ? AND kind = ?`, projectID, mutation.Kind); err != nil {
					return fmt.Errorf("delete encrypted secret %s: %w", mutation.Kind, err)
				}
				continue
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO project_secrets(project_id, kind, envelope_version, nonce, ciphertext, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, kind) DO UPDATE SET envelope_version = excluded.envelope_version, nonce = excluded.nonce, ciphertext = excluded.ciphertext, updated_at = excluded.updated_at`,
				projectID, mutation.Kind, mutation.Envelope.Version, mutation.Envelope.Nonce, mutation.Envelope.Ciphertext, formatTime(now)); err != nil {
				return fmt.Errorf("store encrypted secret %s: %w", mutation.Kind, err)
			}
		}
		result = ConfigurationSnapshot{ProjectID: projectID, Revision: next, LastGoodRevision: lastGood, Configuration: redacted}
		return nil
	})
	if err != nil {
		return ConfigurationSnapshot{}, err
	}
	return result, nil
}

// RestoreSecretsRevision restores the encrypted secret set that existed just
// before the specified configuration revision was attempted.
func (s *Store) RestoreSecretsRevision(ctx context.Context, projectID string, revision int64) error {
	return s.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_secrets WHERE project_id = ?`, projectID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO project_secrets(project_id, kind, envelope_version, nonce, ciphertext, updated_at) SELECT project_id, kind, envelope_version, nonce, ciphertext, ? FROM project_secret_versions WHERE project_id = ? AND revision = ?`, formatTime(time.Now()), projectID, revision)
		return err
	})
}

func (s *Store) BindOperationConfiguration(ctx context.Context, operationID, projectID string, snapshot ConfigurationSnapshot, now time.Time) error {
	payload, err := json.Marshal(snapshot.Configuration)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO operation_configurations(operation_id, project_id, revision, config_json, created_at) VALUES (?, ?, ?, ?, ?)`, operationID, projectID, snapshot.Revision, string(payload), formatTime(now))
	return err
}

func (s *Store) GetOperationConfiguration(ctx context.Context, operationID string) (ConfigurationSnapshot, error) {
	var snapshot ConfigurationSnapshot
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT project_id, revision, config_json FROM operation_configurations WHERE operation_id = ?`, operationID).Scan(&snapshot.ProjectID, &snapshot.Revision, &raw)
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

func (s *Store) MarkConfigurationGood(ctx context.Context, projectID string, revision int64) error {
	if revision <= 0 {
		return ErrStaleConfiguration
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
		if _, err := tx.ExecContext(ctx, `UPDATE projects SET last_good_revision = ? WHERE id = ?`, revision, projectID); err != nil {
			return fmt.Errorf("mark configuration good: %w", err)
		}
		return nil
	})
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
		_, err := tx.ExecContext(ctx, `UPDATE projects SET last_good_revision=?, updated_at=? WHERE id=?`, revision, formatTime(now), projectID)
		return err
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
