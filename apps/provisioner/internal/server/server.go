package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	provisionerauth "supabase-manager/apps/provisioner/internal/auth"
	"supabase-manager/apps/provisioner/internal/projectfs"
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
	private.HandleFunc("POST /internal/v1/projects/lifecycle", service.lifecycle)
	private.HandleFunc("POST /internal/v1/projects/inspect", service.inspect)
	private.HandleFunc("POST /internal/v1/projects/reconcile", service.reconcile)
	private.HandleFunc("POST /internal/v1/projects/rotate-database-password", service.rotateDatabasePassword)
	private.HandleFunc("POST /internal/v1/projects/rollback-database-password", service.rollbackDatabasePassword)
	private.HandleFunc("POST /internal/v1/projects/confirm-database-password-rotation", service.confirmDatabasePasswordRotation)
	root := http.NewServeMux()
	root.Handle("/internal/", provisionerauth.RequireManagerToken(options.ManagerToken, private))
	root.HandleFunc("GET /health/live", func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusNoContent) })
	return root
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
	if errors.Is(err, contracts.ErrInvalidReconcileRevision) {
		writeError(response, http.StatusBadRequest, "INVALID_CONFIG_REVISION", "Next revision must advance the typed configuration snapshot")
		return
	}
	if err != nil {
		var failure *contracts.ReconcileFailure
		if errors.As(err, &failure) && failure.Response.Error != nil {
			writeJSON(response, http.StatusUnprocessableEntity, failure.Response)
			return
		}
		// Runtime errors are deliberately generic: rendered environment files
		// and secret values must never cross this private API boundary.
		writeError(response, http.StatusUnprocessableEntity, "RECONCILE_FAILED", "Project runtime reconciliation failed")
		return
	}
	writeJSON(response, http.StatusOK, result)
}

type passwordRotationBackend interface {
	RotateDatabasePassword(context.Context, contracts.RotateDatabasePasswordRequest) (contracts.RotateDatabasePasswordResponse, error)
}

func (s *server) rotateDatabasePassword(response http.ResponseWriter, request *http.Request) {
	backend, ok := s.backend.(passwordRotationBackend)
	if !ok {
		writeError(response, http.StatusServiceUnavailable, "ROTATION_UNAVAILABLE", "Database password rotation is unavailable")
		return
	}
	var input contracts.RotateDatabasePasswordRequest
	if err := decodeJSON(response, request, &input); err != nil || input.OperationID == "" || input.IdempotencyKey == "" || input.ProjectID == "" || input.Slug == "" || input.OldPassword == "" || input.NewPassword == "" {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A typed password rotation request is required")
		return
	}
	result, err := backend.RotateDatabasePassword(request.Context(), input)
	if errors.Is(err, contracts.ErrStaleConfigRevision) {
		writeError(response, http.StatusConflict, "STALE_CONFIG_REVISION", "Project configuration revision is stale")
		return
	}
	if errors.Is(err, contracts.ErrInvalidReconcileRevision) {
		writeError(response, http.StatusBadRequest, "INVALID_CONFIG_REVISION", "Next revision must advance the typed configuration snapshot")
		return
	}
	if err != nil {
		var failure *contracts.ReconcileFailure
		if errors.As(err, &failure) {
			writeJSON(response, http.StatusUnprocessableEntity, result)
			return
		}
		writeError(response, http.StatusUnprocessableEntity, "ROTATE_DATABASE_PASSWORD_FAILED", "Database password rotation failed")
		return
	}
	writeJSON(response, http.StatusOK, result)
}

type passwordRotationRollbackBackend interface {
	RollbackDatabasePassword(context.Context, contracts.RotateDatabasePasswordRequest) error
}

func (s *server) rollbackDatabasePassword(response http.ResponseWriter, request *http.Request) {
	backend, ok := s.backend.(passwordRotationRollbackBackend)
	if !ok {
		writeError(response, http.StatusServiceUnavailable, "ROTATION_UNAVAILABLE", "Database password rollback is unavailable")
		return
	}
	var input contracts.RotateDatabasePasswordRequest
	if err := decodeJSON(response, request, &input); err != nil || input.OperationKind != "ROLLBACK_DATABASE_PASSWORD" || input.OperationID == "" || input.IdempotencyKey == "" || input.ProjectID == "" || input.Slug == "" || input.OldPassword == "" || input.NewPassword == "" {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A typed password rollback request is required")
		return
	}
	if err := backend.RollbackDatabasePassword(request.Context(), input); err != nil {
		if errors.Is(err, contracts.ErrStaleConfigRevision) {
			writeError(response, http.StatusConflict, "STALE_CONFIG_REVISION", "Project configuration revision is stale")
			return
		}
		writeJSON(response, http.StatusUnprocessableEntity, contracts.RotateDatabasePasswordResponse{OperationID: input.OperationID, ProjectID: input.ProjectID, Revision: input.ExpectedRevision, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rollback failed"}})
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

type passwordRotationConfirmationBackend interface {
	ConfirmDatabasePasswordRotation(context.Context, contracts.ConfirmDatabasePasswordRotationRequest) error
}

func (s *server) confirmDatabasePasswordRotation(response http.ResponseWriter, request *http.Request) {
	backend, ok := s.backend.(passwordRotationConfirmationBackend)
	if !ok {
		writeError(response, http.StatusServiceUnavailable, "ROTATION_UNAVAILABLE", "Database password rotation is unavailable")
		return
	}
	var input contracts.ConfirmDatabasePasswordRotationRequest
	if err := decodeJSON(response, request, &input); err != nil || input.OperationID == "" || input.IdempotencyKey == "" || input.ProjectID == "" || input.Slug == "" || input.NextRevision <= input.ExpectedRevision {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A typed rotation confirmation request is required")
		return
	}
	if err := backend.ConfirmDatabasePasswordRotation(request.Context(), input); err != nil {
		if errors.Is(err, contracts.ErrStaleConfigRevision) {
			writeError(response, http.StatusConflict, "STALE_CONFIG_REVISION", "Project configuration revision is stale")
			return
		}
		writeError(response, http.StatusUnprocessableEntity, "ROTATE_DATABASE_PASSWORD_FAILED", "Database password rotation confirmation failed")
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

var errStaleRevision = errors.New("stale config revision")

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}

func writeError(response http.ResponseWriter, status int, code, message string) {
	writeJSON(response, status, contracts.ErrorEnvelope{Error: contracts.APIError{Code: code, Message: message}})
}
