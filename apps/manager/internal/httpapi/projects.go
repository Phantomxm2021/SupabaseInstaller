package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"supabase-manager/apps/manager/internal/authadmin"
	"supabase-manager/apps/manager/internal/configuration"
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

type LifecycleManager interface {
	Queue(ctx context.Context, project contracts.Project, action lifecycle.Action, confirmation string) (operation.Operation, error)
	Run(ctx context.Context, project contracts.Project, action lifecycle.Action, operation operation.Operation) (operation.Operation, error)
}

type ProjectOptions struct {
	Projects      *project.Service
	AuthAdmin     *authadmin.Service
	Installer     Installer
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
		writeError(response, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project was not found")
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, "PROJECT_GET_FAILED", "Unable to read project")
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
		_, _ = handlers.options.Installer.Run(ctx, found, retryOperation)
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
	handlers.queueLifecycle(response, request, action, input.Confirmation)
}

func (handlers projectHandlers) queueLifecycle(response http.ResponseWriter, request *http.Request, action lifecycle.Action, confirmation string) {
	if handlers.options.Lifecycle == nil {
		writeError(response, http.StatusServiceUnavailable, "LIFECYCLE_UNAVAILABLE", "Project lifecycle is unavailable")
		return
	}
	found, err := handlers.options.Projects.Get(request.Context(), request.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(response, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project was not found")
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, "PROJECT_GET_FAILED", "Unable to read project")
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
		writeError(response, http.StatusConflict, "PROJECT_CONFLICT", "Project slug or domain already exists")
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
		writeError(response, http.StatusInternalServerError, "PROJECT_LIST_FAILED", "Unable to list projects")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"projects": projects})
}

func (handlers projectHandlers) get(response http.ResponseWriter, request *http.Request) {
	found, err := handlers.options.Projects.Get(request.Context(), request.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(response, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project was not found")
		return
	}
	if err != nil {
		writeError(response, http.StatusInternalServerError, "PROJECT_GET_FAILED", "Unable to read project")
		return
	}
	writeJSON(response, http.StatusOK, found)
}
