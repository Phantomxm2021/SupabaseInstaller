package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"supabase-manager/apps/manager/internal/authadmin"
	"supabase-manager/apps/manager/internal/configuration"
	"supabase-manager/apps/manager/internal/install"
	"supabase-manager/apps/manager/internal/lifecycle"
	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/project"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type Installer interface {
	CreateOperation(ctx context.Context, projectID string) (operation.Operation, error)
	Run(ctx context.Context, project contracts.Project, operation operation.Operation) (operation.Operation, error)
}

// ProjectInspector is the live runtime health boundary. Project health is
// persisted for fast reads, but Docker can be restarted outside Manager; API
// reads therefore refresh the displayed health from Provisioner when present.
type ProjectInspector interface {
	Inspect(ctx context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error)
}

type LifecycleManager interface {
	Queue(ctx context.Context, project contracts.Project, action lifecycle.Action, confirmation string) (operation.Operation, error)
	Run(ctx context.Context, project contracts.Project, action lifecycle.Action, operation operation.Operation) (operation.Operation, error)
	ForceDelete(ctx context.Context, project contracts.Project, action lifecycle.Action, confirmation string) error
}

type ProjectOptions struct {
	Projects      *project.Service
	AuthAdmin     *authadmin.Service
	Installer     Installer
	Inspector     ProjectInspector
	Lifecycle     LifecycleManager
	Configuration *configuration.Orchestrator
}

type projectHandlers struct {
	options ProjectOptions
}

func RegisterProjectRoutes(mux *http.ServeMux, options ProjectOptions) {
	handlers := projectHandlers{options: options}
	mux.HandleFunc("POST /api/projects", handlers.create)
	mux.HandleFunc("GET /api/projects", handlers.list)
	mux.HandleFunc("GET /api/projects/{id}", handlers.get)
	mux.HandleFunc("GET /api/projects/{id}/auth/users", handlers.listUsers)
	mux.HandleFunc("POST /api/projects/{id}/auth/users", handlers.createUser)
	mux.HandleFunc("POST /api/projects/{id}/auth/users/invite", handlers.inviteUser)
	mux.HandleFunc("GET /api/projects/{id}/auth/oauth-apps", handlers.listOAuthApps)
	mux.HandleFunc("POST /api/projects/{id}/auth/oauth-apps", handlers.createOAuthApp)
	mux.HandleFunc("POST /api/projects/{id}/start", handlers.lifecycle(lifecycle.ActionStart))
	mux.HandleFunc("POST /api/projects/{id}/stop", handlers.lifecycle(lifecycle.ActionStop))
	mux.HandleFunc("POST /api/projects/{id}/restart", handlers.lifecycle(lifecycle.ActionRestart))
	mux.HandleFunc("POST /api/projects/{id}/retry", handlers.retry)
	mux.HandleFunc("POST /api/projects/{id}/rollback", handlers.lifecycle(lifecycle.ActionDeleteRuntime))
	mux.HandleFunc("DELETE /api/projects/{id}", handlers.delete)
}

func (handlers projectHandlers) retry(response http.ResponseWriter, request *http.Request) {
	found, err := handlers.options.Projects.Get(request.Context(), request.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(response, http.StatusNotFound, "PROJECT_NOT_FOUND", "Server was not found")
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, "PROJECT_GET_FAILED", "Unable to read server")
		return
	}
	retryOperation, err := handlers.options.Installer.CreateOperation(request.Context(), found.ID)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "OPERATION_CREATE_FAILED", "Unable to create retry operation")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		if _, runErr := handlers.options.Installer.Run(ctx, found, retryOperation); runErr != nil {
			// The durable operation remains the source of truth for the UI, but
			// a structured record is essential when a retry fails before an
			// operator can open its detail panel.
			slog.Error("project retry failed", "project_id", found.ID, "operation_id", retryOperation.ID, "error", runErr)
		}
	}()
	writeJSON(response, http.StatusAccepted, map[string]string{"projectId": found.ID, "operationId": retryOperation.ID})
}

func (handlers projectHandlers) lifecycle(action lifecycle.Action) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		handlers.queueLifecycle(response, request, action, "")
	}
}

