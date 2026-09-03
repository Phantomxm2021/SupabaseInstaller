package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
	const servicePathPrefix = "./examples/"
	functionName := strings.TrimPrefix(envelope.Metadata.ServicePath, servicePathPrefix)
	if !strings.HasPrefix(envelope.Metadata.ServicePath, servicePathPrefix) ||
		functionName == "" || strings.ContainsAny(functionName, `/\\`) ||
		ValidateFunctionName(functionName) != nil {
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
	if query.Before != "" {
		if _, err := DecodeFunctionLogCursor(query.Before); err != nil {
			return fmt.Errorf("invalid before cursor: %w", err)
		}
	}
	if query.After != "" {
		if _, err := DecodeFunctionLogCursor(query.After); err != nil {
			return fmt.Errorf("invalid after cursor: %w", err)
		}
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
	var cursor FunctionLogCursor
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return FunctionLogCursor{}, fmt.Errorf("decode cursor JSON: expected object")
	}
	seen := make(map[string]bool, 2)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return FunctionLogCursor{}, fmt.Errorf("decode cursor JSON key: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok || seen[key] {
			return FunctionLogCursor{}, fmt.Errorf("decode cursor JSON: invalid or duplicate field")
		}
		seen[key] = true
		switch key {
		case "timestamp":
			err = decoder.Decode(&cursor.Timestamp)
		case "id":
			err = decoder.Decode(&cursor.ID)
		default:
			return FunctionLogCursor{}, fmt.Errorf("decode cursor JSON: unknown field %q", key)
		}
		if err != nil {
			return FunctionLogCursor{}, fmt.Errorf("decode cursor JSON field %q: %w", key, err)
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return FunctionLogCursor{}, fmt.Errorf("decode cursor JSON: incomplete object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return FunctionLogCursor{}, fmt.Errorf("decode cursor JSON: trailing data")
	}
	if cursor.Timestamp.IsZero() || cursor.ID == "" {
		return FunctionLogCursor{}, fmt.Errorf("cursor timestamp and id are required")
	}
	return cursor, nil
}
