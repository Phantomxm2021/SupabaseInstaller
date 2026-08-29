package project

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/store"
)

func TestServiceCreatesValidatedProjectWithServerOwnedFields(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	defer database.Close()
	service := NewService(database, func() string { return "project-1" }, func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) })

	created, err := service.Create(context.Background(), validDraft())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != "project-1" || created.Status != ProjectStatusDraft || created.Health != HealthUnknown {
		t.Fatalf("Create() = %#v", created)
	}
}

func TestServiceRejectsDuplicateSlug(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	ids := []string{"project-1", "project-2"}
	service := NewService(database, func() string { id := ids[0]; ids = ids[1:]; return id }, time.Now)
	_, _ = service.Create(context.Background(), validDraft())
	_, err := service.Create(context.Background(), validDraft())
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("second Create() error = %v, want ErrConflict", err)
	}
}

func TestServiceUsesServerTerminologyForEmptyGeneratedID(t *testing.T) {
	service := NewService(nil, func() string { return "" }, time.Now)
	_, err := service.Create(context.Background(), validDraft())
	if err == nil || !strings.Contains(err.Error(), "server ID generator") {
		t.Fatalf("Create() error = %v, want server terminology", err)
	}
}

func TestServiceUsesServerTerminologyForCreationSecretActions(t *testing.T) {
	draft := validDraft()
	draft.Configuration.General.StudioPassword.Action = "retain"
	service := NewService(nil, func() string { return "server-1" }, time.Now)
	_, err := service.Create(context.Background(), draft)
	if err == nil || !strings.Contains(err.Error(), "during server creation") {
		t.Fatalf("Create() error = %v, want server terminology", err)
	}
}
