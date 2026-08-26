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
	payload, _ := json.Marshal(projectDraftFixture())
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

func TestCreateProjectRejectsLegacyTopLevelProjections(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	mux := http.NewServeMux()
	RegisterProjectRoutes(mux, ProjectOptions{Projects: project.NewService(database, func() string { return "project-1" }, time.Now), Installer: &fakeInstaller{}})
	request := httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(`{"name":"Bee","slug":"bee","domain":"bee.example.com"}`))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("legacy create status = %d, body = %s; want bad request", response.Code, response.Body.String())
	}
}

func TestLifecycleEndpointReturnsDurableOperation(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	projects := project.NewService(database, func() string { return "project-1" }, time.Now)
	created, _ := projects.Create(context.Background(), projectDraftFixture())
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

func TestRetryEndpointCreatesNewInstallationOperation(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	projects := project.NewService(database, func() string { return "project-1" }, time.Now)
	created, _ := projects.Create(context.Background(), projectDraftFixture())
	installer := &fakeInstaller{}
	mux := http.NewServeMux()
	RegisterProjectRoutes(mux, ProjectOptions{Projects: projects, Installer: installer, Lifecycle: &fakeLifecycle{}})
	request := httptest.NewRequest(http.MethodPost, "/api/projects/"+created.ID+"/retry", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), "operation-1") || installer.projectID != created.ID {
		t.Fatalf("status=%d body=%s installer project=%s", response.Code, response.Body.String(), installer.projectID)
	}
}

type fakeInstaller struct{ projectID string }

func projectDraftFixture() contracts.ProjectDraft {
	cfg := project.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	return contracts.ProjectDraft{Name: "Bee", Slug: "bee", Preset: contracts.PresetLightweight, Configuration: cfg}
}

func (fake *fakeInstaller) CreateOperation(_ context.Context, projectID string) (operation.Operation, error) {
	fake.projectID = projectID
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
