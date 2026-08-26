package contracts

import (
	"encoding/json"
	"time"
)

type OperationType string

const (
	OperationCreate        OperationType = "CREATE"
	OperationStart         OperationType = "START"
	OperationStop          OperationType = "STOP"
	OperationRestart       OperationType = "RESTART"
	OperationUpdateConfig  OperationType = "UPDATE_CONFIG"
	OperationUpdateVersion OperationType = "UPDATE_VERSION"
	OperationDelete        OperationType = "DELETE"
	OperationBackup        OperationType = "BACKUP"
	OperationRestore       OperationType = "RESTORE"
)

type OperationStatus string

const (
	OperationQueued      OperationStatus = "QUEUED"
	OperationRunning     OperationStatus = "RUNNING"
	OperationSucceeded   OperationStatus = "SUCCEEDED"
	OperationFailed      OperationStatus = "FAILED"
	OperationRollingBack OperationStatus = "ROLLING_BACK"
	OperationRolledBack  OperationStatus = "ROLLED_BACK"
	OperationCancelled   OperationStatus = "CANCELLED"
)

type Operation struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"projectId"`
	Type         OperationType   `json:"type"`
	Status       OperationStatus `json:"status"`
	CurrentStep  string          `json:"currentStep,omitempty"`
	Progress     int             `json:"progress"`
	ErrorCode    string          `json:"errorCode,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	StartedAt    *time.Time      `json:"startedAt,omitempty"`
	FinishedAt   *time.Time      `json:"finishedAt,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

type OperationEvent struct {
	OperationID string          `json:"operationId"`
	Sequence    int64           `json:"sequence"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"createdAt"`
}
