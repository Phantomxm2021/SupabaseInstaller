package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
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

// PortInUse includes both last-good allocations and pending configuration
// candidates. Callers use it only while choosing a candidate; admission is
// still the authoritative transaction that reserves the selected value.
func (s *Store) PortInUse(ctx context.Context, port int) (bool, error) {
	var used int
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM port_allocations WHERE port=?
		UNION ALL
		SELECT 1 FROM configuration_reservations WHERE resource_kind=? AND resource_key=?
	)`, port, "port:"+strconv.Itoa(port), strconv.Itoa(port)).Scan(&used)
	if err != nil {
		return false, fmt.Errorf("check port candidate: %w", err)
	}
	return used == 1, nil
}

// TryReservePorts atomically claims a set of ports. A conflict rolls back all
// inserts so a multi-service install never leaves a partially allocated
// aggregate behind.
func (s *Store) TryReservePorts(ctx context.Context, projectID string, reservations map[string]int, now time.Time) (bool, error) {
	conflict := errors.New("port reservation conflict")
	err := s.InTx(ctx, func(tx *sql.Tx) error {
		for kind, port := range reservations {
			// A configuration candidate owns a port before it is promoted into
			// port_allocations. Check that pending global reservation in this
			// transaction as well, otherwise an install could steal an update's
			// candidate between CandidateMany and MarkConfigurationGood.
			var pendingProject string
			err := tx.QueryRowContext(ctx, `SELECT project_id FROM configuration_reservations WHERE resource_kind=? AND resource_key=? LIMIT 1`, "port:"+strconv.Itoa(port), strconv.Itoa(port)).Scan(&pendingProject)
			if err == nil {
				return conflict
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("check pending %s port: %w", kind, err)
			}
			result, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO port_allocations(port, project_id, kind, created_at) VALUES (?, ?, ?, ?)`, port, projectID, kind, formatTime(now))
			if err != nil {
				return fmt.Errorf("reserve %s port: %w", kind, err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if count != 1 {
				return conflict
			}
		}
		return nil
	})
	if errors.Is(err, conflict) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) ReleaseProjectPorts(ctx context.Context, projectID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM port_allocations WHERE project_id = ?`, projectID)
	if err != nil {
		return fmt.Errorf("release server ports: %w", err)
	}
	return nil
}

// ReleasePort removes one server-owned reservation when a capability is
// disabled. Keeping this operation narrow lets installs preserve API/Studio
// allocations while returning optional ports to the global pool.
func (s *Store) ReleasePort(ctx context.Context, projectID, kind string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM port_allocations WHERE project_id = ? AND kind = ?`, projectID, kind); err != nil {
		return fmt.Errorf("release %s port: %w", kind, err)
	}
	return nil
}
