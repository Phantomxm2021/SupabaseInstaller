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
	Code             string
	Message          string
	Status           int
	RollbackComplete bool
}

func (e *ClientError) Error() string           { return fmt.Sprintf("provisioner %s: %s", e.Code, e.Message) }
func (e *ClientError) RollbackSucceeded() bool { return e.RollbackComplete }

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

func (c *Client) Prepare(ctx context.Context, input contracts.PrepareProjectRequest) (contracts.PrepareProjectResponse, error) {
	var output contracts.PrepareProjectResponse
	if err := c.post(ctx, "/internal/v1/projects/prepare", input, &output); err != nil {
		return contracts.PrepareProjectResponse{}, err
	}
	return output, nil
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
		if envelope.Error.Code == "" {
			var reconcile contracts.ReconcileProjectResponse
			if json.Unmarshal(payload, &reconcile) == nil && reconcile.Error != nil {
				envelope.Error = *reconcile.Error
				return &ClientError{Code: envelope.Error.Code, Message: envelope.Error.Message, Status: response.StatusCode, RollbackComplete: reconcile.RolledBack}
			}
		}
		code, message := envelope.Error.Code, envelope.Error.Message
		if code == "" {
			code = "PROVISIONER_ERROR"
		}
		if message == "" {
			message = "Provisioner request failed"
		}
		return &ClientError{Code: code, Message: message, Status: response.StatusCode}
	}
	if output != nil && response.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(response.Body).Decode(output); err != nil {
			return fmt.Errorf("decode provisioner response: %w", err)
		}
	}
	return nil
}
