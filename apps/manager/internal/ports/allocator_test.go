package ports

import (
	"context"
	"path/filepath"
	"testing"

	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestAllocatorNeverReturnsReservedOrListeningPort(t *testing.T) {
	database := openPortStore(t)
	createPortProject(t, database, "bee")
	createPortProject(t, database, "nomo")
	allocator := NewAllocator(database, 18001, 18003, fakeProbe{busy: map[int]bool{18002: true}})

	first, err := allocator.Reserve(context.Background(), "bee", KindAPI)
	if err != nil {
		t.Fatalf("first Reserve() error = %v", err)
	}
	second, err := allocator.Reserve(context.Background(), "nomo", KindAPI)
	if err != nil {
		t.Fatalf("second Reserve() error = %v", err)
	}
	if first != 18001 || second != 18003 {
		t.Fatalf("reserved ports = %d, %d, want 18001, 18003", first, second)
	}
}

func TestAllocatorReturnsExistingReservationForSameProjectAndKind(t *testing.T) {
	database := openPortStore(t)
	createPortProject(t, database, "bee")
	allocator := NewAllocator(database, 18001, 18003, fakeProbe{})
	first, _ := allocator.Reserve(context.Background(), "bee", KindAPI)
	second, _ := allocator.Reserve(context.Background(), "bee", KindAPI)
	if first != second {
		t.Fatalf("second Reserve() = %d, want existing %d", second, first)
	}
}

type fakeProbe struct{ busy map[int]bool }

func (probe fakeProbe) Available(port int) bool { return !probe.busy[port] }

func openPortStore(t *testing.T) *store.Store {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func createPortProject(t *testing.T, database *store.Store, id string) {
	t.Helper()
	project := contracts.Project{ID: id, Name: id, Slug: id, Domain: id + ".example.com", SiteURL: "https://example.com", Status: contracts.ProjectStatusDraft, Health: contracts.HealthUnknown, SupabaseVersion: "self-hosted/v0.8.0", Preset: contracts.PresetLightweight}
	if err := database.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
}
