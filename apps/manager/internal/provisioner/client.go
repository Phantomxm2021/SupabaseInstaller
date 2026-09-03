package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"supabase-manager/internal/contracts"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type FunctionsClient interface {
	DeployFunction(context.Context, string, string, string, io.Reader) (contracts.FunctionDeploymentResult, error)
	ListFunctions(context.Context, string) ([]contracts.FunctionSummary, error)
	FunctionLogs(context.Context, string, string, contracts.FunctionLogQuery) (contracts.FunctionLogPage, error)
	RollbackFunction(context.Context, string, string, string) (contracts.FunctionDeploymentResult, error)
	DeleteFunction(context.Context, string, string, string) (contracts.FunctionDeploymentResult, error)
}

const DefaultRequestTimeout = 45 * time.Minute

// ClientError is a redacted error returned by the private Provisioner API.
// Only the typed code and user-safe message are retained; response bodies are
// never embedded in the error so rendered secrets cannot escape this boundary.
type ClientError struct {
	Code                string
	Message             string
	Status              int
	RollbackComplete    bool
	RuntimeStateKnown   bool
	RuntimeStateChanged bool
}

type expectedFailureIdentity struct {
	operationID string
	projectID   string
}

func (e *ClientError) Error() string             { return fmt.Sprintf("provisioner %s: %s", e.Code, e.Message) }
func (e *ClientError) RollbackSucceeded() bool   { return e.RollbackComplete }
func (e *ClientError) RuntimeOutcomeKnown() bool { return e.RuntimeStateKnown }
func (e *ClientError) RuntimeChanged() bool      { return e.RuntimeStateChanged }

func (e *ClientError) Unwrap() error {
	switch e.Code {
	case "STALE_CONFIG_REVISION":
		return contracts.ErrStaleConfigRevision
	case "INVALID_CONFIG_REVISION":
		return contracts.ErrInvalidReconcileRevision
	default:
		return nil
	}
}

func NewClient(baseURL, token string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: DefaultRequestTimeout}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: client}
}

func (c *Client) Lifecycle(ctx context.Context, input contracts.LifecycleRequest) error {
	return c.post(ctx, "/internal/v1/projects/lifecycle", input, nil)
}

func (c *Client) Inspect(ctx context.Context, input contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error) {
	var output contracts.InspectProjectResponse
	if err := c.post(ctx, "/internal/v1/projects/inspect", input, &output); err != nil {
		return contracts.InspectProjectResponse{}, err
	}
	return output, nil
}

func (c *Client) HostResources(ctx context.Context) (contracts.HostResources, error) {
	var output contracts.HostResources
	if err := c.get(ctx, "/internal/v1/host/resources", &output); err != nil {
		return contracts.HostResources{}, err
	}
	return output, nil
}

func (c *Client) HostPortAvailable(ctx context.Context, port int) (bool, error) {
	var output contracts.HostPortAvailability
	if err := c.get(ctx, fmt.Sprintf("/internal/v1/host/ports/%d", port), &output); err != nil {
		return false, err
	}
	return output.Available, nil
}

func (c *Client) AvailableContext(ctx context.Context, port int) (bool, error) {
	return c.HostPortAvailable(ctx, port)
}

func (c *Client) Reconcile(ctx context.Context, input contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	var output contracts.ReconcileProjectResponse
	if err := c.post(ctx, "/internal/v1/projects/reconcile", input, &output); err != nil {
		return contracts.ReconcileProjectResponse{}, err
	}
	return output, nil
}

