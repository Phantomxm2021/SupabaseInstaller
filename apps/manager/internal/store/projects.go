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

var ErrNotFound = errors.New("not found")

func (s *Store) CreateProject(ctx context.Context, project contracts.Project, configurations ...contracts.ProjectConfiguration) error {
	configuration := firstConfiguration(project, configurations...)
	if configurationHasReplacement(configuration) {
		return errors.New("secret cipher is required for replacement values")
	}
	return s.CreateProjectWithSecrets(ctx, project, configuration, nil)
}

func configurationHasReplacement(cfg contracts.ProjectConfiguration) bool {
	if cfg.Auth.SMTP.Password.Action == "replace" || cfg.Auth.Phone.Secret.Action == "replace" || cfg.Storage.SecretAccessKey.Action == "replace" {
		return true
	}
	for _, oauth := range cfg.Auth.OAuth {
		if oauth.Secret.Action == "replace" {
			return true
		}
	}
	for _, variable := range cfg.Functions.Variables {
		if variable.Value.Action == "replace" {
			return true
		}
	}
	return false
}

func firstConfiguration(project contracts.Project, configurations ...contracts.ProjectConfiguration) contracts.ProjectConfiguration {
	configuration := contracts.ProjectConfiguration{Revision: 1, General: contracts.GeneralConfig{Domain: project.Domain, SiteURL: project.SiteURL, SupabaseVersion: project.SupabaseVersion}, Services: project.Services}
	if len(configurations) > 0 {
		configuration = configurations[0]
	}
	configuration.Revision = 1
	return configuration
}

func (s *Store) CreateProjectWithSecrets(ctx context.Context, project contracts.Project, configuration contracts.ProjectConfiguration, mutations []SecretMutation) error {
	servicesJSON, err := json.Marshal(project.Services)
	if err != nil {
		return fmt.Errorf("encode services: %w", err)
	}
	configuration.Revision = 1
	configurationJSON, err := json.Marshal(redactConfiguration(configuration))
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	err = s.InTx(ctx, func(tx *sql.Tx) error {
		_, err = tx.ExecContext(ctx, `
INSERT INTO projects (
  id, name, slug, domain, site_url, status, health, supabase_version,
  preset, services_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			project.ID, project.Name, project.Slug, project.Domain, project.SiteURL,
			project.Status, project.Health, project.SupabaseVersion, project.Preset,
			string(servicesJSON), formatTime(project.CreatedAt), formatTime(project.UpdatedAt),
		)
		if err != nil {
			return fmt.Errorf("create project: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO project_configs(project_id, section, revision, config_json, created_at) VALUES (?, 'aggregate', 1, ?, ?)`, project.ID, string(configurationJSON), formatTime(project.CreatedAt)); err != nil {
			return fmt.Errorf("create initial configuration: %w", err)
		}
		for _, service := range projectServiceProjection(configuration.Services) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO project_services(project_id, service, enabled, status) VALUES (?, ?, ?, 'UNKNOWN')`, project.ID, service.name, service.enabled); err != nil {
				return fmt.Errorf("create service projection %s: %w", service.name, err)
			}
		}
		for _, mutation := range mutations {
			if mutation.Delete {
				continue
			}
			if mutation.Kind == "" {
				return errors.New("encrypted secret kind is required")
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO project_secrets(project_id, kind, envelope_version, nonce, ciphertext, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, project.ID, mutation.Kind, mutation.Envelope.Version, mutation.Envelope.Nonce, mutation.Envelope.Ciphertext, formatTime(project.CreatedAt)); err != nil {
				return fmt.Errorf("create encrypted secret %s: %w", mutation.Kind, err)
			}
		}
		// Revision 1 is an immutable pre-mutation snapshot too. Without these
		// rows a rollback of the first configuration operation would restore an
		// intentionally empty set even when creation supplied encrypted secrets.
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO project_secret_versions(project_id, revision, kind, envelope_version, nonce, ciphertext) SELECT project_id, 1, kind, envelope_version, nonce, ciphertext FROM project_secrets WHERE project_id = ?`, project.ID); err != nil {
			return fmt.Errorf("snapshot initial encrypted secrets: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_secret_snapshot_markers(project_id, revision, present) VALUES (?, 1, CASE WHEN EXISTS (SELECT 1 FROM project_secrets WHERE project_id = ?) THEN 1 ELSE 0 END)`, project.ID, project.ID); err != nil {
			return fmt.Errorf("create initial secret snapshot marker: %w", err)
		}
		return nil
	})
	return err
}

