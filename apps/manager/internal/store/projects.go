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

func (s *Store) CreateProject(ctx context.Context, project contracts.Project) error {
	servicesJSON, err := json.Marshal(project.Services)
	if err != nil {
		return fmt.Errorf("encode services: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
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
	return nil
}

func (s *Store) GetProject(ctx context.Context, id string) (contracts.Project, error) {
	var project contracts.Project
	var servicesJSON, createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, slug, domain, site_url, status, health, supabase_version,
       preset, services_json, created_at, updated_at
FROM projects WHERE id = ?`, id).Scan(
		&project.ID, &project.Name, &project.Slug, &project.Domain, &project.SiteURL,
		&project.Status, &project.Health, &project.SupabaseVersion, &project.Preset,
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

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
