package contracts

import (
	"errors"
	"time"
)

var ErrStaleConfigRevision = errors.New("stale config revision")
var ErrInvalidReconcileRevision = errors.New("invalid reconcile revision")

// ReconcileFailure preserves the runtime outcome without exposing rendered
// files or secret material through an API error.
type ReconcileFailure struct {
	Cause             error
	RollbackSucceeded bool
	// RuntimeChanged distinguishes failures before publication (render/stage/
	// validation) from failures after Docker/runtime side effects. Managers can
	// safely restore an admitted desired revision only for the former or after
	// a confirmed rollback.
	RuntimeChanged bool
	Response       ReconcileProjectResponse
}

func (e *ReconcileFailure) Error() string { return "runtime reconciliation failed" }
func (e *ReconcileFailure) Unwrap() error { return e.Cause }

type ReconcileProjectRequest struct {
	OperationID      string               `json:"operationId"`
	IdempotencyKey   string               `json:"idempotencyKey"`
	ProjectID        string               `json:"projectId"`
	ProjectName      string               `json:"projectName"`
	Slug             string               `json:"slug"`
	ExpectedRevision int64                `json:"expectedRevision"`
	NextRevision     int64                `json:"nextRevision"`
	APIPort          int                  `json:"apiPort"`
	Configuration    ProjectConfiguration `json:"configuration"`
	Secrets          ProjectSecrets       `json:"secrets"`
	RuntimeSecrets   map[string]string    `json:"runtimeSecrets,omitempty"`
	Fence            int64                `json:"fence,omitempty"`
}

type ReconcileProjectResponse struct {
	OperationID       string    `json:"operationId"`
	ProjectID         string    `json:"projectId"`
	Revision          int64     `json:"revision"`
	EnabledServices   []string  `json:"enabledServices"`
	RecreatedServices []string  `json:"recreatedServices"`
	RolledBack        bool      `json:"rolledBack"`
	RuntimeChanged    bool      `json:"runtimeChanged,omitempty"`
	Error             *APIError `json:"error,omitempty"`
}

// RotateDatabasePasswordRequest is a narrowly scoped sensitive RPC. The
// Provisioner receives values only over the authenticated private channel and
// never includes them in responses, logs, or operation events.
type RotateDatabasePasswordRequest struct {
	OperationKind        string               `json:"operationKind"`
	OperationID          string               `json:"operationId"`
	IdempotencyKey       string               `json:"idempotencyKey"`
	ProjectID            string               `json:"projectId"`
	ProjectName          string               `json:"projectName"`
	Slug                 string               `json:"slug"`
	ExpectedRevision     int64                `json:"expectedRevision"`
	NextRevision         int64                `json:"nextRevision"`
	OldPassword          string               `json:"oldPassword"`
	NewPassword          string               `json:"newPassword"`
	Configuration        ProjectConfiguration `json:"configuration"`
	Secrets              ProjectSecrets       `json:"secrets"`
	RuntimeSecrets       map[string]string    `json:"runtimeSecrets,omitempty"`
	Fence                int64                `json:"fence,omitempty"`
	OldRuntimeGeneration string               `json:"oldRuntimeGeneration,omitempty"`
	NewRuntimeGeneration string               `json:"newRuntimeGeneration,omitempty"`
}

type RotateDatabasePasswordResponse struct {
	OperationID    string    `json:"operationId"`
	ProjectID      string    `json:"projectId"`
	Revision       int64     `json:"revision"`
	RolledBack     bool      `json:"rolledBack"`
	RuntimeChanged bool      `json:"runtimeChanged,omitempty"`
	Error          *APIError `json:"error,omitempty"`
}

// ConfirmDatabasePasswordRotation closes the durable rotation journal only
// after Manager has committed the encrypted secret.
type ConfirmDatabasePasswordRotationRequest struct {
	OperationID      string `json:"operationId"`
	IdempotencyKey   string `json:"idempotencyKey"`
	ProjectID        string `json:"projectId"`
	Slug             string `json:"slug"`
	ExpectedRevision int64  `json:"expectedRevision"`
	NextRevision     int64  `json:"nextRevision"`
	Fence            int64  `json:"fence,omitempty"`
}

type ProjectSecrets struct {
	DatabasePassword           string `json:"databasePassword"`
	JWTSecret                  string `json:"jwtSecret"`
	AnonKey                    string `json:"anonKey"`
	ServiceRoleKey             string `json:"serviceRoleKey"`
	DashboardPassword          string `json:"dashboardPassword"`
	SecretKeyBase              string `json:"secretKeyBase"`
	VaultEncryptionKey         string `json:"vaultEncryptionKey"`
	RealtimeDBEncryptionKey    string `json:"realtimeDbEncryptionKey"`
	LogflarePublicAccessToken  string `json:"logflarePublicAccessToken"`
	LogflarePrivateAccessToken string `json:"logflarePrivateAccessToken"`
	S3ProtocolAccessKeyID      string `json:"s3ProtocolAccessKeyId"`
	S3ProtocolAccessKeySecret  string `json:"s3ProtocolAccessKeySecret"`
	PoolerTenantID             string `json:"poolerTenantId"`
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
	ProjectName        string          `json:"projectName,omitempty"`
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

// HostResources is a redacted, read-only snapshot used by the projects
// landing page. It contains capacity/usage values only; no runtime secrets or
// rendered project configuration crosses the provisioner boundary.
type HostResources struct {
	CPUPercent       float64   `json:"cpuPercent"`
	CPUCores         int       `json:"cpuCores"`
	MemoryUsedBytes  uint64    `json:"memoryUsedBytes"`
	MemoryTotalBytes uint64    `json:"memoryTotalBytes"`
	DiskUsedBytes    uint64    `json:"diskUsedBytes"`
	DiskTotalBytes   uint64    `json:"diskTotalBytes"`
	CollectedAt      time.Time `json:"collectedAt"`
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
