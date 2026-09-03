package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"
	"unicode/utf8"
)

type FunctionLogLevel string

const (
	FunctionLogLevelDebug FunctionLogLevel = "debug"
	FunctionLogLevelInfo  FunctionLogLevel = "info"
	FunctionLogLevelWarn  FunctionLogLevel = "warn"
	FunctionLogLevelError FunctionLogLevel = "error"
)

type FunctionLogRecord struct {
	ID           string           `json:"id"`
	ProjectID    string           `json:"projectId"`
	FunctionName string           `json:"functionName"`
	ExecutionID  string           `json:"executionId"`
	EventType    string           `json:"eventType"`
	Message      string           `json:"message"`
	Timestamp    time.Time        `json:"timestamp"`
	IngestedAt   time.Time        `json:"ingestedAt"`
	Level        FunctionLogLevel `json:"level"`
	Truncated    bool             `json:"truncated"`
}

type FunctionLogQuery struct {
	Limit  int    `json:"limit"`
	Before string `json:"before"`
	After  string `json:"after"`
	Level  string `json:"level"`
	Search string `json:"search"`
}

type FunctionLogHealth struct {
	Status   string `json:"status"`
	Dropped  uint64 `json:"dropped"`
	Rejected uint64 `json:"rejected"`
	Detail   string `json:"detail"`
}

type FunctionLogPage struct {
	Logs        []FunctionLogRecord `json:"logs"`
	OlderCursor string              `json:"olderCursor"`
	NewerCursor string              `json:"newerCursor"`
	Health      FunctionLogHealth   `json:"health"`
	ServerTime  time.Time           `json:"serverTime"`
}

type EdgeRuntimeEvent struct {
	Version      int              `json:"version"`
	EventID      string           `json:"eventId"`
	FunctionName string           `json:"functionName"`
	ExecutionID  string           `json:"executionId"`
	EventType    string           `json:"eventType"`
	Message      string           `json:"message"`
	Timestamp    time.Time        `json:"timestamp"`
	Level        FunctionLogLevel `json:"level"`
}

type FunctionLogBatch struct {
	Version   int                `json:"version"`
	ProjectID string             `json:"projectId"`
	Events    []EdgeRuntimeEvent `json:"events"`
}

var ErrIncompatibleEdgeRuntimeEvent = errors.New("incompatible edge runtime event")

type EdgeRuntimeIncompatibilityError struct{ Reason string }

func (e *EdgeRuntimeIncompatibilityError) Error() string {
	return fmt.Sprintf("%s: %s", ErrIncompatibleEdgeRuntimeEvent, e.Reason)
}

func (e *EdgeRuntimeIncompatibilityError) Unwrap() error { return ErrIncompatibleEdgeRuntimeEvent }

func incompatibleEvent(reason string) error { return &EdgeRuntimeIncompatibilityError{Reason: reason} }

