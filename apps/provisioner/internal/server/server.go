package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	provisionerauth "supabase-manager/apps/provisioner/internal/auth"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/redact"
	"supabase-manager/internal/contracts"
)

type Backend interface {
	Lifecycle(ctx context.Context, request contracts.LifecycleRequest) error
	Inspect(ctx context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error)
	Reconcile(ctx context.Context, request contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error)
}

type hostResourcesBackend interface {
	HostResources(context.Context) (contracts.HostResources, error)
}

type hostPortBackend interface {
	HostPortAvailable(context.Context, int) (bool, error)
}

type functionDeploymentBackend interface {
	DeployFunction(context.Context, contracts.DeployFunctionRequest) (contracts.FunctionDeploymentResult, error)
}

// certificateStager is deliberately separate from runtime reconciliation: it
// forwards short-lived PEM bytes to the host-owned Nginx agent and returns
// only safe filenames for the project configuration.
type certificateStager interface {
	StageCertificate(context.Context, contracts.StageManagedTLSRequest) (contracts.StageManagedTLSResponse, error)
}

type Options struct {
	ManagerToken      string
	ProjectFS         *projectfs.Root
	Backend           Backend
	CertificateStager certificateStager
	Logger            *slog.Logger
}

type server struct {
	projectFS    *projectfs.Root
	backend      Backend
	certificates certificateStager
	logger       *slog.Logger
}

func New(options Options) http.Handler {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	service := &server{projectFS: options.ProjectFS, backend: options.Backend, certificates: options.CertificateStager, logger: logger}
	private := http.NewServeMux()
	private.HandleFunc("POST /internal/v1/projects/lifecycle", service.lifecycle)
	private.HandleFunc("POST /internal/v1/projects/inspect", service.inspect)
	private.HandleFunc("POST /internal/v1/projects/reconcile", service.reconcile)
	private.HandleFunc("POST /internal/v1/nginx/certificates/stage", service.stageCertificate)
	private.HandleFunc("POST /internal/v1/projects/rotate-database-password", service.rotateDatabasePassword)
	private.HandleFunc("POST /internal/v1/projects/rollback-database-password", service.rollbackDatabasePassword)
	private.HandleFunc("POST /internal/v1/projects/confirm-database-password-rotation", service.confirmDatabasePasswordRotation)
	private.HandleFunc("POST /internal/v1/projects/{slug}/functions/{name}/deploy", service.deployFunction)
	private.HandleFunc("GET /internal/v1/host/resources", service.hostResources)
	private.HandleFunc("GET /internal/v1/host/ports/{port}", service.hostPortAvailable)
	root := http.NewServeMux()
	root.Handle("/internal/", provisionerauth.RequireManagerToken(options.ManagerToken, private))
	root.HandleFunc("GET /health/live", func(response http.ResponseWriter, _ *http.Request) { response.WriteHeader(http.StatusNoContent) })
	return root
}

func (s *server) deployFunction(response http.ResponseWriter, request *http.Request) {
	backend, ok := s.backend.(functionDeploymentBackend)
	if !ok {
		writeError(response, http.StatusServiceUnavailable, "FUNCTIONS_UNAVAILABLE", "Functions deployment is unavailable")
		return
	}
	name, operationID := request.PathValue("name"), request.Header.Get("X-Operation-ID")
	if err := contracts.ValidateFunctionName(name); err != nil || operationID == "" {
		writeError(response, http.StatusBadRequest, "INVALID_FUNCTION_DEPLOYMENT", "A valid function name and operation ID are required")
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 20<<20)
	result, err := backend.DeployFunction(request.Context(), contracts.DeployFunctionRequest{Slug: request.PathValue("slug"), Name: name, OperationID: operationID, Archive: request.Body})
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "FUNCTION_DEPLOY_FAILED", "Functions deployment failed")
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (s *server) stageCertificate(response http.ResponseWriter, request *http.Request) {
	if s.certificates == nil {
		writeError(response, http.StatusServiceUnavailable, "MANAGED_TLS_UNAVAILABLE", "Managed TLS is unavailable")
		return
	}
	var input contracts.StageManagedTLSRequest
	if err := decodeJSONLimit(response, request, &input, 2<<20); err != nil || input.CertificateName == "" || input.BaseDomain == "" || len(input.CertificatePEM) == 0 || len(input.PrivateKeyPEM) == 0 {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A certificate name, base domain, certificate, and private key are required")
		return
	}
	output, err := s.certificates.StageCertificate(request.Context(), input)
	if err != nil {
		writeError(response, http.StatusUnprocessableEntity, "TLS_STAGE_FAILED", "Unable to stage managed TLS certificate")
		return
	}
	writeJSON(response, http.StatusOK, output)
}

func (s *server) hostResources(response http.ResponseWriter, request *http.Request) {
	backend, ok := s.backend.(hostResourcesBackend)
	if !ok {
		writeError(response, http.StatusServiceUnavailable, "BACKEND_UNAVAILABLE", "Host resource inspection is unavailable")
		return
	}
	resources, err := backend.HostResources(request.Context())
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "HOST_RESOURCES_UNAVAILABLE", "Host resource inspection failed")
		return
	}
	writeJSON(response, http.StatusOK, resources)
}

