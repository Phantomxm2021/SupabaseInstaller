package operation

import "supabase-manager/internal/contracts"

type Type = contracts.OperationType
type Status = contracts.OperationStatus
type Operation = contracts.Operation
type Event = contracts.OperationEvent

const (
	TypeCreate  = contracts.OperationCreate
	TypeStart   = contracts.OperationStart
	TypeStop    = contracts.OperationStop
	TypeRestart = contracts.OperationRestart
	TypeDelete  = contracts.OperationDelete
	Queued      = contracts.OperationQueued
	Running     = contracts.OperationRunning
	Succeeded   = contracts.OperationSucceeded
	Failed      = contracts.OperationFailed
	RollingBack = contracts.OperationRollingBack
	RolledBack  = contracts.OperationRolledBack
	Cancelled   = contracts.OperationCancelled
)