// DeployFunction streams a previously admitted archive to the Provisioner's
// single typed Functions endpoint. The caller supplies no filesystem path or
// Docker command.
func (c *Client) DeployFunction(ctx context.Context, slug, name, operationID string, archive io.Reader) (contracts.FunctionDeploymentResult, error) {
	if err := contracts.ValidateFunctionName(name); err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	if slug == "" || operationID == "" || archive == nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("function deployment identity and archive are required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/v1/projects/"+slug+"/functions/"+name+"/deploy", archive)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("create function deployment request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/zip")
	request.Header.Set("X-Operation-ID", operationID)
	response, err := c.http.Do(request)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("deploy function: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return contracts.FunctionDeploymentResult{}, clientErrorForPayload(request.URL.Path, response.StatusCode, payload)
	}
	var result contracts.FunctionDeploymentResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("decode function deployment response: %w", err)
	}
	return result, nil
}

func (c *Client) ListFunctions(ctx context.Context, slug string) ([]contracts.FunctionSummary, error) {
	var output struct {
		Functions []contracts.FunctionSummary `json:"functions"`
	}
	if err := c.get(ctx, "/internal/v1/projects/"+slug+"/functions", &output); err != nil {
		return nil, err
	}
	return output.Functions, nil
}

func (c *Client) FunctionLogs(ctx context.Context, slug, name string, query contracts.FunctionLogQuery) (contracts.FunctionLogPage, error) {
	if slug == "" {
		return contracts.FunctionLogPage{}, errors.New("project slug is required")
	}
	if err := contracts.ValidateFunctionName(name); err != nil {
		return contracts.FunctionLogPage{}, err
	}
	if err := contracts.ValidateFunctionLogQuery(query); err != nil {
		return contracts.FunctionLogPage{}, err
	}
	values := url.Values{}
	values.Set("limit", strconv.Itoa(query.Limit))
	if query.Before != "" {
		values.Set("before", query.Before)
	}
	if query.After != "" {
		values.Set("after", query.After)
	}
	if query.Level != "" {
		values.Set("level", query.Level)
	}
	if query.Search != "" {
		values.Set("search", query.Search)
	}
	endpoint := c.baseURL + "/internal/v1/projects/" + url.PathEscape(slug) + "/functions/" + url.PathEscape(name) + "/logs?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return contracts.FunctionLogPage{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	response, err := c.http.Do(request)
	if err != nil {
		return contracts.FunctionLogPage{}, fmt.Errorf("query function logs: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return contracts.FunctionLogPage{}, errors.New("read function logs response")
	}
	if len(payload) > 1<<20 {
		return contracts.FunctionLogPage{}, errors.New("function logs response is too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return contracts.FunctionLogPage{}, clientErrorForPayload(request.URL.Path, response.StatusCode, payload)
	}
	if err := rejectDuplicateJSONFields(payload); err != nil {
		return contracts.FunctionLogPage{}, errors.New("decode function logs response")
	}
	var page contracts.FunctionLogPage
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&page); err != nil {
		return contracts.FunctionLogPage{}, errors.New("decode function logs response")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return contracts.FunctionLogPage{}, errors.New("decode function logs response")
	}
	return page, nil
}

func rejectDuplicateJSONFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("invalid JSON field")
				}
				if _, exists := seen[key]; exists {
					return errors.New("duplicate JSON field")
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return errors.New("invalid JSON")
		}
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func (c *Client) RollbackFunction(ctx context.Context, slug, name, operationID string) (contracts.FunctionDeploymentResult, error) {
	return c.functionAction(ctx, http.MethodPost, slug, name, operationID, "")
}

func (c *Client) DeleteFunction(ctx context.Context, slug, name, operationID string) (contracts.FunctionDeploymentResult, error) {
	return c.functionAction(ctx, http.MethodDelete, slug, name, operationID, name)
}

