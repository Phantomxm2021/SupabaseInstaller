package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type AdminRecord struct {
	ID                 string
	Username           string
	PasswordHash       string
	MustChangePassword bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type RecoveryCodeRecord struct {
	ID       string
	CodeHash string
}

type SessionRecord struct {
	TokenHash  [sha256.Size]byte
	AdminID    string
	CSRFHash   [sha256.Size]byte
	LastSeenAt time.Time
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

func (s *Store) AdminCount(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count admins: %w", err)
	}
	return count, nil
}

func (s *Store) CreateFirstAdmin(ctx context.Context, admin AdminRecord, codes []RecoveryCodeRecord) error {
	return s.InTx(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&count); err != nil {
			return fmt.Errorf("count admins: %w", err)
		}
		if count != 0 {
			return ErrConflict
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO admins(id, username, password_hash, must_change_password, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`, admin.ID, admin.Username, admin.PasswordHash, admin.MustChangePassword, formatTime(admin.CreatedAt), formatTime(admin.UpdatedAt))
		if err != nil {
			return fmt.Errorf("create first admin: %w", err)
		}
		for _, code := range codes {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO recovery_codes(id, admin_id, code_hash, created_at) VALUES (?, ?, ?, ?)`, code.ID, admin.ID, code.CodeHash, formatTime(admin.CreatedAt)); err != nil {
				return fmt.Errorf("create recovery code: %w", err)
			}
		}
		return nil
	})
}

func (s *Store) FindAdminByUsername(ctx context.Context, username string) (AdminRecord, error) {
	var admin AdminRecord
	var mustChange int
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
SELECT id, username, password_hash, must_change_password, created_at, updated_at
FROM admins WHERE username = ?`, username).Scan(
		&admin.ID, &admin.Username, &admin.PasswordHash, &mustChange, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminRecord{}, ErrNotFound
	}
	if err != nil {
		return AdminRecord{}, fmt.Errorf("find admin: %w", err)
	}
	admin.MustChangePassword = mustChange == 1
	admin.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	admin.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return admin, nil
}

func (s *Store) GetAdmin(ctx context.Context, id string) (AdminRecord, error) {
	var username string
	err := s.db.QueryRowContext(ctx, `SELECT username FROM admins WHERE id = ?`, id).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminRecord{}, ErrNotFound
	}
	if err != nil {
		return AdminRecord{}, fmt.Errorf("get admin username: %w", err)
	}
	return s.FindAdminByUsername(ctx, username)
}

func (s *Store) UpdateAdminPassword(ctx context.Context, id, passwordHash string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE admins SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ?`, passwordHash, formatTime(now), id)
	if err != nil {
		return fmt.Errorf("update admin password: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, session SessionRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO sessions(id_hash, admin_id, csrf_hash, last_seen_at, expires_at, created_at)
VALUES (?, ?, ?, ?, ?, ?)`, session.TokenHash[:], session.AdminID, session.CSRFHash[:], formatTime(session.LastSeenAt), formatTime(session.ExpiresAt), formatTime(session.CreatedAt))
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *Store) FindSession(ctx context.Context, tokenHash [sha256.Size]byte) (SessionRecord, error) {
	var record SessionRecord
	var storedTokenHash, csrfHash []byte
	var lastSeenAt, expiresAt, createdAt string
	err := s.db.QueryRowContext(ctx, `
SELECT id_hash, admin_id, csrf_hash, last_seen_at, expires_at, created_at
FROM sessions WHERE id_hash = ?`, tokenHash[:]).Scan(&storedTokenHash, &record.AdminID, &csrfHash, &lastSeenAt, &expiresAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionRecord{}, ErrNotFound
	}
	if err != nil {
		return SessionRecord{}, fmt.Errorf("find session: %w", err)
	}
	copy(record.TokenHash[:], storedTokenHash)
	copy(record.CSRFHash[:], csrfHash)
	record.LastSeenAt, _ = time.Parse(time.RFC3339Nano, lastSeenAt)
	record.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
	record.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return record, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash [sha256.Size]byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, tokenHash[:])
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) UpdateSessionCSRF(ctx context.Context, tokenHash, csrfHash [sha256.Size]byte) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET csrf_hash = ? WHERE id_hash = ?`, csrfHash[:], tokenHash[:])
	if err != nil {
		return fmt.Errorf("update session csrf: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

var ErrConflict = errors.New("conflict")
