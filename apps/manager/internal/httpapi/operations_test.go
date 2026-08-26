package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestOperationEventsResumeAfterLastEventID(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", Status: contracts.ProjectStatusDraft, Health: contracts.HealthUnknown, SupabaseVersion: "self-hosted/v0.8.0", Preset: contracts.PresetLightweight}
	_ = database.CreateProject(context.Background(), project, contracts.ProjectConfiguration{General: contracts.GeneralConfig{Domain: project.Domain, SiteURL: project.SiteURL, SupabaseVersion: project.SupabaseVersion}, Services: project.Services})
	operations := operation.NewService(database, func() string { return "op-1" }, time.Now)
	created, _ := operations.Create(context.Background(), "bee", operation.TypeCreate)
	_ = operations.Start(context.Background(), created.ID)
	_ = operations.StartStep(context.Background(), created.ID, "VALIDATE_HOST", 5)
	_ = operations.Succeed(context.Background(), created.ID)
	mux := http.NewServeMux()
	RegisterOperationRoutes(mux, operations)
	request := httptest.NewRequest(http.MethodGet, "/api/operations/op-1/events", nil)
	request.Header.Set("Last-Event-ID", "1")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "id: 2") || !strings.Contains(response.Body.String(), "id: 4") || strings.Contains(response.Body.String(), "id: 1") {
		t.Fatalf("SSE response = status %d, body %s", response.Code, response.Body.String())
	}
}
