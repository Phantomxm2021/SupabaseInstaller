package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

func (s *Store) ReservedPort(ctx context.Context, projectID, kind string) (int, error) {
	var port int
	err := s.db.QueryRowContext(ctx, `SELECT port FROM port_allocations WHERE project_id = ? AND kind = ?`, projectID, kind).Scan(&port)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("read port reservation: %w", err)
	}
	return port, nil
}

func (s *Store) TryReservePort(ctx context.Context, projectID, kind string, port int, now time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO port_allocations(port, project_id, kind, created_at)
VALUES (?, ?, ?, ?)`, port, projectID, kind, formatTime(now))
	if err != nil {
		return false, fmt.Errorf("reserve port: %w", err)
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (s *Store) ReleaseProjectPorts(ctx context.Context, projectID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM port_allocations WHERE project_id = ?`, projectID)
	if err != nil {
		return fmt.Errorf("release project ports: %w", err)
	}
	return nil
}
