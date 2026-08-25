package contracts

import "time"

type PrepareProjectRequest struct {
	OperationID      string `json:"operationId"`
	IdempotencyKey   string `json:"idempotencyKey"`
	ProjectID        string `json:"projectId"`
	Slug             string `json:"slug"`
	ExpectedRevision int64  `json:"expectedRevision"`
	NextRevision     int64  `json:"nextRevision"`
}

type PrepareProjectResponse struct {
	OperationID    string `json:"operationId"`
	IdempotencyKey string `json:"idempotencyKey"`
	ProjectID      string `json:"projectId"`
	Slug           string `json:"slug"`
	ProjectDir     string `json:"projectDir"`
	Revision       int64  `json:"revision"`
}

type LifecycleAction string

const (
	LifecycleStart         LifecycleAction = "START"
	LifecycleStop          LifecycleAction = "STOP"
	LifecycleRestart       LifecycleAction = "RESTART"
	LifecycleDeleteRuntime LifecycleAction = "DELETE_RUNTIME"
	LifecycleDeleteData    LifecycleAction = "DELETE_DATA"
)

type LifecycleRequest struct {
	OperationID    string          `json:"operationId"`
	IdempotencyKey string          `json:"idempotencyKey"`
	ProjectID      string          `json:"projectId"`
	Slug           string          `json:"slug"`
	Action         LifecycleAction `json:"action"`
}

type InspectProjectRequest struct {
	ProjectID string `json:"projectId"`
	Slug      string `json:"slug"`
}

type ServiceState struct {
	Name   string       `json:"name"`
	Health HealthStatus `json:"health"`
	Status string       `json:"status"`
}

type InspectProjectResponse struct {
	ProjectID string         `json:"projectId"`
	Health    HealthStatus   `json:"health"`
	Services  []ServiceState `json:"services"`
	CheckedAt time.Time      `json:"checkedAt"`
}

type ErrorEnvelope struct {
	Error APIError `json:"error"`
}

type APIError struct {
	Code          string            `json:"code"`
	Message       string            `json:"message"`
	CorrelationID string            `json:"correlationId,omitempty"`
	Fields        map[string]string `json:"fields,omitempty"`
}