func (s *Store) GetProject(ctx context.Context, id string) (contracts.Project, error) {
	var project contracts.Project
	var servicesJSON, createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
	SELECT id, name, slug, domain, site_url, status, health, supabase_version,
	       config_revision,
       preset, services_json, created_at, updated_at
FROM projects WHERE id = ?`, id).Scan(
		&project.ID, &project.Name, &project.Slug, &project.Domain, &project.SiteURL,
		&project.Status, &project.Health, &project.SupabaseVersion, &project.ConfigurationRevision, &project.Preset,
		&servicesJSON, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.Project{}, ErrNotFound
	}
	if err != nil {
		return contracts.Project{}, fmt.Errorf("get project: %w", err)
	}
	if err := json.Unmarshal([]byte(servicesJSON), &project.Services); err != nil {
		return contracts.Project{}, fmt.Errorf("decode services: %w", err)
	}
	project.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return contracts.Project{}, fmt.Errorf("parse project created time: %w", err)
	}
	project.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return contracts.Project{}, fmt.Errorf("parse project updated time: %w", err)
	}
	return project, nil
}

func (s *Store) ListProjects(ctx context.Context) ([]contracts.Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM projects ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list project IDs: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan project ID: %w", err)
		}
		ids = append(ids, id)
	}
	projects := make([]contracts.Project, 0, len(ids))
	for _, id := range ids {
		project, err := s.GetProject(ctx, id)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (s *Store) UpdateProjectStatus(ctx context.Context, id string, status contracts.ProjectStatus, health contracts.HealthStatus) error {
	result, err := s.db.ExecContext(ctx, `UPDATE projects SET status = ?, health = ?, updated_at = ? WHERE id = ?`, status, health, formatTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("update project status: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read update result: %w", err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project metadata: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) PutSecret(ctx context.Context, projectID, kind string, envelope secrets.Envelope) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO project_secrets(project_id, kind, envelope_version, nonce, ciphertext, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(project_id, kind) DO UPDATE SET
  envelope_version = excluded.envelope_version,
  nonce = excluded.nonce,
  ciphertext = excluded.ciphertext,
  updated_at = excluded.updated_at`,
		projectID, kind, envelope.Version, envelope.Nonce, envelope.Ciphertext, formatTime(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("store encrypted secret: %w", err)
	}
	return nil
}

func (s *Store) GetSecret(ctx context.Context, projectID, kind string) (secrets.Envelope, error) {
	var envelope secrets.Envelope
	err := s.db.QueryRowContext(ctx, `
SELECT envelope_version, nonce, ciphertext
FROM project_secrets WHERE project_id = ? AND kind = ?`, projectID, kind).Scan(
		&envelope.Version, &envelope.Nonce, &envelope.Ciphertext,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return secrets.Envelope{}, ErrNotFound
	}
	if err != nil {
		return secrets.Envelope{}, fmt.Errorf("get encrypted secret: %w", err)
	}
	return envelope, nil
}

// ListSecretKinds returns only identifiers. Callers can explicitly decrypt the
// small set needed for a renderer without ever exposing ciphertext or values
// through normal configuration projections.
func (s *Store) ListSecretKinds(ctx context.Context, projectID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT kind FROM project_secrets WHERE project_id = ? ORDER BY kind`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list encrypted secret kinds: %w", err)
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			return nil, fmt.Errorf("scan encrypted secret kind: %w", err)
		}
		kinds = append(kinds, kind)
	}
	return kinds, rows.Err()
}

func (s *Store) DeleteSecret(ctx context.Context, projectID, kind string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM project_secrets WHERE project_id = ? AND kind = ?`, projectID, kind)
	if err != nil {
		return fmt.Errorf("delete encrypted secret: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return ErrNotFound
	}
	return nil
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
