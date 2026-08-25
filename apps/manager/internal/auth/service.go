package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"supabase-manager/apps/manager/internal/store"
)

var (
	ErrAlreadyBootstrapped = errors.New("manager is already bootstrapped")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrUnauthenticated     = errors.New("unauthenticated")
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{2,63}$`)

type BootstrapResult struct {
	AdminID       string
	RecoveryCodes []string
}

type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
}

type Identity struct {
	AdminID            string
	Username           string
	MustChangePassword bool
}

type Service struct {
	store  *store.Store
	hasher PasswordHasher
	random io.Reader
	now    func() time.Time
}

func NewService(store *store.Store, hasher PasswordHasher, random io.Reader, now func() time.Time) *Service {
	return &Service{store: store, hasher: hasher, random: random, now: now}
}

func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	count, err := s.store.AdminCount(ctx)
	return count == 0, err
}

func (s *Service) Bootstrap(ctx context.Context, username, password string) (BootstrapResult, error) {
	if !usernamePattern.MatchString(username) {
		return BootstrapResult{}, fmt.Errorf("username must contain 3 to 64 safe characters")
	}
	if len(password) < 12 {
		return BootstrapResult{}, fmt.Errorf("password must contain at least 12 characters")
	}
	passwordHash, err := s.hasher.Hash(password)
	if err != nil {
		return BootstrapResult{}, err
	}
	now := s.now()
	adminID, err := s.randomID(16)
	if err != nil {
		return BootstrapResult{}, err
	}
	codes := make([]string, 10)
	records := make([]store.RecoveryCodeRecord, 10)
	for index := range codes {
		code, err := s.recoveryCode()
		if err != nil {
			return BootstrapResult{}, err
		}
		codeHash, err := s.hasher.Hash(code)
		if err != nil {
			return BootstrapResult{}, err
		}
		codeID, err := s.randomID(12)
		if err != nil {
			return BootstrapResult{}, err
		}
		codes[index] = code
		records[index] = store.RecoveryCodeRecord{ID: codeID, CodeHash: codeHash}
	}
	err = s.store.CreateFirstAdmin(ctx, store.AdminRecord{
		ID: adminID, Username: username, PasswordHash: passwordHash, CreatedAt: now, UpdatedAt: now,
	}, records)
	if errors.Is(err, store.ErrConflict) {
		return BootstrapResult{}, ErrAlreadyBootstrapped
	}
	if err != nil {
		return BootstrapResult{}, err
	}
	return BootstrapResult{AdminID: adminID, RecoveryCodes: codes}, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (Session, error) {
	admin, err := s.store.FindAdminByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, err
	}
	valid, err := s.hasher.Verify(password, admin.PasswordHash)
	if err != nil || !valid {
		return Session{}, ErrInvalidCredentials
	}
	token, err := s.randomToken(32)
	if err != nil {
		return Session{}, err
	}
	csrf, err := s.randomToken(32)
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	expiresAt := now.Add(7 * 24 * time.Hour)
	record := store.SessionRecord{
		TokenHash:  sha256.Sum256([]byte(token)),
		AdminID:    admin.ID,
		CSRFHash:   sha256.Sum256([]byte(csrf)),
		LastSeenAt: now,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
	}
	if err := s.store.CreateSession(ctx, record); err != nil {
		return Session{}, err
	}
	return Session{Token: token, CSRFToken: csrf, ExpiresAt: expiresAt}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (Identity, error) {
	if token == "" {
		return Identity{}, ErrUnauthenticated
	}
	record, err := s.store.FindSession(ctx, sha256.Sum256([]byte(token)))
	if errors.Is(err, store.ErrNotFound) {
		return Identity{}, ErrUnauthenticated
	}
	if err != nil {
		return Identity{}, err
	}
	if !s.now().Before(record.ExpiresAt) || s.now().Sub(record.LastSeenAt) > 12*time.Hour {
		_ = s.store.DeleteSession(ctx, record.TokenHash)
		return Identity{}, ErrUnauthenticated
	}
	admin, err := s.store.GetAdmin(ctx, record.AdminID)
	if err != nil {
		return Identity{}, ErrUnauthenticated
	}
	return Identity{AdminID: admin.ID, Username: admin.Username, MustChangePassword: admin.MustChangePassword}, nil
}

func (s *Service) ValidateCSRF(ctx context.Context, token, csrf string) error {
	record, err := s.store.FindSession(ctx, sha256.Sum256([]byte(token)))
	if err != nil || record.CSRFHash != sha256.Sum256([]byte(csrf)) {
		return ErrUnauthenticated
	}
	return nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	return s.store.DeleteSession(ctx, sha256.Sum256([]byte(token)))
}

func (s *Service) ChangePassword(ctx context.Context, identity Identity, currentPassword, nextPassword string) error {
	if len(nextPassword) < 12 {
		return fmt.Errorf("password must contain at least 12 characters")
	}
	admin, err := s.store.GetAdmin(ctx, identity.AdminID)
	if err != nil {
		return ErrUnauthenticated
	}
	valid, err := s.hasher.Verify(currentPassword, admin.PasswordHash)
	if err != nil || !valid {
		return ErrInvalidCredentials
	}
	hash, err := s.hasher.Hash(nextPassword)
	if err != nil {
		return err
	}
	return s.store.UpdateAdminPassword(ctx, admin.ID, hash, s.now())
}

func (s *Service) randomID(size int) (string, error) {
	return s.randomToken(size)
}

func (s *Service) randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(s.random, value); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (s *Service) recoveryCode() (string, error) {
	value := make([]byte, 10)
	if _, err := io.ReadFull(s.random, value); err != nil {
		return "", fmt.Errorf("generate recovery code: %w", err)
	}
	encoded := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value))
	return encoded[:4] + "-" + encoded[4:8] + "-" + encoded[8:12] + "-" + encoded[12:16], nil
}
