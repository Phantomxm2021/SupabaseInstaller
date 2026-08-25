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

type ConfigurationSnapshot struct {
	ProjectID        string
	Revision         int64
	LastGoodRevision int64
	Configuration    contracts.ProjectConfiguration
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
JOIN project_configs AS c ON c.project_id = p.id AND c.section = 'aggregate' AND c.revision = p.config_revision
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
	var result ConfigurationSnapshot
	err = s.InTx(ctx, func(tx *sql.Tx) error {
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

func (s *Store) MarkConfigurationGood(ctx context.Context, projectID string, revision int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE projects SET last_good_revision = ? WHERE id = ? AND config_revision >= ?`, revision, projectID, revision)
	if err != nil {
		return fmt.Errorf("mark configuration good: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		var exists string
		if err := s.db.QueryRowContext(ctx, `SELECT id FROM projects WHERE id = ?`, projectID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return ErrStaleConfiguration
	}
	return nil
}

func redactConfiguration(cfg contracts.ProjectConfiguration) contracts.ProjectConfiguration {
	cfg.Revision = cfg.Revision
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
