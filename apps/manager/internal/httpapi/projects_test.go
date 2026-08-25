package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"supabase-manager/apps/manager/internal/lifecycle"
	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/project"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestCreateProjectReturnsOperationAndNeverSecret(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	projects := project.NewService(database, func() string { return "project-1" }, time.Now)
	installer := &fakeInstaller{}
	mux := http.NewServeMux()
	RegisterProjectRoutes(mux, ProjectOptions{Projects: projects, Installer: installer})
	payload, _ := json.Marshal(contracts.ProjectDraft{
		Name: "Bee", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0",
		Preset: contracts.PresetLightweight, Services: contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true},
	})
	request := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewReader(payload))
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"projectId":"project-1"`) || !strings.Contains(response.Body.String(), `"operationId":"operation-1"`) {
		t.Fatalf("response = %s", response.Body.String())
	}
	for _, forbidden := range []string{"postgresPassword", "jwtSecret", "serviceRoleKey"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("response leaked %s: %s", forbidden, response.Body.String())
		}
	}
}

func TestLifecycleEndpointReturnsDurableOperation(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	projects := project.NewService(database, func() string { return "project-1" }, time.Now)
	created, _ := projects.Create(context.Background(), contracts.ProjectDraft{Name: "Bee", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0", Preset: contracts.PresetLightweight, Services: contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true}})
	lifecycleManager := &fakeLifecycle{}
	mux := http.NewServeMux()
	RegisterProjectRoutes(mux, ProjectOptions{Projects: projects, Installer: &fakeInstaller{}, Lifecycle: lifecycleManager})
	request := httptest.NewRequest(http.MethodPost, "/api/projects/"+created.ID+"/stop", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), "lifecycle-op-1") || lifecycleManager.action != lifecycle.ActionStop {
		t.Fatalf("status=%d body=%s action=%s", response.Code, response.Body.String(), lifecycleManager.action)
	}
}

type fakeInstaller struct{}

func (*fakeInstaller) CreateOperation(context.Context, string) (operation.Operation, error) {
	return operation.Operation{ID: "operation-1", Status: operation.Queued}, nil
}

type fakeLifecycle struct{ action lifecycle.Action }

func (fake *fakeLifecycle) Queue(_ context.Context, _ contracts.Project, action lifecycle.Action, _ string) (operation.Operation, error) {
	fake.action = action
	return operation.Operation{ID: "lifecycle-op-1", Status: operation.Queued}, nil
}
func (*fakeLifecycle) Run(context.Context, contracts.Project, lifecycle.Action, operation.Operation) (operation.Operation, error) {
	return operation.Operation{ID: "lifecycle-op-1", Status: operation.Succeeded}, nil
}
func (*fakeInstaller) Run(context.Context, contracts.Project, operation.Operation) (operation.Operation, error) {
	return operation.Operation{ID: "operation-1", Status: operation.Succeeded}, nil
}
