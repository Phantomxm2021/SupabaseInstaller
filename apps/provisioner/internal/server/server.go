package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	provisionerauth "supabase-manager/apps/provisioner/internal/auth"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/render"
	"supabase-manager/internal/contracts"
)

type Backend interface {
	Lifecycle(ctx context.Context, request contracts.LifecycleRequest) error
	Inspect(ctx context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error)
	Reconcile(ctx context.Context, request contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error)
}

type Options struct {
	ManagerToken string
	ProjectFS    *projectfs.Root
	Backend      Backend
}

type server struct {
	projectFS *projectfs.Root
	backend   Backend
}

func New(options Options) http.Handler {
	service := &server{projectFS: options.ProjectFS, backend: options.Backend}
	private := http.NewServeMux()
	private.HandleFunc("POST /internal/v1/projects/prepare", service.prepare)
	private.HandleFunc("POST /internal/v1/projects/lifecycle", service.lifecycle)
	private.HandleFunc("POST /internal/v1/projects/inspect", service.inspect)
	private.HandleFunc("POST /internal/v1/projects/reconcile", service.reconcile)
	root := http.NewServeMux()
	root.Handle("/internal/", provisionerauth.RequireManagerToken(options.ManagerToken, private))
	root.HandleFunc("GET /health/live", func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusNoContent) })
	return root
}

func (s *server) prepare(response http.ResponseWriter, request *http.Request) {
	var input contracts.PrepareProjectRequest
	if err := decodeJSON(response, request, &input); err != nil || input.OperationID == "" || input.IdempotencyKey == "" || input.ProjectID == "" {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A typed prepare request is required")
		return
	}
	var prepared contracts.PrepareProjectResponse
	metadata, err := s.projectFS.UpdateMetadata(input.Slug, func(metadata *projectfs.Metadata) error {
		if stored, ok := metadata.Idempotency[input.IdempotencyKey]; ok {
			return json.Unmarshal(stored, &prepared)
		}
		if metadata.Revision != input.ExpectedRevision {
			return errStaleRevision
		}
		rendered, err := render.Lightweight(render.Input{
			ProjectID: input.ProjectID, Slug: input.Slug, Domain: input.Domain, SiteURL: input.SiteURL,
			APIPort: input.APIPort, Secrets: input.Secrets,
		})
		if err != nil {
			return err
		}
		if err := s.projectFS.WriteRuntimeFiles(input.Slug, []byte(rendered.Compose), []byte(rendered.Env)); err != nil {
			return err
		}
		projectDir, err := s.projectFS.ProjectPath(input.Slug)
		if err != nil {
			return err
		}
		prepared = contracts.PrepareProjectResponse{
			OperationID: input.OperationID, IdempotencyKey: input.IdempotencyKey, ProjectID: input.ProjectID,
			Slug: input.Slug, ProjectDir: projectDir, Revision: input.NextRevision,
		}
		encoded, _ := json.Marshal(prepared)
		metadata.ProjectID = input.ProjectID
		metadata.ProjectName = input.ProjectName
		metadata.Revision = input.NextRevision
		metadata.Idempotency[input.IdempotencyKey] = encoded
		return nil
	})
	if errors.Is(err, errStaleRevision) {
		writeError(response, http.StatusConflict, "STALE_CONFIG_REVISION", "Project configuration revision is stale")
		return
	}
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "PREPARE_FAILED", err.Error())
		return
	}
	prepared.Revision = metadata.Revision
	writeJSON(response, http.StatusCreated, prepared)
}

func (s *server) lifecycle(response http.ResponseWriter, request *http.Request) {
	if s.backend == nil {
		writeError(response, http.StatusServiceUnavailable, "BACKEND_UNAVAILABLE", "Provisioner lifecycle backend is unavailable")
		return
	}
	var input contracts.LifecycleRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A typed lifecycle request is required")
		return
	}
	if err := s.backend.Lifecycle(request.Context(), input); err != nil {
		writeError(response, http.StatusUnprocessableEntity, "LIFECYCLE_FAILED", err.Error())
		return
	}
	response.WriteHeader(http.StatusAccepted)
}

func (s *server) inspect(response http.ResponseWriter, request *http.Request) {
	if s.backend == nil {
		writeError(response, http.StatusServiceUnavailable, "BACKEND_UNAVAILABLE", "Provisioner inspection backend is unavailable")
		return
	}
	var input contracts.InspectProjectRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A typed inspect request is required")
		return
	}
	result, err := s.backend.Inspect(request.Context(), input)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "INSPECT_FAILED", err.Error())
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (s *server) reconcile(response http.ResponseWriter, request *http.Request) {
	if s.backend == nil {
		writeError(response, http.StatusServiceUnavailable, "BACKEND_UNAVAILABLE", "Provisioner reconciliation backend is unavailable")
		return
	}
	var input contracts.ReconcileProjectRequest
	if err := decodeJSON(response, request, &input); err != nil || input.OperationID == "" || input.IdempotencyKey == "" || input.ProjectID == "" || input.Slug == "" {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A typed reconcile request is required")
		return
	}
	result, err := s.backend.Reconcile(request.Context(), input)
	if errors.Is(err, contracts.ErrStaleConfigRevision) {
		writeError(response, http.StatusConflict, "STALE_CONFIG_REVISION", "Project configuration revision is stale")
		return
	}
	if err != nil {
		// Runtime errors are deliberately generic: rendered environment files
		// and secret values must never cross this private API boundary.
		writeError(response, http.StatusUnprocessableEntity, "RECONCILE_FAILED", "Project runtime reconciliation failed")
		return
	}
	writeJSON(response, http.StatusOK, result)
}

var errStaleRevision = errors.New("stale config revision")

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}

func writeError(response http.ResponseWriter, status int, code, message string) {
	writeJSON(response, status, contracts.ErrorEnvelope{Error: contracts.APIError{Code: code, Message: message}})
}
