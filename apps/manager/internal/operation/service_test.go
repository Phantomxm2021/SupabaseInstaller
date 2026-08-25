package operation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestFailedInstallCanEnterRollbackButCannotSucceed(t *testing.T) {
	service := newOperationService(t)
	operation, err := service.Create(context.Background(), "bee", TypeCreate)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := service.Start(context.Background(), operation.ID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := service.Fail(context.Background(), operation.ID, "START_AUTH", errors.New("unhealthy")); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if err := service.BeginRollback(context.Background(), operation.ID); err != nil {
		t.Fatalf("BeginRollback() error = %v", err)
	}
	if err := service.Succeed(context.Background(), operation.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Succeed() error = %v, want ErrInvalidTransition", err)
	}
}

func TestEventsAfterReturnsOrderedReplay(t *testing.T) {
	service := newOperationService(t)
	operation, _ := service.Create(context.Background(), "bee", TypeCreate)
	_ = service.Start(context.Background(), operation.ID)
	_ = service.StartStep(context.Background(), operation.ID, "VALIDATE_HOST", 5)
	_ = service.CompleteStep(context.Background(), operation.ID, "VALIDATE_HOST", 10)

	events, err := service.EventsAfter(context.Background(), operation.ID, 1)
	if err != nil {
		t.Fatalf("EventsAfter() error = %v", err)
	}
	if len(events) != 3 || events[0].Sequence != 2 || events[2].Sequence != 4 {
		t.Fatalf("EventsAfter() = %#v, want sequences 2..4", events)
	}
}

func newOperationService(t *testing.T) *Service {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", Status: contracts.ProjectStatusDraft, Health: contracts.HealthUnknown, SupabaseVersion: "self-hosted/v0.8.0", Preset: contracts.PresetLightweight}
	if err := database.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	ids := []string{"op-1", "op-2"}
	return NewService(database, func() string {
		id := ids[0]
		ids = ids[1:]
		return id
	}, func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) })
}
