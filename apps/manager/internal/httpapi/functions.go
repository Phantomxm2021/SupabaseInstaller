package httpapi

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	managerfunctions "supabase-manager/apps/manager/internal/functions"
	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/project"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type FunctionsOptions struct {
	Service  *managerfunctions.Service
	Projects *project.Service
}

func RegisterFunctionsRoutes(mux *http.ServeMux, options FunctionsOptions) {
	if options.Service == nil || options.Projects == nil {
		return
	}
	h := functionHandlers{service: options.Service, projects: options.Projects}
	mux.HandleFunc("GET /api/projects/{id}/functions", h.list)
	mux.HandleFunc("POST /api/projects/{id}/functions/{name}", h.deploy)
	mux.HandleFunc("POST /api/projects/{id}/functions/{name}/deploy", h.deploy)
	mux.HandleFunc("POST /api/projects/{id}/functions/{name}/rollback", h.rollback)
	mux.HandleFunc("DELETE /api/projects/{id}/functions/{name}", h.delete)
}

type functionHandlers struct {
	service  *managerfunctions.Service
	projects *project.Service
}

func (h functionHandlers) getProject(w http.ResponseWriter, r *http.Request) (contracts.Project, bool) {
	p, err := h.projects.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Server was not found")
		return contracts.Project{}, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "PROJECT_GET_FAILED", "Unable to read server")
		return contracts.Project{}, false
	}
	return p, true
}

func (h functionHandlers) list(w http.ResponseWriter, r *http.Request) {
	p, ok := h.getProject(w, r)
	if !ok {
		return
	}
	items, err := h.service.List(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusBadGateway, "FUNCTIONS_LIST_FAILED", "Unable to list functions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"functions": items, "enabled": p.Services.Functions})
}

const maxFunctionUpload = 20 << 20

func (h functionHandlers) deploy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := contracts.ValidateFunctionName(name); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_FUNCTION_NAME", "Function name is invalid")
		return
	}
	p, ok := h.getProject(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxFunctionUpload+1024)
	if err := r.ParseMultipartForm(maxFunctionUpload + 1024); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "FUNCTION_UPLOAD_TOO_LARGE", "Function ZIP is too large or invalid")
		return
	}
	file, header, err := r.FormFile("archive")
	if err != nil {
		writeError(w, http.StatusBadRequest, "FUNCTION_ARCHIVE_REQUIRED", "Upload a ZIP archive in the archive field")
		return
	}
	defer file.Close()
	if !strings.EqualFold(filepath.Ext(header.Filename), ".zip") {
		writeError(w, http.StatusBadRequest, "FUNCTION_ARCHIVE_REQUIRED", "Upload a .zip archive")
		return
	}
	if filepath.Base(header.Filename) != name+".zip" {
		writeError(w, http.StatusUnprocessableEntity, "FUNCTION_ARCHIVE_NAME_MISMATCH", "ZIP filename must match the function name")
		return
	}
	queued, err := h.service.QueueDeploy(r.Context(), p, name, file)
	if err != nil {
		writeFunctionQueueError(w, err)
		return
	}
	h.runAsync(p, queued)
	writeJSON(w, http.StatusAccepted, map[string]string{"projectId": p.ID, "operationId": queued.ID})
}

func (h functionHandlers) rollback(w http.ResponseWriter, r *http.Request) {
	p, ok := h.getProject(w, r)
	if !ok {
		return
	}
	queued, err := h.service.QueueRollback(r.Context(), p, r.PathValue("name"))
	if err != nil {
		writeFunctionQueueError(w, err)
		return
	}
	h.runAsync(p, queued)
	writeJSON(w, http.StatusAccepted, map[string]string{"projectId": p.ID, "operationId": queued.ID})
}

func (h functionHandlers) delete(w http.ResponseWriter, r *http.Request) {
	p, ok := h.getProject(w, r)
	if !ok {
		return
	}
	var input struct {
		Confirmation string `json:"confirmation"`
	}
	if err := decodeJSON(w, r, &input); err != nil || input.Confirmation != r.PathValue("name") {
		writeError(w, http.StatusBadRequest, "INVALID_CONFIRMATION", "Exact function name confirmation is required")
		return
	}
	queued, err := h.service.QueueDelete(r.Context(), p, r.PathValue("name"), input.Confirmation)
	if err != nil {
		writeFunctionQueueError(w, err)
		return
	}
	h.runAsync(p, queued)
	writeJSON(w, http.StatusAccepted, map[string]string{"projectId": p.ID, "operationId": queued.ID})
}

func (h functionHandlers) runAsync(p contracts.Project, queued operation.Operation) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		_, _ = h.service.Run(ctx, p, queued)
	}()
}

func writeFunctionQueueError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConfigurationBusy) {
		writeError(w, http.StatusConflict, "FUNCTIONS_BUSY", "Another operation is active for this server")
		return
	}
	writeError(w, http.StatusUnprocessableEntity, "FUNCTION_OPERATION_INVALID", "Function operation could not be queued")
}
