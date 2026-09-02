package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"

	provisionerauth "supabase-manager/apps/provisioner/internal/auth"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
	"supabase-manager/internal/diagnostic"
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

type functionListBackend interface {
	ListFunctions(context.Context, contracts.FunctionOperationRequest) ([]contracts.FunctionSummary, error)
}
type functionRollbackBackend interface {
	RollbackFunction(context.Context, contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error)
}
type functionDeleteBackend interface {
	DeleteFunction(context.Context, contracts.FunctionOperationRequest) (contracts.FunctionDeploymentResult, error)
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
	private.HandleFunc("GET /internal/v1/projects/{slug}/functions", service.listFunctions)
	private.HandleFunc("POST /internal/v1/projects/{slug}/functions/{name}/rollback", service.rollbackFunction)
	private.HandleFunc("DELETE /internal/v1/projects/{slug}/functions/{name}", service.deleteFunction)
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
	mediaType, _, _ := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if mediaType != "application/zip" && mediaType != "application/octet-stream" {
		writeError(response, http.StatusBadRequest, "INVALID_FUNCTION_ARCHIVE", "Function archive must be a ZIP")
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, 20<<20)
	result, err := backend.DeployFunction(request.Context(), contracts.DeployFunctionRequest{Slug: request.PathValue("slug"), Name: name, OperationID: operationID, Archive: request.Body})
	if err != nil {
		failure := deployFunctionFailureEnvelope(err)
		s.logger.Error("function deployment failed", "slug", request.PathValue("slug"), "function", name, "operation_id", operationID, "error", failure.Diagnostic)
		if result.RolledBack {
			result.Error = &failure.Error
			result.Diagnostic = failure.Diagnostic
			writeJSON(response, http.StatusUnprocessableEntity, result)
			return
		}
		writeJSON(response, http.StatusUnprocessableEntity, failure)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (s *server) listFunctions(response http.ResponseWriter, request *http.Request) {
	backend, ok := s.backend.(functionListBackend)
	if !ok {
		writeError(response, http.StatusServiceUnavailable, "FUNCTIONS_UNAVAILABLE", "Functions are unavailable")
		return
	}
	items, err := backend.ListFunctions(request.Context(), contracts.FunctionOperationRequest{Slug: request.PathValue("slug")})
	if err != nil {
		failure := operationalErrorEnvelope("FUNCTIONS_LIST_FAILED", "Unable to list functions", err, nil)
		s.logger.Error("functions listing failed", "slug", request.PathValue("slug"), "error", failure.Diagnostic)
		writeJSON(response, http.StatusUnprocessableEntity, failure)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"functions": items})
}

func (s *server) rollbackFunction(response http.ResponseWriter, request *http.Request) {
	backend, ok := s.backend.(functionRollbackBackend)
	if !ok {
		writeError(response, http.StatusServiceUnavailable, "FUNCTIONS_UNAVAILABLE", "Functions are unavailable")
		return
	}
	name, operationID := request.PathValue("name"), request.Header.Get("X-Operation-ID")
	if err := contracts.ValidateFunctionName(name); err != nil || operationID == "" {
		writeError(response, http.StatusBadRequest, "INVALID_FUNCTION_ROLLBACK", "A valid function name and operation ID are required")
		return
	}
	result, err := backend.RollbackFunction(request.Context(), contracts.FunctionOperationRequest{Slug: request.PathValue("slug"), Name: name, OperationID: operationID})
	if err != nil {
		failure := operationalErrorEnvelope("FUNCTION_ROLLBACK_FAILED", "Function rollback failed", err, nil)
		s.logger.Error("function rollback failed", "slug", request.PathValue("slug"), "function", name, "operation_id", operationID, "error", failure.Diagnostic)
		result.Error = &failure.Error
		result.Diagnostic = failure.Diagnostic
		writeJSON(response, http.StatusUnprocessableEntity, result)
		return
	}
	writeJSON(response, http.StatusOK, result)
}

func (s *server) deleteFunction(response http.ResponseWriter, request *http.Request) {
	backend, ok := s.backend.(functionDeleteBackend)
	if !ok {
		writeError(response, http.StatusServiceUnavailable, "FUNCTIONS_UNAVAILABLE", "Functions are unavailable")
		return
	}
	name, operationID := request.PathValue("name"), request.Header.Get("X-Operation-ID")
	if err := contracts.ValidateFunctionName(name); err != nil || operationID == "" || request.Header.Get("X-Confirm-Function") != name {
		writeError(response, http.StatusBadRequest, "INVALID_FUNCTION_DELETE", "Function name, operation ID, and exact confirmation are required")
		return
	}
	result, err := backend.DeleteFunction(request.Context(), contracts.FunctionOperationRequest{Slug: request.PathValue("slug"), Name: name, OperationID: operationID})
	if err != nil {
		failure := operationalErrorEnvelope("FUNCTION_DELETE_FAILED", "Function deletion failed", err, nil)
		s.logger.Error("function deletion failed", "slug", request.PathValue("slug"), "function", name, "operation_id", operationID, "error", failure.Diagnostic)
		result.Error = &failure.Error
		result.Diagnostic = failure.Diagnostic
		writeJSON(response, http.StatusUnprocessableEntity, result)
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
		failure := operationalErrorEnvelope("TLS_STAGE_FAILED", "Unable to stage managed TLS certificate", err, []string{string(input.CertificatePEM), string(input.PrivateKeyPEM)})
		s.logger.Error("managed TLS certificate staging failed", "certificate_name", input.CertificateName, "base_domain", input.BaseDomain, "error", failure.Diagnostic)
		writeJSON(response, http.StatusUnprocessableEntity, failure)
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
		failure := operationalErrorEnvelope("HOST_RESOURCES_UNAVAILABLE", "Host resource inspection failed", err, nil)
		s.logger.Error("host resource inspection failed", "error", failure.Diagnostic)
		writeJSON(response, http.StatusServiceUnavailable, failure)
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
		failure := operationalErrorEnvelope("HOST_PORT_UNAVAILABLE", "Host port inspection failed", err, nil)
		s.logger.Error("host port inspection failed", "port", port, "error", failure.Diagnostic)
		writeJSON(response, http.StatusServiceUnavailable, failure)
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
		failure := operationalErrorEnvelope("LIFECYCLE_FAILED", "Project lifecycle action failed", err, nil)
		s.logger.Error("project lifecycle failed", "project_id", input.ProjectID, "slug", input.Slug, "action", input.Action, "error", failure.Diagnostic)
		writeJSON(response, http.StatusUnprocessableEntity, failure)
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
		failure := operationalErrorEnvelope("INSPECT_FAILED", "Project inspection failed", err, nil)
		s.logger.Error("project inspection failed", "project_id", input.ProjectID, "slug", input.Slug, "error", failure.Diagnostic)
		writeJSON(response, http.StatusUnprocessableEntity, failure)
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
		var failure *contracts.ReconcileFailure
		if errors.As(err, &failure) {
			result := failure.Response
			if result.OperationID == "" {
				result.OperationID = input.OperationID
			}
			if result.ProjectID == "" {
				result.ProjectID = input.ProjectID
			}
			canonical := contracts.APIError{Code: "RECONCILE_FAILED", Message: "Server runtime reconciliation failed"}
			result.Error = &canonical
			if failure.Cause == nil && !contracts.SupportsDiagnosticVersion(result.DiagnosticVersion) {
				result.Diagnostic = canonical.Message
			} else if result.Diagnostic == "" {
				result.Diagnostic = operationalErrorEnvelope(canonical.Code, canonical.Message, failure.Cause, reconcileKnownValues(input)).Diagnostic
			} else {
				result.Diagnostic = diagnostic.Sanitize(result.Diagnostic, reconcileKnownValues(input))
			}
			s.logReconcileFailure(input, result.Diagnostic)
			writeJSON(response, http.StatusUnprocessableEntity, result)
			return
		}
		failureEnvelope := operationalErrorEnvelope("RECONCILE_FAILED", "Server runtime reconciliation failed", err, reconcileKnownValues(input))
		s.logReconcileFailure(input, failureEnvelope.Diagnostic)
		writeJSON(response, http.StatusUnprocessableEntity, failureEnvelope)
		return
	}
	s.logger.Info("project runtime reconciliation completed", "project_id", input.ProjectID, "slug", input.Slug, "operation_id", input.OperationID, "revision", result.Revision, "recreated_services", result.RecreatedServices)
	writeJSON(response, http.StatusOK, result)
}

func (s *server) logReconcileFailure(input contracts.ReconcileProjectRequest, detail string) {
	s.logger.Error("project runtime reconciliation failed", "project_id", input.ProjectID, "slug", input.Slug, "operation_id", input.OperationID, "error", detail)
}

func reconcileKnownValues(input contracts.ReconcileProjectRequest) []string {
	secrets := input.Secrets
	values := []string{
		secrets.DatabasePassword, secrets.JWTSecret, secrets.AnonKey, secrets.ServiceRoleKey,
		secrets.DashboardPassword, secrets.SecretKeyBase, secrets.VaultEncryptionKey,
		secrets.RealtimeDBEncryptionKey, secrets.LogflarePublicAccessToken,
		secrets.LogflarePrivateAccessToken, secrets.S3ProtocolAccessKeyID,
		secrets.S3ProtocolAccessKeySecret, secrets.PoolerTenantID,
		secrets.SupabasePublishableKey, secrets.SupabaseSecretKey, secrets.AnonKeyAsymmetric,
		secrets.ServiceRoleKeyAsymmetric, secrets.JWTKeys, secrets.JWTJWKS,
	}
	for _, value := range input.RuntimeSecrets {
		values = append(values, value)
	}
	return append(values, diagnostic.ConfigurationSecretValues(input.Configuration)...)
}

func rotationKnownValues(input contracts.RotateDatabasePasswordRequest) []string {
	values := reconcileKnownValues(contracts.ReconcileProjectRequest{Configuration: input.Configuration, Secrets: input.Secrets, RuntimeSecrets: input.RuntimeSecrets})
	return append(values, input.OldPassword, input.NewPassword)
}

type archiveIngestionFailure interface {
	ArchiveIngestionFailure()
}

type archivePathFilesystemFailure interface {
	ArchivePathFilesystemFailure()
}

func deployFunctionFailureEnvelope(cause error) contracts.ErrorEnvelope {
	var archiveFailure archiveIngestionFailure
	if errors.As(cause, &archiveFailure) {
		return operationalErrorEnvelope("FUNCTION_DEPLOY_FAILED", "Function deployment failed", errors.New("Function archive processing failed"), nil)
	}
	var filesystemFailure archivePathFilesystemFailure
	if errors.As(cause, &filesystemFailure) {
		return operationalErrorEnvelope("FUNCTION_DEPLOY_FAILED", "Function deployment failed", cause, nil)
	}
	return operationalErrorEnvelope("FUNCTION_DEPLOY_FAILED", "Function deployment failed", cause, nil)
}

func operationalErrorEnvelope(code, message string, cause error, knownValues []string) contracts.ErrorEnvelope {
	detail := message
	if cause != nil {
		if sanitized := diagnostic.Sanitize(cause.Error(), knownValues); sanitized != "" {
			detail = sanitized
		}
	}
	return contracts.ErrorEnvelope{Error: contracts.APIError{Code: code, Message: message}, Diagnostic: detail}
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
			canonical := contracts.APIError{Code: "ROTATE_DATABASE_PASSWORD_FAILED", Message: "Database password rotation failed"}
			result.Error = &canonical
			if failure.Cause == nil && !contracts.SupportsDiagnosticVersion(result.DiagnosticVersion) {
				result.Diagnostic = canonical.Message
			} else if result.Diagnostic == "" {
				cause := err
				if failure.Cause != nil {
					cause = failure.Cause
				}
				result.Diagnostic = operationalErrorEnvelope(canonical.Code, canonical.Message, cause, rotationKnownValues(input)).Diagnostic
			} else {
				result.Diagnostic = diagnostic.Sanitize(result.Diagnostic, rotationKnownValues(input))
			}
			s.logger.Error("database password rotation failed", "project_id", input.ProjectID, "slug", input.Slug, "operation_id", input.OperationID, "error", result.Diagnostic)
			writeJSON(response, http.StatusUnprocessableEntity, result)
			return
		}
		failureEnvelope := operationalErrorEnvelope("ROTATE_DATABASE_PASSWORD_FAILED", "Database password rotation failed", err, rotationKnownValues(input))
		s.logger.Error("database password rotation failed", "project_id", input.ProjectID, "slug", input.Slug, "operation_id", input.OperationID, "error", failureEnvelope.Diagnostic)
		writeJSON(response, http.StatusUnprocessableEntity, failureEnvelope)
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
		failure := operationalErrorEnvelope("ROTATE_DATABASE_PASSWORD_FAILED", "Database password rollback failed", err, rotationKnownValues(input))
		s.logger.Error("database password rollback failed", "project_id", input.ProjectID, "slug", input.Slug, "operation_id", input.OperationID, "error", failure.Diagnostic)
		writeJSON(response, http.StatusUnprocessableEntity, contracts.RotateDatabasePasswordResponse{OperationID: input.OperationID, ProjectID: input.ProjectID, Revision: input.ExpectedRevision, RolledBack: false, RuntimeChanged: true, Error: &failure.Error, Diagnostic: failure.Diagnostic})
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
		failure := operationalErrorEnvelope("ROTATE_DATABASE_PASSWORD_FAILED", "Database password rotation confirmation failed", err, nil)
		s.logger.Error("database password rotation confirmation failed", "project_id", input.ProjectID, "slug", input.Slug, "operation_id", input.OperationID, "error", failure.Diagnostic)
		writeJSON(response, http.StatusUnprocessableEntity, failure)
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
