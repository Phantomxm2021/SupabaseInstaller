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

	firstSet, err := allocator.ReserveMany(context.Background(), "bee", []Kind{KindAPI})
	if err != nil {
		t.Fatalf("first Reserve() error = %v", err)
	}
	secondSet, err := allocator.ReserveMany(context.Background(), "nomo", []Kind{KindAPI})
	if err != nil {
		t.Fatalf("second Reserve() error = %v", err)
	}
	if firstSet[KindAPI] != 18001 || secondSet[KindAPI] != 18003 {
		t.Fatalf("reserved ports = %d, %d, want 18001, 18003", firstSet[KindAPI], secondSet[KindAPI])
	}
}

func TestAllocatorReturnsExistingReservationForSameProjectAndKind(t *testing.T) {
	database := openPortStore(t)
	createPortProject(t, database, "bee")
	allocator := NewAllocator(database, 18001, 18003, fakeProbe{})
	first, _ := allocator.ReserveMany(context.Background(), "bee", []Kind{KindAPI})
	second, _ := allocator.ReserveMany(context.Background(), "bee", []Kind{KindAPI})
	if first[KindAPI] != second[KindAPI] {
		t.Fatalf("second ReserveMany() = %d, want existing %d", second[KindAPI], first[KindAPI])
	}
}

func TestAllocatorReservesSelectedPortsAtomically(t *testing.T) {
	database := openPortStore(t)
	createPortProject(t, database, "bee")
	allocator := NewAllocator(database, 18001, 18001, fakeProbe{})
	if _, err := allocator.ReserveMany(context.Background(), "bee", []Kind{KindAPI, KindStudio}); err != ErrExhausted {
		t.Fatalf("ReserveMany() error = %v, want exhausted", err)
	}
	var count int
	if err := database.DB().QueryRow(`SELECT COUNT(*) FROM port_allocations WHERE project_id = ?`, "bee").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("partial reservation count = %d, want 0", count)
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
	configuration := contracts.ProjectConfiguration{General: contracts.GeneralConfig{Domain: project.Domain, SiteURL: project.SiteURL, SupabaseVersion: project.SupabaseVersion}, Services: project.Services}
	if err := database.CreateProject(context.Background(), project, configuration); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
}