func (s *server) hostPortAvailable(response http.ResponseWriter, request *http.Request) {
	backend, ok := s.backend.(hostPortBackend)
	if !ok {
		writeError(response, http.StatusServiceUnavailable, "BACKEND_UNAVAILABLE", "Host port inspection is unavailable")
		return
	}
	port, err := strconv.Atoi(request.PathValue("port"))
	if err != nil || port < 1 || port > 65535 {
		writeError(response, http.StatusBadRequest, "INVALID_PORT", "A valid TCP port is required")
		return
	}
	available, err := backend.HostPortAvailable(request.Context(), port)
	if err != nil {
		writeError(response, http.StatusServiceUnavailable, "HOST_PORT_UNAVAILABLE", "Host port inspection failed")
		return
	}
	writeJSON(response, http.StatusOK, contracts.HostPortAvailability{Port: port, Available: available})
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
		s.logger.Error("project lifecycle failed", "project_id", input.ProjectID, "slug", input.Slug, "action", input.Action, "error", redact.New(nil).String(err.Error()))
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
	if err := decodeJSON(response, request, &input); err != nil || input.OperationID == "" || input.ProjectID == "" || input.Slug == "" {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "A typed reconcile request is required")
		return
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = input.OperationID
	}
	s.logger.Info("project runtime reconciliation started", "project_id", input.ProjectID, "slug", input.Slug, "operation_id", input.OperationID)
	result, err := s.backend.Reconcile(request.Context(), input)
	if err != nil {
		s.logReconcileFailure(input, err)
		var failure *contracts.ReconcileFailure
		if errors.As(err, &failure) {
			result := failure.Response
			if result.OperationID == "" {
				result.OperationID = input.OperationID
			}
			if result.ProjectID == "" {
				result.ProjectID = input.ProjectID
			}
			if result.Error == nil {
				result.Error = &contracts.APIError{Code: "RECONCILE_FAILED", Message: redactedReconcileDiagnostic(input, failure.Cause)}
			}
			writeJSON(response, http.StatusUnprocessableEntity, result)
			return
		}
		// Runtime errors are deliberately generic: rendered environment files
		// and secret values must never cross this private API boundary.
		writeError(response, http.StatusUnprocessableEntity, "RECONCILE_FAILED", "Server runtime reconciliation failed")
		return
	}
	s.logger.Info("project runtime reconciliation completed", "project_id", input.ProjectID, "slug", input.Slug, "operation_id", input.OperationID, "revision", result.Revision, "recreated_services", result.RecreatedServices)
	writeJSON(response, http.StatusOK, result)
}

func (s *server) logReconcileFailure(input contracts.ReconcileProjectRequest, err error) {
	logErr := err
	var failure *contracts.ReconcileFailure
	if errors.As(err, &failure) && failure.Cause != nil {
		logErr = failure.Cause
	}
	s.logger.Error("project runtime reconciliation failed", "project_id", input.ProjectID, "slug", input.Slug, "operation_id", input.OperationID, "error", redactedReconcileDiagnostic(input, logErr))
}

func redactedReconcileDiagnostic(input contracts.ReconcileProjectRequest, cause error) string {
	if cause == nil {
		return "Server runtime reconciliation failed"
	}
	secrets := input.Secrets
	values := []string{
		secrets.DatabasePassword, secrets.JWTSecret, secrets.AnonKey, secrets.ServiceRoleKey,
		secrets.DashboardPassword, secrets.SecretKeyBase, secrets.VaultEncryptionKey,
		secrets.RealtimeDBEncryptionKey, secrets.LogflarePublicAccessToken,
		secrets.LogflarePrivateAccessToken, secrets.S3ProtocolAccessKeyID,
		secrets.S3ProtocolAccessKeySecret, secrets.PoolerTenantID,
	}
	for _, value := range input.RuntimeSecrets {
		values = append(values, value)
	}
	return redact.New(values).String(cause.Error())
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
		writeError(response, http.StatusConflict, "STALE_CONFIG_REVISION", "Server configuration revision is stale")
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
			writeError(response, http.StatusConflict, "STALE_CONFIG_REVISION", "Server configuration revision is stale")
			return
		}
		writeJSON(response, http.StatusUnprocessableEntity, contracts.RotateDatabasePasswordResponse{OperationID: input.OperationID, ProjectID: input.ProjectID, Revision: input.ExpectedRevision, RolledBack: false, RuntimeChanged: true, Error: &contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rollback failed"}})
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
			writeError(response, http.StatusConflict, "STALE_CONFIG_REVISION", "Server configuration revision is stale")
			return
		}
		writeError(response, http.StatusUnprocessableEntity, "ROTATE_DATABASE_PASSWORD_FAILED", "Database password rotation confirmation failed")
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

var errStaleRevision = errors.New("stale config revision")

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) error {
	return decodeJSONLimit(response, request, target, 1<<20)
}

func decodeJSONLimit(response http.ResponseWriter, request *http.Request, target any, limit int64) error {
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, limit))
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
