package contracts

import "time"

type PrepareProjectRequest struct {
	OperationID      string         `json:"operationId"`
	IdempotencyKey   string         `json:"idempotencyKey"`
	ProjectID        string         `json:"projectId"`
	ProjectName      string         `json:"projectName"`
	Slug             string         `json:"slug"`
	ExpectedRevision int64          `json:"expectedRevision"`
	NextRevision     int64          `json:"nextRevision"`
	Domain           string         `json:"domain"`
	SiteURL          string         `json:"siteUrl"`
	APIPort          int            `json:"apiPort"`
	Secrets          ProjectSecrets `json:"secrets"`
}

type ProjectSecrets struct {
	DatabasePassword   string `json:"databasePassword"`
	JWTSecret          string `json:"jwtSecret"`
	AnonKey            string `json:"anonKey"`
	ServiceRoleKey     string `json:"serviceRoleKey"`
	DashboardPassword  string `json:"dashboardPassword"`
	SecretKeyBase      string `json:"secretKeyBase"`
	VaultEncryptionKey string `json:"vaultEncryptionKey"`
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
	OperationID        string          `json:"operationId"`
	IdempotencyKey     string          `json:"idempotencyKey"`
	ProjectID          string          `json:"projectId"`
	Slug               string          `json:"slug"`
	Action             LifecycleAction `json:"action"`
	ConfirmProjectName string          `json:"confirmProjectName,omitempty"`
}

type InspectProjectRequest struct {
	ProjectID       string   `json:"projectId"`
	Slug            string   `json:"slug"`
	EnabledServices []string `json:"enabledServices"`
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
