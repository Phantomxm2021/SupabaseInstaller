package install

import (
	"context"
	"fmt"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/ports"
	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type Provisioner interface {
	Prepare(ctx context.Context, request contracts.PrepareProjectRequest) (contracts.PrepareProjectResponse, error)
	Lifecycle(ctx context.Context, request contracts.LifecycleRequest) error
	Inspect(ctx context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error)
}

// ReconcileProvisioner is implemented by the current private RPC client. The
// legacy Prepare methods remain optional for old test doubles and deployments.
type ReconcileProvisioner interface {
	Reconcile(ctx context.Context, request contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error)
}

type SecretGenerator interface {
	Generate() (contracts.ProjectSecrets, error)
}

type Orchestrator struct {
	store       *store.Store
	operations  *operation.Service
	ports       *ports.Allocator
	cipher      *managersecrets.Cipher
	provisioner Provisioner
	generator   SecretGenerator
	now         func() time.Time
}

func NewOrchestrator(store *store.Store, operations *operation.Service, ports *ports.Allocator, cipher *managersecrets.Cipher, provisioner Provisioner, generator SecretGenerator, now func() time.Time) *Orchestrator {
	return &Orchestrator{store: store, operations: operations, ports: ports, cipher: cipher, provisioner: provisioner, generator: generator, now: now}
}

func (orchestrator *Orchestrator) CreateOperation(ctx context.Context, projectID string) (operation.Operation, error) {
	return orchestrator.operations.Create(ctx, projectID, operation.TypeCreate)
}

func (orchestrator *Orchestrator) Install(ctx context.Context, project contracts.Project) (operation.Operation, error) {
	created, err := orchestrator.CreateOperation(ctx, project.ID)
	if err != nil {
		return operation.Operation{}, err
	}
	return orchestrator.Run(ctx, project, created)
}

func (orchestrator *Orchestrator) Run(ctx context.Context, project contracts.Project, current operation.Operation) (operation.Operation, error) {
	if err := orchestrator.operations.Start(ctx, current.ID); err != nil {
		return current, err
	}
	_ = orchestrator.store.UpdateProjectStatus(ctx, project.ID, contracts.ProjectStatusInstalling, contracts.HealthStarting)

	if err := orchestrator.step(ctx, current.ID, "VALIDATE_PORTS", 5, func() error {
		_, err := orchestrator.ports.Reserve(ctx, project.ID, ports.KindAPI)
		return err
	}); err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "VALIDATE_PORTS", err)
	}
	apiPort, _ := orchestrator.ports.Reserve(ctx, project.ID, ports.KindAPI)

	generated, err := orchestrator.generator.Generate()
	if err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "GENERATE_SECRETS", err)
	}
	if err := orchestrator.step(ctx, current.ID, "GENERATE_SECRETS", 15, func() error {
		return orchestrator.persistSecrets(ctx, project.ID, generated)
	}); err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "GENERATE_SECRETS", err)
	}

	configuration := contracts.ProjectConfiguration{Revision: 1, General: contracts.GeneralConfig{Domain: project.Domain, SiteURL: project.SiteURL, SupabaseVersion: project.SupabaseVersion}, Services: project.Services}
	if snapshot, readErr := orchestrator.store.GetConfiguration(ctx, project.ID); readErr == nil {
		configuration = snapshot.Configuration
		configuration.Revision = 1
	}
	if reconcileProvisioner, ok := orchestrator.provisioner.(ReconcileProvisioner); ok {
		reconcile := contracts.ReconcileProjectRequest{
			OperationID: current.ID, IdempotencyKey: current.ID + ":reconcile", ProjectID: project.ID,
			ProjectName: project.Name, Slug: project.Slug, ExpectedRevision: 0, NextRevision: 1,
			APIPort: apiPort, Configuration: configuration, Secrets: generated,
		}
		if err := orchestrator.step(ctx, current.ID, "RECONCILE_RUNTIME", 35, func() error { _, err := reconcileProvisioner.Reconcile(ctx, reconcile); return err }); err != nil {
			return orchestrator.rollback(ctx, project, current.ID, "RECONCILE_RUNTIME", err)
		}
	} else {
		prepare := contracts.PrepareProjectRequest{
			OperationID: current.ID, IdempotencyKey: current.ID + ":prepare", ProjectID: project.ID, ProjectName: project.Name,
			Slug: project.Slug, ExpectedRevision: 0, NextRevision: 1, Domain: project.Domain, SiteURL: project.SiteURL,
			APIPort: apiPort, Secrets: generated,
		}
		if err := orchestrator.step(ctx, current.ID, "PREPARE_SUPABASE", 35, func() error { _, err := orchestrator.provisioner.Prepare(ctx, prepare); return err }); err != nil {
			return orchestrator.rollback(ctx, project, current.ID, "PREPARE_SUPABASE", err)
		}
	}
	if err := orchestrator.step(ctx, current.ID, "START_RUNTIME", 70, func() error {
		return orchestrator.provisioner.Lifecycle(ctx, contracts.LifecycleRequest{OperationID: current.ID, IdempotencyKey: current.ID + ":start", ProjectID: project.ID, Slug: project.Slug, Action: contracts.LifecycleStart})
	}); err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "START_AUTH", err)
	}
	if err := orchestrator.step(ctx, current.ID, "FINAL_HEALTH_CHECK", 95, func() error {
		result, err := orchestrator.provisioner.Inspect(ctx, contracts.InspectProjectRequest{ProjectID: project.ID, Slug: project.Slug, EnabledServices: enabledComposeServices(configuration.Services)})
		if err != nil {
			return err
		}
		if result.Health != contracts.HealthHealthy {
			return fmt.Errorf("runtime health is %s", result.Health)
		}
		return nil
	}); err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "FINAL_HEALTH_CHECK", err)
	}
	if err := orchestrator.store.UpdateProjectStatus(ctx, project.ID, contracts.ProjectStatusRunning, contracts.HealthHealthy); err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "MARK_RUNNING", err)
	}
	if err := orchestrator.operations.Succeed(ctx, current.ID); err != nil {
		return current, err
	}
	return orchestrator.operations.Get(ctx, current.ID)
}

