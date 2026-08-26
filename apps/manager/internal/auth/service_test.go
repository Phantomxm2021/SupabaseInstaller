package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/store"
)

func TestBootstrapCanOnlyCreateFirstAdmin(t *testing.T) {
	service, _ := newTestService(t)
	first, err := service.Bootstrap(context.Background(), "admin", "correct horse battery staple")
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if len(first.RecoveryCodes) != 10 {
		t.Fatalf("RecoveryCodes count = %d, want 10", len(first.RecoveryCodes))
	}
	_, err = service.Bootstrap(context.Background(), "other", "another secure password")
	if !errors.Is(err, ErrAlreadyBootstrapped) {
		t.Fatalf("second Bootstrap() error = %v, want ErrAlreadyBootstrapped", err)
	}
}

func TestLoginStoresHashNotCookieValue(t *testing.T) {
	service, db := newTestService(t)
	if _, err := service.Bootstrap(context.Background(), "admin", "correct horse battery staple"); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	session, err := service.Login(context.Background(), "admin", "correct horse battery staple")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	var storedHash []byte
	if err := db.QueryRow(`SELECT id_hash FROM sessions`).Scan(&storedHash); err != nil {
		t.Fatalf("read session hash: %v", err)
	}
	if strings.Contains(hex.EncodeToString(storedHash), session.Token) || string(storedHash) == session.Token {
		t.Fatal("session table contains raw cookie token")
	}
	identity, err := service.Authenticate(context.Background(), session.Token)
	if err != nil || identity.Username != "admin" {
		t.Fatalf("Authenticate() = %#v, %v", identity, err)
	}
}

func TestLoginRejectsWrongPasswordWithoutSession(t *testing.T) {
	service, db := newTestService(t)
	_, _ = service.Bootstrap(context.Background(), "admin", "correct horse battery staple")
	_, err := service.Login(context.Background(), "admin", "wrong password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("session count = %d, %v, want zero", count, err)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	service, _ := newTestService(t)
	_, _ = service.Bootstrap(context.Background(), "admin", "correct horse battery staple")
	session, _ := service.Login(context.Background(), "admin", "correct horse battery staple")
	if err := service.Logout(context.Background(), session.Token); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), session.Token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authenticate() after logout error = %v, want ErrUnauthenticated", err)
	}
}

func newTestService(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	now := func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) }
	service := NewService(s, NewPasswordHasher(Params{Memory: 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}), rand.Reader, now)
	return service, s.DB()
}
