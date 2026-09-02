package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"supabase-manager/apps/provisioner/internal/compose"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

type rotationTestRunner struct {
	fakeReconcileRunner
	rotations      [][2]string
	rotationErrors []error
	recreateErrors []error
}

func (r *rotationTestRunner) RotateDatabasePassword(_ context.Context, _ compose.ProjectRef, oldPassword, newPassword string) error {
	r.rotations = append(r.rotations, [2]string{oldPassword, newPassword})
	if len(r.rotationErrors) > 0 {
		err := r.rotationErrors[0]
		r.rotationErrors = r.rotationErrors[1:]
		return err
	}
	return nil
}

func (r *rotationTestRunner) Recreate(_ context.Context, _ compose.ProjectRef, services ...string) error {
	r.recreated = append(r.recreated, services...)
	if len(r.recreateErrors) > 0 {
		err := r.recreateErrors[0]
		r.recreateErrors = r.recreateErrors[1:]
		return err
	}
	return r.recreateError
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
	current, err := root.CurrentRuntimeFiles("bee")
	if err != nil {
		t.Fatal(err)
	}
	composeFile, err := os.ReadFile(current.ComposeFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(composeFile), ".candidate-") {
		t.Fatalf("published Compose retains candidate functions env path: %q", composeFile)
	}
	if !strings.Contains(string(composeFile), ".manager-runtime/current/.env.functions") {
		t.Fatalf("published Compose = %q, want stable functions env path", composeFile)
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
	if failure.Cause == nil || !strings.Contains(failure.Cause.Error(), "runtime health is UNHEALTHY") {
		t.Fatalf("rotation failure cause = %#v, want original health diagnostic", failure)
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

func TestRotateDatabasePasswordHealthFailurePreservesRollbackRecreateFailure(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &rotationTestRunner{recreateErrors: []error{nil, errors.New("rollback recreate compose output: auth failed")}}
	inspector := &sequenceInspector{reports: []health.Report{{Health: contracts.HealthHealthy}, {Health: contracts.HealthUnhealthy}}}
	backend := NewBackend(root, runner, inspector)
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	request := contracts.RotateDatabasePasswordRequest{OperationKind: "ROTATE_DATABASE_PASSWORD", OperationID: "rotate-health-rollback", IdempotencyKey: "rotate-health-rollback-key", ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", ExpectedRevision: 1, NextRevision: 2, OldPassword: "db-password", NewPassword: "new-db-password", Configuration: baseConfig(), Secrets: contracts.ProjectSecrets{DatabasePassword: "new-db-password", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"}}
	_, err = backend.RotateDatabasePassword(context.Background(), request)
	var failure *contracts.ReconcileFailure
	if err == nil || !errors.As(err, &failure) || failure.RollbackSucceeded || failure.Cause == nil {
		t.Fatalf("rotation failure = %#v, want failed typed rollback", failure)
	}
	for _, want := range []string{"runtime health is UNHEALTHY", "rollback recreate compose output: auth failed"} {
		if !strings.Contains(failure.Cause.Error(), want) {
			t.Fatalf("rotation failure cause = %q, missing %q", failure.Cause, want)
		}
	}
}

func TestRotateDatabasePasswordPreservesDatabaseUpdateFailure(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &rotationTestRunner{rotationErrors: []error{errors.New("psql password update output: permission denied")}}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}
	request := contracts.RotateDatabasePasswordRequest{OperationKind: "ROTATE_DATABASE_PASSWORD", OperationID: "rotate-db-update", IdempotencyKey: "rotate-db-update-key", ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", ExpectedRevision: 1, NextRevision: 2, OldPassword: "db-password", NewPassword: "new-db-password", Configuration: baseConfig(), Secrets: contracts.ProjectSecrets{DatabasePassword: "new-db-password", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-key"}}
	_, err = backend.RotateDatabasePassword(context.Background(), request)
	var failure *contracts.ReconcileFailure
	if err == nil || !errors.As(err, &failure) || failure.Cause == nil || !strings.Contains(failure.Cause.Error(), "psql password update output: permission denied") {
		t.Fatalf("rotation failure = %#v, want database update diagnostic", failure)
	}
}