func ParseEdgeRuntimeEvent(raw []byte) (EdgeRuntimeEvent, error) {
	var envelope struct {
		Timestamp string          `json:"timestamp"`
		EventType string          `json:"event_type"`
		Event     json.RawMessage `json:"event"`
		Metadata  struct {
			ServicePath string `json:"service_path"`
			ExecutionID string `json:"execution_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return EdgeRuntimeEvent{}, incompatibleEvent("invalid JSON")
	}
	timestamp, err := time.Parse(time.RFC3339Nano, envelope.Timestamp)
	if err != nil {
		return EdgeRuntimeEvent{}, incompatibleEvent("invalid timestamp")
	}
	functionName := filepath.Base(filepath.Clean(envelope.Metadata.ServicePath))
	if envelope.Metadata.ServicePath == "" || ValidateFunctionName(functionName) != nil {
		return EdgeRuntimeEvent{}, incompatibleEvent("invalid metadata.service_path")
	}
	if envelope.Metadata.ExecutionID == "" {
		return EdgeRuntimeEvent{}, incompatibleEvent("missing metadata.execution_id")
	}

	event := EdgeRuntimeEvent{Version: 1, FunctionName: functionName, ExecutionID: envelope.Metadata.ExecutionID, EventType: envelope.EventType, Timestamp: timestamp}
	switch envelope.EventType {
	case "Boot":
		var body struct {
			BootTime *int64 `json:"boot_time"`
		}
		if json.Unmarshal(envelope.Event, &body) != nil || body.BootTime == nil {
			return EdgeRuntimeEvent{}, incompatibleEvent("invalid Boot event")
		}
		event.Level = FunctionLogLevelInfo
	case "Log":
		var body struct {
			Message *string `json:"msg"`
			Level   string  `json:"level"`
		}
		if json.Unmarshal(envelope.Event, &body) != nil || body.Message == nil {
			return EdgeRuntimeEvent{}, incompatibleEvent("invalid Log event")
		}
		event.Message = *body.Message
		switch body.Level {
		case "Debug":
			event.Level = FunctionLogLevelDebug
		case "Info":
			event.Level = FunctionLogLevelInfo
		case "Warn":
			event.Level = FunctionLogLevelWarn
		case "Error":
			event.Level = FunctionLogLevelError
		default:
			return EdgeRuntimeEvent{}, incompatibleEvent("invalid Log level")
		}
	case "UncaughtException":
		var body struct {
			Exception *string `json:"exception"`
		}
		if json.Unmarshal(envelope.Event, &body) != nil || body.Exception == nil {
			return EdgeRuntimeEvent{}, incompatibleEvent("invalid UncaughtException event")
		}
		event.Message = *body.Exception
		event.Level = FunctionLogLevelError
	default:
		return EdgeRuntimeEvent{}, incompatibleEvent("unknown event_type")
	}
	hash := sha256.Sum256(raw)
	event.EventID = hex.EncodeToString(hash[:])
	return event, nil
}

func ValidateFunctionLogQuery(query FunctionLogQuery) error {
	if query.Limit < 1 || query.Limit > 200 {
		return fmt.Errorf("limit must be between 1 and 200")
	}
	if query.Before != "" && query.After != "" {
		return fmt.Errorf("before and after are mutually exclusive")
	}
	if query.Level != "" && query.Level != string(FunctionLogLevelDebug) && query.Level != string(FunctionLogLevelInfo) && query.Level != string(FunctionLogLevelWarn) && query.Level != string(FunctionLogLevelError) {
		return fmt.Errorf("invalid level")
	}
	if !utf8.ValidString(query.Search) {
		return fmt.Errorf("search must be valid UTF-8")
	}
	if len(query.Search) > 256 {
		return fmt.Errorf("search must not exceed 256 UTF-8 bytes")
	}
	return nil
}

type FunctionLogCursor struct {
	Timestamp time.Time `json:"timestamp"`
	ID        string    `json:"id"`
}

func EncodeFunctionLogCursor(cursor FunctionLogCursor) (string, error) {
	if cursor.Timestamp.IsZero() || cursor.ID == "" {
		return "", fmt.Errorf("cursor timestamp and id are required")
	}
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func DecodeFunctionLogCursor(encoded string) (FunctionLogCursor, error) {
	if encoded == "" {
		return FunctionLogCursor{}, fmt.Errorf("cursor is empty")
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return FunctionLogCursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var cursor FunctionLogCursor
	if err := decoder.Decode(&cursor); err != nil {
		return FunctionLogCursor{}, fmt.Errorf("decode cursor JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) == nil {
		return FunctionLogCursor{}, fmt.Errorf("decode cursor JSON: trailing value")
	}
	if cursor.Timestamp.IsZero() || cursor.ID == "" {
		return FunctionLogCursor{}, fmt.Errorf("cursor timestamp and id are required")
	}
	return cursor, nil
}
