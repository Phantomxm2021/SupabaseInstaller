package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"supabase-manager/internal/contracts"
)

func (s *Store) CreateOperation(ctx context.Context, operation contracts.Operation, eventType string, payload json.RawMessage) error {
	return s.InTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO operations(id, project_id, type, status, progress, created_at)
VALUES (?, ?, ?, ?, ?, ?)`, operation.ID, operation.ProjectID, operation.Type, operation.Status, operation.Progress, formatTime(operation.CreatedAt))
		if err != nil {
			return fmt.Errorf("create operation: %w", err)
		}
		return appendOperationEvent(ctx, tx, operation.ID, eventType, payload, operation.CreatedAt)
	})
}

func (s *Store) GetOperation(ctx context.Context, id string) (contracts.Operation, error) {
	var operation contracts.Operation
	var startedAt, finishedAt sql.NullString
	var createdAt string
	err := s.db.QueryRowContext(ctx, `
SELECT id, project_id, type, status, COALESCE(current_step, ''), progress,
       COALESCE(error_code, ''), COALESCE(error_message, ''), started_at, finished_at, created_at
FROM operations WHERE id = ?`, id).Scan(
		&operation.ID, &operation.ProjectID, &operation.Type, &operation.Status, &operation.CurrentStep,
		&operation.Progress, &operation.ErrorCode, &operation.ErrorMessage, &startedAt, &finishedAt, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.Operation{}, ErrNotFound
	}
	if err != nil {
		return contracts.Operation{}, fmt.Errorf("get operation: %w", err)
	}
	operation.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return contracts.Operation{}, fmt.Errorf("parse operation created time: %w", err)
	}
	if startedAt.Valid {
		value, _ := time.Parse(time.RFC3339Nano, startedAt.String)
		operation.StartedAt = &value
	}
	if finishedAt.Valid {
		value, _ := time.Parse(time.RFC3339Nano, finishedAt.String)
		operation.FinishedAt = &value
	}
	return operation, nil
}

type OperationUpdate struct {
	Status       contracts.OperationStatus
	CurrentStep  string
	Progress     int
	ErrorCode    string
	ErrorMessage string
	StartedAt    *time.Time
	FinishedAt   *time.Time
	EventType    string
	EventPayload json.RawMessage
	EventTime    time.Time
}

func (s *Store) UpdateOperation(ctx context.Context, id string, update OperationUpdate) error {
	return s.InTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
UPDATE operations SET status = ?, current_step = NULLIF(?, ''), progress = ?, error_code = NULLIF(?, ''),
 error_message = NULLIF(?, ''), started_at = ?, finished_at = ? WHERE id = ?`,
			update.Status, update.CurrentStep, update.Progress, update.ErrorCode, update.ErrorMessage,
			nullableTime(update.StartedAt), nullableTime(update.FinishedAt), id,
		)
		if err != nil {
			return fmt.Errorf("update operation: %w", err)
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			return ErrNotFound
		}
		return appendOperationEvent(ctx, tx, id, update.EventType, update.EventPayload, update.EventTime)
	})
}

func (s *Store) OperationEventsAfter(ctx context.Context, operationID string, sequence int64) ([]contracts.OperationEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT operation_id, sequence, event_type, payload_json, created_at
FROM operation_events WHERE operation_id = ? AND sequence > ? ORDER BY sequence`, operationID, sequence)
	if err != nil {
		return nil, fmt.Errorf("list operation events: %w", err)
	}
	defer rows.Close()
	var events []contracts.OperationEvent
	for rows.Next() {
		var event contracts.OperationEvent
		var payload, createdAt string
		if err := rows.Scan(&event.OperationID, &event.Sequence, &event.Type, &payload, &createdAt); err != nil {
			return nil, fmt.Errorf("scan operation event: %w", err)
		}
		event.Payload = json.RawMessage(payload)
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		events = append(events, event)
	}
	return events, rows.Err()
}

func appendOperationEvent(ctx context.Context, tx *sql.Tx, operationID, eventType string, payload json.RawMessage, at time.Time) error {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	_, err := tx.ExecContext(ctx, `
INSERT INTO operation_events(operation_id, sequence, event_type, payload_json, created_at)
SELECT ?, COALESCE(MAX(sequence), 0) + 1, ?, ?, ? FROM operation_events WHERE operation_id = ?`,
		operationID, eventType, string(payload), formatTime(at), operationID)
	if err != nil {
		return fmt.Errorf("append operation event: %w", err)
	}
	return nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}
