package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"supabase-manager/internal/contracts"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

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
		client = &http.Client{Timeout: 30 * time.Second}
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

func (c *Client) Reconcile(ctx context.Context, input contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error) {
	var output contracts.ReconcileProjectResponse
	if err := c.post(ctx, "/internal/v1/projects/reconcile", input, &output); err != nil {
		return contracts.ReconcileProjectResponse{}, err
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
		var envelope contracts.ErrorEnvelope
		_ = json.Unmarshal(payload, &envelope)
		rollbackComplete := false
		runtimeStateKnown := false
		runtimeStateChanged := false
		var rotation contracts.RotateDatabasePasswordResponse
		if json.Unmarshal(payload, &rotation) == nil {
			rollbackComplete = rotation.RolledBack
		}
		if envelope.Error.Code == "" {
			if rotation.Error != nil {
				envelope.Error = *rotation.Error
			} else {
				var reconcile contracts.ReconcileProjectResponse
				if json.Unmarshal(payload, &reconcile) == nil && reconcile.Error != nil {
					envelope.Error = *reconcile.Error
					rollbackComplete = reconcile.RolledBack
					runtimeStateKnown = true
					runtimeStateChanged = reconcile.RuntimeChanged
				}
			}
		}
		code, message := envelope.Error.Code, envelope.Error.Message
		allowed := map[string]string{"STALE_CONFIG_REVISION": "Project configuration revision is stale", "INVALID_CONFIG_REVISION": "Project configuration revision is invalid", "RECONCILE_FAILED": "Project runtime reconciliation failed", "ROTATE_DATABASE_PASSWORD_FAILED": "Database password rotation failed", "INVALID_REQUEST": "Provisioner request is invalid", "LIFECYCLE_FAILED": "Project lifecycle operation failed", "INSPECT_FAILED": "Project inspection failed"}
		local, ok := allowed[code]
		if !ok {
			code, local = "PROVISIONER_ERROR", "Provisioner request failed"
		}
		message = local
		return &ClientError{Code: code, Message: message, Status: response.StatusCode, RollbackComplete: rollbackComplete, RuntimeStateKnown: runtimeStateKnown, RuntimeStateChanged: runtimeStateChanged}
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
