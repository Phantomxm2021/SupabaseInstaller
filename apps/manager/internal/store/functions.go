package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"supabase-manager/internal/contracts"
)

type FunctionCommand struct {
	OperationID   string
	ProjectID     string
	Name          string
	ArchiveSHA256 string
	CreatedAt     time.Time
}

// AdmitFunctionOperation serializes function actions with configuration and
// lifecycle operations at the project boundary. Retries for the same action
// reuse the existing queued/running operation.
func (s *Store) AdmitFunctionOperation(ctx context.Context, op contracts.Operation, name, archiveSHA256 string) (contracts.Operation, error) {
	var existing contracts.Operation
	err := s.InTx(ctx, func(tx *sql.Tx) error {
		var id, typ, status, created string
		err := tx.QueryRowContext(ctx, `SELECT id,type,status,created_at FROM operations WHERE project_id=? AND status IN (?,?) ORDER BY created_at DESC,id DESC LIMIT 1`, op.ProjectID, contracts.OperationQueued, contracts.OperationRunning).Scan(&id, &typ, &status, &created)
		if err == nil {
			if typ == string(op.Type) {
				var existingName string
				if e := tx.QueryRowContext(ctx, `SELECT function_name FROM function_operations WHERE operation_id=?`, id).Scan(&existingName); e == nil && existingName == name {
					existing = contracts.Operation{ID: id, ProjectID: op.ProjectID, Type: contracts.OperationType(typ), Status: contracts.OperationStatus(status)}
					existing.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
					return nil
				}
			}
			return ErrConfigurationBusy
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("check active function operation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO operations(id,project_id,type,status,progress,created_at) VALUES(?,?,?,?,?,?)`, op.ID, op.ProjectID, op.Type, op.Status, op.Progress, formatTime(op.CreatedAt)); err != nil {
			return fmt.Errorf("create function operation: %w", err)
		}
		payload, _ := json.Marshal(map[string]any{"status": "QUEUED", "function": name})
		if err := appendOperationEvent(ctx, tx, op.ID, "OPERATION_QUEUED", payload, op.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO function_operations(operation_id,project_id,function_name,archive_sha256,created_at) VALUES(?,?,?,?,?)`, op.ID, op.ProjectID, name, archiveSHA256, formatTime(op.CreatedAt)); err != nil {
			return fmt.Errorf("create function command: %w", err)
		}
		existing = op
		return nil
	})
	return existing, err
}

func (s *Store) GetFunctionCommand(ctx context.Context, operationID string) (FunctionCommand, error) {
	var c FunctionCommand
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT operation_id,project_id,function_name,COALESCE(archive_sha256,''),created_at FROM function_operations WHERE operation_id=?`, operationID).Scan(&c.OperationID, &c.ProjectID, &c.Name, &c.ArchiveSHA256, &created)
	if err == sql.ErrNoRows {
		return FunctionCommand{}, ErrNotFound
	}
	if err != nil {
		return FunctionCommand{}, err
	}
	c.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return c, err
}