func (orchestrator *Orchestrator) step(ctx context.Context, operationID, name string, progress int, action func() error) error {
	if err := orchestrator.operations.StartStep(ctx, operationID, name, progress); err != nil {
		return err
	}
	if err := action(); err != nil {
		return err
	}
	return orchestrator.operations.CompleteStep(ctx, operationID, name, progress)
}

func (orchestrator *Orchestrator) rollback(ctx context.Context, project contracts.Project, operationID, step string, cause error) (operation.Operation, error) {
	_ = orchestrator.operations.Fail(ctx, operationID, step, cause)
	_ = orchestrator.operations.BeginRollback(ctx, operationID)
	rollbackErr := orchestrator.provisioner.Lifecycle(ctx, contracts.LifecycleRequest{OperationID: operationID, IdempotencyKey: operationID + ":rollback", ProjectID: project.ID, Slug: project.Slug, Action: contracts.LifecycleDeleteRuntime})
	if rollbackErr == nil {
		_ = orchestrator.operations.CompleteRollback(ctx, operationID)
	}
	_ = orchestrator.store.UpdateProjectStatus(ctx, project.ID, contracts.ProjectStatusFailed, contracts.HealthUnknown)
	latest, _ := orchestrator.operations.Get(ctx, operationID)
	if rollbackErr != nil {
		return latest, fmt.Errorf("install failed: %w; rollback failed: %v", cause, rollbackErr)
	}
	return latest, cause
}

func (orchestrator *Orchestrator) persistSecrets(ctx context.Context, projectID string, generated contracts.ProjectSecrets) error {
	values := map[string]string{
		"database-password":          generated.DatabasePassword,
		"jwt-secret":                 generated.JWTSecret,
		"anon-key":                   generated.AnonKey,
		"service-role-key":           generated.ServiceRoleKey,
		"dashboard-password":         generated.DashboardPassword,
		"secret-key-base":            generated.SecretKeyBase,
		"vault-encryption-key":       generated.VaultEncryptionKey,
		"realtime-db-encryption-key": generated.RealtimeDBEncryptionKey, "logflare-public-access-token": generated.LogflarePublicAccessToken, "logflare-private-access-token": generated.LogflarePrivateAccessToken,
		"s3-protocol-access-key-id": generated.S3ProtocolAccessKeyID, "s3-protocol-access-key-secret": generated.S3ProtocolAccessKeySecret, "pooler-tenant-id": generated.PoolerTenantID,
	}
	for kind, plaintext := range values {
		envelope, err := orchestrator.cipher.Encrypt(projectID, kind, []byte(plaintext))
		if err != nil {
			return err
		}
		if err := orchestrator.store.PutSecret(ctx, projectID, kind, envelope); err != nil {
			return err
		}
	}
	return nil
}

func lightweightServiceNames() []string {
	return []string{"db", "auth", "rest", "meta", "studio", "api-gw"}
}

func enabledComposeServices(services contracts.Services) []string {
	result := make([]string, 0, 14)
	if services.Database {
		result = append(result, "db")
	}
	if services.Gateway {
		result = append(result, "api-gw")
	}
	if services.Auth {
		result = append(result, "auth")
	}
	if services.REST {
		result = append(result, "rest")
	}
	if services.Studio {
		result = append(result, "studio")
	}
	if services.PostgresMeta {
		result = append(result, "meta")
	}
	if services.Realtime {
		result = append(result, "realtime")
	}
	if services.Storage {
		result = append(result, "storage")
	}
	if services.Imgproxy {
		result = append(result, "imgproxy")
	}
	if services.Functions {
		result = append(result, "functions")
	}
	if services.Supavisor {
		result = append(result, "pooler")
	}
	if services.Logs {
		result = append(result, "analytics", "vector")
	}
	return result
}

// EnabledComposeServices is the Manager-side projection used by health checks
// and installation handoff; it mirrors the pinned renderer's service names.
func EnabledComposeServices(services contracts.Services) []string {
	return enabledComposeServices(services)
}