func (handlers projectHandlers) delete(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Mode         string `json:"mode"`
		Confirmation string `json:"confirmation"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_JSON", "Request body is invalid")
		return
	}
	action := lifecycle.ActionDeleteRuntime
	if input.Mode == "data" {
		action = lifecycle.ActionDeleteData
	} else if input.Mode != "runtime" {
		writeError(response, http.StatusUnprocessableEntity, "INVALID_DELETE_MODE", "Delete mode must be runtime or data")
		return
	}
	if handlers.options.Lifecycle == nil {
		writeError(response, http.StatusServiceUnavailable, "LIFECYCLE_UNAVAILABLE", "Server lifecycle is unavailable")
		return
	}
	found, err := handlers.options.Projects.Get(request.Context(), request.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(response, http.StatusNotFound, "PROJECT_NOT_FOUND", "Server was not found")
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, "PROJECT_GET_FAILED", "Unable to read server")
		return
	}
	if err := handlers.options.Lifecycle.ForceDelete(request.Context(), found, action, input.Confirmation); err != nil {
		writeError(response, http.StatusUnprocessableEntity, "DELETE_FAILED", err.Error())
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (handlers projectHandlers) queueLifecycle(response http.ResponseWriter, request *http.Request, action lifecycle.Action, confirmation string) {
	if handlers.options.Lifecycle == nil {
		writeError(response, http.StatusServiceUnavailable, "LIFECYCLE_UNAVAILABLE", "Server lifecycle is unavailable")
		return
	}
	found, err := handlers.options.Projects.Get(request.Context(), request.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(response, http.StatusNotFound, "PROJECT_NOT_FOUND", "Server was not found")
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, "PROJECT_GET_FAILED", "Unable to read server")
		return
	}
	queued, err := handlers.options.Lifecycle.Queue(request.Context(), found, action, confirmation)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "INVALID_LIFECYCLE", err.Error())
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_, _ = handlers.options.Lifecycle.Run(ctx, found, action, queued)
	}()
	writeJSON(response, http.StatusAccepted, map[string]string{"projectId": found.ID, "operationId": queued.ID})
}

func (handlers projectHandlers) create(response http.ResponseWriter, request *http.Request) {
	var draft contracts.ProjectDraft
	if err := decodeJSON(response, request, &draft); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_JSON", "Request body is invalid")
		return
	}
	created, err := handlers.options.Projects.Create(request.Context(), draft)
	switch {
	case errors.Is(err, project.ErrConflict):
		writeError(response, http.StatusConflict, "PROJECT_CONFLICT", "Server slug or domain already exists")
		return
	case err != nil:
		writeError(response, http.StatusUnprocessableEntity, "INVALID_PROJECT", err.Error())
		return
	}
	installOperation, err := handlers.options.Installer.CreateOperation(request.Context(), created.ID)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "OPERATION_CREATE_FAILED", "Unable to create installation operation")
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		_, _ = handlers.options.Installer.Run(ctx, created, installOperation)
	}()
	writeJSON(response, http.StatusAccepted, map[string]string{"projectId": created.ID, "operationId": installOperation.ID})
}

func (handlers projectHandlers) list(response http.ResponseWriter, request *http.Request) {
	projects, err := handlers.options.Projects.List(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, "PROJECT_LIST_FAILED", "Unable to list servers")
		return
	}
	for index := range projects {
		projects[index] = handlers.refreshHealth(request.Context(), projects[index])
	}
	writeJSON(response, http.StatusOK, map[string]any{"projects": projects})
}

func (handlers projectHandlers) get(response http.ResponseWriter, request *http.Request) {
	found, err := handlers.options.Projects.Get(request.Context(), request.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(response, http.StatusNotFound, "PROJECT_NOT_FOUND", "Server was not found")
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, "PROJECT_GET_FAILED", "Unable to read server")
		return
	}
	found = handlers.refreshHealth(request.Context(), found)
	writeJSON(response, http.StatusOK, found)
}

func (handlers projectHandlers) refreshHealth(ctx context.Context, found contracts.Project) contracts.Project {
	if handlers.options.Inspector == nil || found.Status == contracts.ProjectStatusDraft || found.Status == contracts.ProjectStatusInstalling {
		return found
	}
	inspection, err := handlers.options.Inspector.Inspect(ctx, contracts.InspectProjectRequest{
		ProjectID: found.ID, Slug: found.Slug, EnabledServices: install.EnabledComposeServices(found.Services),
	})
	if err != nil {
		return found
	}
	found.Health = inspection.Health
	return found
}