func TestRotateDatabasePasswordReplayPreservesRedactedFailureDiagnostic(t *testing.T) {
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := contracts.RotateDatabasePasswordRequest{OperationKind: "ROTATE_DATABASE_PASSWORD", OperationID: "rotate-replay-diagnostic", IdempotencyKey: "rotate-replay-diagnostic-key", ProjectID: "project-1", ProjectName: "Bee", Slug: "bee", ExpectedRevision: 1, NextRevision: 2, OldPassword: "old-password", NewPassword: "new-password", Configuration: baseConfig(), Secrets: contracts.ProjectSecrets{DatabasePassword: "database-password", JWTSecret: "jwt-secret", AnonKey: "anon-key", ServiceRoleKey: "service-role-key", DashboardPassword: "dashboard-password", SecretKeyBase: "secret-key-base", VaultEncryptionKey: "vault-encryption-key", RealtimeDBEncryptionKey: "realtime-encryption-key", LogflarePublicAccessToken: "logflare-public-token", LogflarePrivateAccessToken: "logflare-private-token", S3ProtocolAccessKeyID: "s3-access-key-id", S3ProtocolAccessKeySecret: "s3-access-key-secret", PoolerTenantID: "pooler-tenant-id"}, RuntimeSecrets: map[string]string{"runtime.secret": "runtime-secret"}}
	knownSecrets := []string{request.OldPassword, request.NewPassword, request.Secrets.DatabasePassword, request.Secrets.JWTSecret, request.Secrets.AnonKey, request.Secrets.ServiceRoleKey, request.Secrets.DashboardPassword, request.Secrets.SecretKeyBase, request.Secrets.VaultEncryptionKey, request.Secrets.RealtimeDBEncryptionKey, request.Secrets.LogflarePublicAccessToken, request.Secrets.LogflarePrivateAccessToken, request.Secrets.S3ProtocolAccessKeyID, request.Secrets.S3ProtocolAccessKeySecret, request.Secrets.PoolerTenantID, request.RuntimeSecrets["runtime.secret"]}
	runner := &rotationTestRunner{rotationErrors: []error{fmt.Errorf("compose operation failed: network unavailable; values=%s", strings.Join(knownSecrets, ","))}}
	backend := NewBackend(root, runner, &sequenceInspector{})
	if _, err := backend.Reconcile(context.Background(), reconcileRequest(baseConfig(), 0, 1)); err != nil {
		t.Fatal(err)
	}

	initial, err := backend.RotateDatabasePassword(context.Background(), request)
	var initialFailure *contracts.ReconcileFailure
	if err == nil || !errors.As(err, &initialFailure) || !strings.Contains(initial.Diagnostic, "operation failed: network unavailable") {
		t.Fatalf("initial rotation result=%#v error=%v, want retained safe diagnostic", initial, err)
	}
	for _, secret := range knownSecrets {
		if strings.Contains(initial.Diagnostic, secret) {
			t.Fatalf("initial diagnostic leaked %q: %q", secret, initial.Diagnostic)
		}
	}

	replayed, err := backend.RotateDatabasePassword(context.Background(), request)
	var replayFailure *contracts.ReconcileFailure
	if err == nil || !errors.As(err, &replayFailure) || replayFailure.Cause == nil || !strings.Contains(replayFailure.Cause.Error(), "operation failed: network unavailable") {
		t.Fatalf("replayed rotation result=%#v error=%v, want original safe diagnostic", replayed, err)
	}
	for _, secret := range knownSecrets {
		if strings.Contains(replayed.Diagnostic, secret) || strings.Contains(replayFailure.Cause.Error(), secret) {
			t.Fatalf("replayed failure leaked %q: result=%#v cause=%q", secret, replayed, replayFailure.Cause)
		}
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
