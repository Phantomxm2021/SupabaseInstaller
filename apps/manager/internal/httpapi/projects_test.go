package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
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

func TestCreateProjectStagesTLSAndPersistsOnlySafePaths(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	projects := project.NewService(database, func() string { return "project-1" }, time.Now)
	stager := &fakeManagedTLSStager{}
	mux := http.NewServeMux()
	RegisterProjectRoutes(mux, ProjectOptions{Projects: projects, Installer: &fakeInstaller{}, ManagedTLS: stager})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	draft, _ := json.Marshal(projectDraftFixture())
	_ = writer.WriteField("draft", string(draft))
	certificate, _ := writer.CreateFormFile("certificate", "origin.pem")
	_, _ = certificate.Write([]byte("-----BEGIN CERTIFICATE-----\ncertificate"))
	privateKey, _ := writer.CreateFormFile("privateKey", "origin.key")
	_, _ = privateKey.Write([]byte("-----BEGIN PRIVATE KEY-----\nkey"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/projects", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
	}
	if string(stager.input.PrivateKeyPEM) != "-----BEGIN PRIVATE KEY-----\nkey" || stager.input.BaseDomain != "example.com" {
		t.Fatalf("staged input = %#v", stager.input)
	}
	snapshot, err := database.GetDesiredConfiguration(context.Background(), "project-1")
	if err != nil || snapshot.Configuration.Network.ManagedTLS == nil {
		t.Fatalf("managed TLS was not persisted safely: %#v err=%v", snapshot.Configuration.Network.ManagedTLS, err)
	}
	if got := snapshot.Configuration.Network.ManagedTLS.CertificateFile; got != "/etc/nginx/ssl/cloudflare-origin-example.pem" {
		t.Fatalf("certificate file = %q", got)
	}
	if strings.Contains(response.Body.String(), "PRIVATE KEY") {
		t.Fatalf("response leaked TLS material: %s", response.Body.String())
	}
}

func TestCreateProjectDoesNotStageTLSForInvalidDraft(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	stager := &fakeManagedTLSStager{}
	mux := http.NewServeMux()
	RegisterProjectRoutes(mux, ProjectOptions{Projects: project.NewService(database, func() string { return "project-1" }, time.Now), Installer: &fakeInstaller{}, ManagedTLS: stager})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	draft := projectDraftFixture()
	draft.Name = ""
	encoded, _ := json.Marshal(draft)
	_ = writer.WriteField("draft", string(encoded))
	certificate, _ := writer.CreateFormFile("certificate", "origin.pem")
	_, _ = certificate.Write([]byte("certificate"))
	privateKey, _ := writer.CreateFormFile("privateKey", "origin.key")
	_, _ = privateKey.Write([]byte("private-key"))
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/projects", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity || len(stager.input.CertificatePEM) != 0 {
		t.Fatalf("status/staged = %d/%q", response.Code, stager.input.CertificatePEM)
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

func TestGetMissingProjectReturnsServerTerminology(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	mux := http.NewServeMux()
	RegisterProjectRoutes(mux, ProjectOptions{Projects: project.NewService(database, func() string { return "project-1" }, time.Now), Installer: &fakeInstaller{}})
	request := httptest.NewRequest(http.MethodGet, "/api/projects/missing", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), `"message":"Server was not found"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
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

func TestDeleteEndpointForceDeletesSynchronously(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	projects := project.NewService(database, func() string { return "project-1" }, time.Now)
	created, _ := projects.Create(context.Background(), projectDraftFixture())
	lifecycleManager := &fakeLifecycle{}
	mux := http.NewServeMux()
	RegisterProjectRoutes(mux, ProjectOptions{Projects: projects, Installer: &fakeInstaller{}, Lifecycle: lifecycleManager})
	request := httptest.NewRequest(http.MethodDelete, "/api/projects/"+created.ID, strings.NewReader(`{"mode":"data","confirmation":"Bee"}`))
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent || !lifecycleManager.forceDelete || lifecycleManager.action != "" {
		t.Fatalf("status=%d body=%s forceDelete=%v queuedAction=%s", response.Code, response.Body.String(), lifecycleManager.forceDelete, lifecycleManager.action)
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

func TestListProjectsUsesLiveProvisionerHealth(t *testing.T) {
	database, _ := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	defer database.Close()
	projects := project.NewService(database, func() string { return "project-1" }, time.Now)
	created, _ := projects.Create(context.Background(), projectDraftFixture())
	if err := database.UpdateProjectStatus(context.Background(), created.ID, contracts.ProjectStatusFailed, contracts.HealthUnhealthy); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterProjectRoutes(mux, ProjectOptions{Projects: projects, Installer: &fakeInstaller{}, Inspector: fakeInspector{health: contracts.HealthHealthy}})
	request := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"health":"HEALTHY"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type fakeInstaller struct{ projectID string }

func projectDraftFixture() contracts.ProjectDraft {
	cfg := project.DefaultConfiguration(contracts.PresetLightweight)
	cfg.General = contracts.GeneralConfig{Domain: "bee.example.com", SiteURL: "https://example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	return contracts.ProjectDraft{Name: "Bee", Slug: "bee", Preset: contracts.PresetLightweight, Configuration: cfg}
}

type fakeManagedTLSStager struct {
	input contracts.StageManagedTLSRequest
}

func (fake *fakeManagedTLSStager) StageManagedTLS(_ context.Context, input contracts.StageManagedTLSRequest) (contracts.StageManagedTLSResponse, error) {
	fake.input = input
	config, err := contracts.ManagedTLSPaths(input.CertificateName, "https://"+input.BaseDomain)
	if err != nil {
		return contracts.StageManagedTLSResponse{}, err
	}
	return contracts.StageManagedTLSResponse{ManagedTLSConfig: config, Created: true}, nil
}

func (fake *fakeInstaller) CreateOperation(_ context.Context, projectID string) (operation.Operation, error) {
	fake.projectID = projectID
	return operation.Operation{ID: "operation-1", Status: operation.Queued}, nil
}

type fakeLifecycle struct {
	action      lifecycle.Action
	forceDelete bool
}

func (fake *fakeLifecycle) Queue(_ context.Context, _ contracts.Project, action lifecycle.Action, _ string) (operation.Operation, error) {
	fake.action = action
	return operation.Operation{ID: "lifecycle-op-1", Status: operation.Queued}, nil
}
func (*fakeLifecycle) Run(context.Context, contracts.Project, lifecycle.Action, operation.Operation) (operation.Operation, error) {
	return operation.Operation{ID: "lifecycle-op-1", Status: operation.Succeeded}, nil
}
func (fake *fakeLifecycle) ForceDelete(context.Context, contracts.Project, lifecycle.Action, string) error {
	fake.forceDelete = true
	return nil
}
func (*fakeInstaller) Run(context.Context, contracts.Project, operation.Operation) (operation.Operation, error) {
	return operation.Operation{ID: "operation-1", Status: operation.Succeeded}, nil
}

type fakeInspector struct{ health contracts.HealthStatus }

func (fake fakeInspector) Inspect(_ context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	return contracts.InspectProjectResponse{ProjectID: request.ProjectID, Health: fake.health}, nil
}