func (c *Client) functionAction(ctx context.Context, method, slug, name, operationID, confirmation string) (contracts.FunctionDeploymentResult, error) {
	if err := contracts.ValidateFunctionName(name); err != nil || slug == "" || operationID == "" {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("function action identity is required")
	}
	endpoint := c.baseURL + "/internal/v1/projects/" + slug + "/functions/" + name + "/rollback"
	if method == http.MethodDelete {
		endpoint = c.baseURL + "/internal/v1/projects/" + slug + "/functions/" + name
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("X-Operation-ID", operationID)
	if confirmation != "" {
		request.Header.Set("X-Confirm-Function", confirmation)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("function action: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return contracts.FunctionDeploymentResult{}, clientErrorForPayload(request.URL.Path, response.StatusCode, payload)
	}
	var output contracts.FunctionDeploymentResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&output); err != nil {
		return contracts.FunctionDeploymentResult{}, fmt.Errorf("decode function action response: %w", err)
	}
	return output, nil
}

func (c *Client) StageManagedTLS(ctx context.Context, input contracts.StageManagedTLSRequest) (contracts.StageManagedTLSResponse, error) {
	var output contracts.StageManagedTLSResponse
	if err := c.post(ctx, "/internal/v1/nginx/certificates/stage", input, &output); err != nil {
		return contracts.StageManagedTLSResponse{}, err
	}
	return output, nil
}

func (c *Client) RotateDatabasePassword(ctx context.Context, input contracts.RotateDatabasePasswordRequest) (contracts.RotateDatabasePasswordResponse, error) {
	var output contracts.RotateDatabasePasswordResponse
	if err := c.post(ctx, "/internal/v1/projects/rotate-database-password", input, &output); err != nil {
		return contracts.RotateDatabasePasswordResponse{}, err
	}
	return output, nil
}

// ReconcileAuthKeys uses a private envelope because ProjectSecrets deliberately
// omits candidate auth-key fields from all ordinary JSON contracts.
func (c *Client) ReconcileAuthKeys(ctx context.Context, input contracts.AuthKeysReconcileRequest) (contracts.ReconcileProjectResponse, error) {
	var output contracts.ReconcileProjectResponse
	if err := c.post(ctx, "/internal/v1/projects/reconcile-auth-keys", input, &output); err != nil {
		return contracts.ReconcileProjectResponse{}, err
	}
	return output, nil
}

func (c *Client) RollbackDatabasePassword(ctx context.Context, input contracts.RotateDatabasePasswordRequest) error {
	return c.post(ctx, "/internal/v1/projects/rollback-database-password", input, nil)
}

func (c *Client) ConfirmDatabasePasswordRotation(ctx context.Context, input contracts.ConfirmDatabasePasswordRotationRequest) error {
	return c.post(ctx, "/internal/v1/projects/confirm-database-password-rotation", input, nil)
}

func (c *Client) post(ctx context.Context, path string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode provisioner request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create provisioner request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call provisioner: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return clientErrorForPayload(path, response.StatusCode, payload, expectedFailureIdentityFor(input))
	}
	if output != nil && response.StatusCode != http.StatusNoContent {
		decoder := json.NewDecoder(response.Body)
		if err := decoder.Decode(output); err != nil {
			return fmt.Errorf("decode provisioner response: %w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if err == nil {
				return fmt.Errorf("decode provisioner response: multiple JSON values")
			}
			return fmt.Errorf("decode provisioner response: %w", err)
		}
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create provisioner request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("call provisioner: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		return clientErrorForPayload(path, response.StatusCode, payload)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("decode provisioner response: %w", err)
	}
	return nil
}

// clientErrorForPayload accepts a diagnostic only after both the endpoint and
// the Provisioner code identify a known operational failure. Error.Message is
// canonical protocol text, not a diagnostic, and is therefore never persisted
// as one. Reconcile and rotation diagnostics additionally require the explicit
// redaction contract version when they arrive in their typed response.
func clientErrorForPayload(path string, status int, payload []byte, expected ...expectedFailureIdentity) *ClientError {
	expectedIdentity := expectedFailureIdentity{}
	if len(expected) > 0 {
		expectedIdentity = expected[0]
	}
	var envelope contracts.ErrorEnvelope
	envelopeOK := json.Unmarshal(payload, &envelope) == nil
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(payload, &fields)

	code := envelope.Error.Code
	diagnostic := ""
	typedResponse := false
	rollbackComplete, runtimeStateKnown, runtimeStateChanged := false, false, false

	if path == "/internal/v1/projects/reconcile" || path == "/internal/v1/projects/reconcile-auth-keys" {
		var result contracts.ReconcileProjectResponse
		if json.Unmarshal(payload, &result) == nil && result.Error != nil {
			typedResponse = true
			code = result.Error.Code
			if hasCompleteTypedFailureIdentity(fields, expectedIdentity, result.OperationID, result.ProjectID, result.Error, result.DiagnosticVersion) {
				rollbackComplete, runtimeStateKnown, runtimeStateChanged = result.RolledBack, !result.RuntimeOutcomeUnknown, result.RuntimeChanged
				diagnostic = result.Diagnostic
			}
		}
	} else if isRotationPath(path) {
		var result contracts.RotateDatabasePasswordResponse
		if json.Unmarshal(payload, &result) == nil && result.Error != nil {
			typedResponse = true
			code = result.Error.Code
			if hasCompleteTypedFailureIdentity(fields, expectedIdentity, result.OperationID, result.ProjectID, result.Error, result.DiagnosticVersion) {
				rollbackComplete, runtimeStateKnown, runtimeStateChanged = result.RolledBack, !result.RuntimeOutcomeUnknown, result.RuntimeChanged
				diagnostic = result.Diagnostic
			}
		}
	} else if functionRouteForPath(path) != "" {
		var result contracts.FunctionDeploymentResult
		if _, ok := fields["rolledBack"]; ok && json.Unmarshal(payload, &result) == nil && result.Error != nil {
			typedResponse = true
			code = result.Error.Code
			rollbackComplete = result.RolledBack
			if rollbackComplete {
				runtimeStateKnown = true
			}
			diagnostic = result.Diagnostic
		}
	}
	if !typedResponse && envelopeOK && path != "/internal/v1/projects/reconcile" && path != "/internal/v1/projects/reconcile-auth-keys" && !isRotationPath(path) {
		diagnostic = envelope.Diagnostic
	}

	canonical, acceptsDiagnostic := canonicalProvisionerError(path, code)
	if canonical == "" {
		code, canonical = "PROVISIONER_ERROR", "Provisioner request failed"
		acceptsDiagnostic = false
	}
	message := canonical
	if acceptsDiagnostic && strings.TrimSpace(diagnostic) != "" {
		message = diagnostic
	}
	return &ClientError{Code: code, Message: message, Status: status, RollbackComplete: rollbackComplete, RuntimeStateKnown: runtimeStateKnown, RuntimeStateChanged: runtimeStateChanged}
}

func expectedFailureIdentityFor(input any) expectedFailureIdentity {
	switch value := input.(type) {
	case contracts.ReconcileProjectRequest:
		return expectedFailureIdentity{operationID: value.OperationID, projectID: value.ProjectID}
	case contracts.AuthKeysReconcileRequest:
		return expectedFailureIdentity{operationID: value.Request.OperationID, projectID: value.Request.ProjectID}
	case contracts.RotateDatabasePasswordRequest:
		return expectedFailureIdentity{operationID: value.OperationID, projectID: value.ProjectID}
	case contracts.ConfirmDatabasePasswordRotationRequest:
		return expectedFailureIdentity{operationID: value.OperationID, projectID: value.ProjectID}
	default:
		return expectedFailureIdentity{}
	}
}

func hasCompleteTypedFailureIdentity(fields map[string]json.RawMessage, expected expectedFailureIdentity, operationID, projectID string, apiError *contracts.APIError, diagnosticVersion int) bool {
	return expected.operationID != "" && expected.projectID != "" && operationID == expected.operationID && projectID == expected.projectID && apiError != nil && apiError.Code != "" && contracts.SupportsDiagnosticVersion(diagnosticVersion) && isJSONBoolean(fields["rolledBack"]) && hasValidOptionalRuntimeChanged(fields) && hasValidOptionalRuntimeOutcomeUnknown(fields)
}

func hasValidOptionalRuntimeChanged(fields map[string]json.RawMessage) bool {
	raw, exists := fields["runtimeChanged"]
	return !exists || isJSONBoolean(raw)
}

func hasValidOptionalRuntimeOutcomeUnknown(fields map[string]json.RawMessage) bool {
	raw, exists := fields["runtimeOutcomeUnknown"]
	return !exists || isJSONBoolean(raw)
}

func isJSONBoolean(raw json.RawMessage) bool {
	value := strings.TrimSpace(string(raw))
	return value == "true" || value == "false"
}

func isRotationPath(path string) bool {
	return path == "/internal/v1/projects/rotate-database-password" || path == "/internal/v1/projects/rollback-database-password" || path == "/internal/v1/projects/confirm-database-password-rotation"
}

func functionRouteForPath(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) < 7 || parts[0] != "" || parts[1] != "internal" || parts[2] != "v1" || parts[3] != "projects" || parts[4] == "" || parts[5] != "functions" || parts[6] == "" {
		return ""
	}
	if len(parts) == 7 {
		return "delete"
	}
	if len(parts) == 8 && (parts[7] == "deploy" || parts[7] == "rollback") {
		return parts[7]
	}
	return ""
}

