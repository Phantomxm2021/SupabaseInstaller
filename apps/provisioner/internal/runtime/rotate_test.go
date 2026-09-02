package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

type rotationTestRunner struct {
	fakeReconcileRunner
	rotations [][2]string
}

func (r *rotationTestRunner) RotateDatabasePassword(_ context.Context, _ compose.ProjectRef, oldPassword, newPassword string) error {
	r.rotations = append(r.rotations, [2]string{oldPassword, newPassword})
	return nil
}

func TestRotateDatabasePasswordPublishesNewGenerationAndMetadata(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &rotationTestRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	request := contracts.RotateDatabasePasswordRequest{
		OperationKind: "ROTATE_DATABASE_PASSWORD", OperationID: "rotate-1", IdempotencyKey: "rotate-key",
		ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", ExpectedRevision: 1, NextRevision: 2,
		OldPassword: "db-password", NewPassword: "new-db-password",
		Configuration: baseConfig(), Secrets: contracts.ProjectSecrets{DatabasePassword: "new-db-password", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"},
	}
	result, err := backend.RotateDatabasePassword(context.Background(), request)
	if err != nil || result.Revision != 2 {
		t.Fatalf("rotation = %#v, %v", result, err)
	}
	if len(runner.rotations) != 1 || runner.rotations[0] != [2]string{"db-password", "new-db-password"} {
		t.Fatalf("database rotations = %#v", runner.rotations)
	}
	metadata, err := root.Metadata("bee")
	if err != nil || metadata.Revision != 2 {
		t.Fatalf("metadata = %#v, %v", metadata, err)
	}
	if metadata.Rotation == nil || metadata.Rotation.Phase != "provisioner-committed" {
		t.Fatalf("rotation journal = %#v, want provisioner-committed until Manager confirmation", metadata.Rotation)
	}
	confirmation := contracts.ConfirmDatabasePasswordRotationRequest{OperationID: request.OperationID, IdempotencyKey: "confirm-key", ProjectID: request.ProjectID, Slug: request.Slug, ExpectedRevision: request.ExpectedRevision, NextRevision: request.NextRevision}
	if err := backend.ConfirmDatabasePasswordRotation(context.Background(), confirmation); err != nil {
		t.Fatalf("confirm rotation publication: %v", err)
	}
	metadata, err = root.Metadata("bee")
	if err != nil || metadata.Rotation != nil {
		t.Fatalf("confirmed metadata = %#v, %v; want cleared journal", metadata, err)
	}
	if err := backend.ConfirmDatabasePasswordRotation(context.Background(), confirmation); err != nil {
		t.Fatalf("idempotent rotation confirmation: %v", err)
	}
}

func TestRotateDatabasePasswordHealthFailureRestoresOldRoleAndReportsRollback(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &rotationTestRunner{}
	inspector := &sequenceInspector{reports: []health.Report{{Health: contracts.HealthHealthy}, {Health: contracts.HealthUnhealthy}, {Health: contracts.HealthHealthy}}}
	backend := NewBackend(root, runner, inspector)
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	request := contracts.RotateDatabasePasswordRequest{OperationKind: "ROTATE_DATABASE_PASSWORD", OperationID: "rotate-fail", IdempotencyKey: "rotate-fail-key", ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", ExpectedRevision: 1, NextRevision: 2, OldPassword: "db-password", NewPassword: "new-db-password", Configuration: baseConfig(), Secrets: contracts.ProjectSecrets{DatabasePassword: "new-db-password", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"}}
	_, err = backend.RotateDatabasePassword(context.Background(), request)
	var failure *contracts.ReconcileFailure
	if err == nil || !errors.As(err, &failure) || !failure.RollbackSucceeded {
		t.Fatalf("rotation failure = %v, want successful typed rollback", err)
	}
	if len(runner.rotations) != 2 || runner.rotations[1] != [2]string{"new-db-password", "db-password"} {
		t.Fatalf("role recovery calls = %#v", runner.rotations)
	}
}

func TestRotateDatabasePasswordPreservesDependentServiceRestartFailure(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &rotationTestRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	runner.recreateError = errors.New("compose action failed: exit status 1; output=auth dependency failed to start")
	request := contracts.RotateDatabasePasswordRequest{OperationKind: "ROTATE_DATABASE_PASSWORD", OperationID: "rotate-recreate-fail", IdempotencyKey: "rotate-recreate-fail-key", ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", ExpectedRevision: 1, NextRevision: 2, OldPassword: "db-password", NewPassword: "new-db-password", Configuration: baseConfig(), Secrets: contracts.ProjectSecrets{DatabasePassword: "new-db-password", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"}}
	_, err = backend.RotateDatabasePassword(context.Background(), request)
	var failure *contracts.ReconcileFailure
	if err == nil || !errors.As(err, &failure) || failure.Cause == nil || !strings.Contains(failure.Cause.Error(), "auth dependency failed to start") {
		t.Fatalf("rotation failure = %#v, want dependent-service Compose diagnostic", failure)
	}
}

func TestRollbackDatabasePasswordKeepsCompensatedOldGenerationCurrent(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &rotationTestRunner{}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	old, _ := root.CurrentRuntimeGeneration("bee")
	req := contracts.RotateDatabasePasswordRequest{OperationKind: "ROTATE_DATABASE_PASSWORD", OperationID: "rotate-ext", IdempotencyKey: "rotate-ext-key", ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", ExpectedRevision: 1, NextRevision: 2, OldPassword: "db-password", NewPassword: "new-db-password", Configuration: baseConfig(), Secrets: contracts.ProjectSecrets{DatabasePassword: "new-db-password", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"}}
	if _, err := backend.RotateDatabasePassword(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	newRef, _ := root.CurrentRuntimeGeneration("bee")
	rollback := req
	rollback.OperationKind = "ROLLBACK_DATABASE_PASSWORD"
	rollback.IdempotencyKey = "rotate-ext-rollback"
	rollback.OldPassword, rollback.NewPassword = req.NewPassword, req.OldPassword
	if err := backend.RollbackDatabasePassword(context.Background(), rollback); err != nil {
		t.Fatal(err)
	}
	final, _ := root.CurrentRuntimeGeneration("bee")
	if final.ComposeFile != old.ComposeFile || final.ComposeFile == newRef.ComposeFile {
		t.Fatalf("current generation after rollback=%q, old=%q new=%q", final.ComposeFile, old.ComposeFile, newRef.ComposeFile)
	}
	metadata, _ := root.Metadata("bee")
	if metadata.Revision != 1 {
		t.Fatalf("metadata revision=%d want 1", metadata.Revision)
	}
	if err := backend.RollbackDatabasePassword(context.Background(), rollback); err != nil {
		t.Fatalf("idempotent rollback: %v", err)
	}
}