func isHostPortPath(path string) bool {
	parts := strings.Split(path, "/")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "internal" || parts[2] != "v1" || parts[3] != "host" || parts[4] != "ports" {
		return false
	}
	port, err := strconv.Atoi(parts[5])
	return err == nil && port >= 1 && port <= 65535
}

func canonicalProvisionerError(path, code string) (canonical string, acceptsDiagnostic bool) {
	if code == "INVALID_REQUEST" {
		return "Provisioner request is invalid", false
	}
	switch {
	case path == "/internal/v1/projects/reconcile":
		switch code {
		case "STALE_CONFIG_REVISION":
			return "Server configuration revision is stale", false
		case "INVALID_CONFIG_REVISION":
			return "Server configuration revision is invalid", false
		case "RECONCILE_FAILED":
			return "Server runtime reconciliation failed", true
		}
	case isRotationPath(path):
		switch code {
		case "STALE_CONFIG_REVISION":
			return "Server configuration revision is stale", false
		case "INVALID_CONFIG_REVISION":
			return "Server configuration revision is invalid", false
		case "ROTATE_DATABASE_PASSWORD_FAILED":
			return "Database password rotation failed", true
		}
	case path == "/internal/v1/projects/lifecycle":
		if code == "LIFECYCLE_FAILED" {
			return "Server lifecycle operation failed", true
		}
	case path == "/internal/v1/projects/inspect":
		if code == "INSPECT_FAILED" {
			return "Server inspection failed", true
		}
	case path == "/internal/v1/nginx/certificates/stage":
		if code == "TLS_STAGE_FAILED" {
			return "Unable to stage managed TLS certificate", true
		}
	case functionRouteForPath(path) == "deploy":
		if code == "FUNCTION_DEPLOY_FAILED" {
			return "Function deployment failed", true
		}
	case functionRouteForPath(path) == "rollback":
		if code == "FUNCTION_ROLLBACK_FAILED" {
			return "Function rollback failed", true
		}
	case functionRouteForPath(path) == "delete":
		if code == "FUNCTION_DELETE_FAILED" {
			return "Function deletion failed", true
		}
	case path == "/internal/v1/host/resources":
		if code == "HOST_RESOURCES_UNAVAILABLE" {
			return "Host resource inspection failed", true
		}
	case isHostPortPath(path):
		if code == "HOST_PORT_UNAVAILABLE" {
			return "Host port inspection failed", true
		}
	}
	return "", false
}
