package install

import (
	"context"
	"errors"
	"fmt"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/ports"
	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type Provisioner interface {
	Reconcile(ctx context.Context, request contracts.ReconcileProjectRequest) (contracts.ReconcileProjectResponse, error)
	Lifecycle(ctx context.Context, request contracts.LifecycleRequest) error
	Inspect(ctx context.Context, request contracts.InspectProjectRequest) (contracts.InspectProjectResponse, error)
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

	snapshot, readErr := orchestrator.store.GetDesiredConfiguration(ctx, project.ID)
	if readErr != nil {
		return orchestrator.rollback(ctx, project, current.ID, "LOAD_CONFIGURATION", readErr)
	}
	configuration := snapshot.Configuration
	configuration.Revision = 1
	if err := orchestrator.step(ctx, current.ID, "VALIDATE_PORTS", 5, func() error {
		if err := orchestrator.allocateConfigurationPorts(ctx, project.ID, &configuration); err != nil {
			return err
		}
		return orchestrator.store.PersistAllocatedConfiguration(ctx, project.ID, configuration, orchestrator.now())
	}); err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "VALIDATE_PORTS", err)
	}
	apiPort := configuration.Network.APIPort

	generated, persisted, err := orchestrator.loadPersistedSecrets(ctx, project.ID)
	if err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "LOAD_SECRETS", err)
	}
	completeSecretSet := len(persisted) == len(persistedProjectSecretSpecs())
	if !completeSecretSet {
		fresh, generateErr := orchestrator.generator.Generate()
		if generateErr != nil {
			return orchestrator.rollback(ctx, project, current.ID, "GENERATE_SECRETS", generateErr)
		}
		for _, spec := range persistedProjectSecretSpecs() {
			if _, exists := persisted[spec.kind]; !exists {
				spec.set(&generated, spec.get(fresh))
			}
		}
	}
	if err := orchestrator.step(ctx, current.ID, "GENERATE_SECRETS", 15, func() error {
		if completeSecretSet {
			return nil
		}
		return orchestrator.persistSecrets(ctx, project.ID, generated, persisted)
	}); err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "GENERATE_SECRETS", err)
	}
	if err := orchestrator.applyConfiguredStudioPassword(ctx, project.ID, configuration, &generated); err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "HYDRATE_SECRETS", err)
	}

	runtimeSecrets, hydrationErr := orchestrator.hydrateConfiguredSecrets(ctx, project.ID, configuration)
	if hydrationErr != nil {
		return orchestrator.rollback(ctx, project, current.ID, "HYDRATE_SECRETS", hydrationErr)
	}
	reconcile := contracts.ReconcileProjectRequest{
		OperationID: current.ID, IdempotencyKey: current.ID + ":reconcile", ProjectID: project.ID,
		ProjectName: project.Name, Slug: project.Slug, ExpectedRevision: 0, NextRevision: 1,
		APIPort: apiPort, Configuration: configuration, Secrets: generated, RuntimeSecrets: runtimeSecrets,
	}
	if err := orchestrator.step(ctx, current.ID, "RECONCILE_RUNTIME", 35, func() error { _, err := orchestrator.provisioner.Reconcile(ctx, reconcile); return err }); err != nil {
		return orchestrator.rollback(ctx, project, current.ID, "RECONCILE_RUNTIME", err)
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

func (orchestrator *Orchestrator) allocateConfigurationPorts(ctx context.Context, projectID string, configuration *contracts.ProjectConfiguration) error {
	release := func(kind ports.Kind) error { return orchestrator.ports.Release(ctx, projectID, kind) }
	if !configuration.Services.Studio {
		if err := release(ports.KindStudio); err != nil {
			return err
		}
	}
	if !configuration.Services.DirectDB {
		if err := release(ports.KindDirectDB); err != nil {
			return err
		}
	}
	if !configuration.Services.Supavisor {
		if err := release(ports.KindPoolerTxn); err != nil {
			return err
		}
		if err := release(ports.KindPoolerSes); err != nil {
			return err
		}
	}
	kinds := []ports.Kind{ports.KindAPI}
	if configuration.Services.Studio {
		kinds = append(kinds, ports.KindStudio)
	}
	if configuration.Services.DirectDB {
		kinds = append(kinds, ports.KindDirectDB)
	}
	if configuration.Services.Supavisor {
		kinds = append(kinds, ports.KindPoolerTxn, ports.KindPoolerSes)
	}
	allocated, err := orchestrator.ports.ReserveMany(ctx, projectID, kinds)
	if err != nil {
		return err
	}
	configuration.Network.APIPort = allocated[ports.KindAPI]
	configuration.Network.StudioPort = allocated[ports.KindStudio]
	if configuration.Services.DirectDB {
		configuration.Database.DirectPort = true
		configuration.Database.DirectPortNumber = allocated[ports.KindDirectDB]
		configuration.Network.DirectDatabasePort = allocated[ports.KindDirectDB]
	} else {
		configuration.Database.DirectPort = false
		configuration.Database.DirectPortNumber = 0
		configuration.Network.DirectDatabasePort = 0
	}
	if configuration.Services.Supavisor {
		configuration.Pooler.TransactionPort = allocated[ports.KindPoolerTxn]
		configuration.Pooler.SessionPort = allocated[ports.KindPoolerSes]
		// The renderer derives the public pooler endpoint from sessionPort.
		configuration.Network.PoolerPort = 0
	} else {
		configuration.Network.PoolerPort = 0
	}
	return nil
}

func (orchestrator *Orchestrator) hydrateConfiguredSecrets(ctx context.Context, projectID string, cfg contracts.ProjectConfiguration) (map[string]string, error) {
	runtime := make(map[string]string)
	if orchestrator.cipher == nil {
		return runtime, errors.New("secret cipher is unavailable")
	}
	add := func(kind, runtimeKind string, required bool) error {
		if !required {
			return nil
		}
		envelope, err := orchestrator.store.GetSecret(ctx, projectID, kind)
		if errors.Is(err, store.ErrNotFound) {
			return errors.New("required configured secret is unavailable")
		}
		if err != nil {
			return err
		}
		plain, err := orchestrator.cipher.Decrypt(projectID, kind, envelope)
		if err != nil {
			return err
		}
		runtime[runtimeKind] = string(plain)
		return nil
	}
	if err := add("smtp.password", "smtp.password", cfg.Services.Auth && cfg.Auth.SMTP.Enabled && cfg.Auth.SMTP.PasswordSet); err != nil {
		return nil, err
	}
	if err := add("phone.secret", "phone.secret", cfg.Services.Auth && cfg.Auth.Phone.Enabled && cfg.Auth.Phone.SecretSet); err != nil {
		return nil, err
	}
	for provider, value := range cfg.Auth.OAuth {
		if err := add("oauth."+provider+".secret", "oauth."+provider+".secret", cfg.Services.Auth && value.Enabled && value.SecretSet); err != nil {
			return nil, err
		}
	}
	if err := add("storage.secretAccessKey", "storage.secretAccessKey", cfg.Services.Storage && cfg.Storage.SecretAccessKeySet); err != nil {
		return nil, err
	}
	for _, variable := range cfg.Functions.Variables {
		if err := add("functions."+variable.Name, "functions."+variable.Name, cfg.Services.Functions && variable.ValueSet); err != nil {
			return nil, err
		}
	}
	return runtime, nil
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

func (orchestrator *Orchestrator) persistSecrets(ctx context.Context, projectID string, generated contracts.ProjectSecrets, existing map[string]struct{}) error {
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
		if _, exists := existing[kind]; exists {
			continue
		}
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

type persistedProjectSecretSpec struct {
	kind string
	set  func(*contracts.ProjectSecrets, string)
	get  func(contracts.ProjectSecrets) string
}

func persistedProjectSecretSpecs() []persistedProjectSecretSpec {
	return []persistedProjectSecretSpec{
		{"database-password", func(secrets *contracts.ProjectSecrets, value string) { secrets.DatabasePassword = value }, func(secrets contracts.ProjectSecrets) string { return secrets.DatabasePassword }},
		{"jwt-secret", func(secrets *contracts.ProjectSecrets, value string) { secrets.JWTSecret = value }, func(secrets contracts.ProjectSecrets) string { return secrets.JWTSecret }},
		{"anon-key", func(secrets *contracts.ProjectSecrets, value string) { secrets.AnonKey = value }, func(secrets contracts.ProjectSecrets) string { return secrets.AnonKey }},
		{"service-role-key", func(secrets *contracts.ProjectSecrets, value string) { secrets.ServiceRoleKey = value }, func(secrets contracts.ProjectSecrets) string { return secrets.ServiceRoleKey }},
		{"dashboard-password", func(secrets *contracts.ProjectSecrets, value string) { secrets.DashboardPassword = value }, func(secrets contracts.ProjectSecrets) string { return secrets.DashboardPassword }},
		{"secret-key-base", func(secrets *contracts.ProjectSecrets, value string) { secrets.SecretKeyBase = value }, func(secrets contracts.ProjectSecrets) string { return secrets.SecretKeyBase }},
		{"vault-encryption-key", func(secrets *contracts.ProjectSecrets, value string) { secrets.VaultEncryptionKey = value }, func(secrets contracts.ProjectSecrets) string { return secrets.VaultEncryptionKey }},
		{"realtime-db-encryption-key", func(secrets *contracts.ProjectSecrets, value string) { secrets.RealtimeDBEncryptionKey = value }, func(secrets contracts.ProjectSecrets) string { return secrets.RealtimeDBEncryptionKey }},
		{"logflare-public-access-token", func(secrets *contracts.ProjectSecrets, value string) { secrets.LogflarePublicAccessToken = value }, func(secrets contracts.ProjectSecrets) string { return secrets.LogflarePublicAccessToken }},
		{"logflare-private-access-token", func(secrets *contracts.ProjectSecrets, value string) { secrets.LogflarePrivateAccessToken = value }, func(secrets contracts.ProjectSecrets) string { return secrets.LogflarePrivateAccessToken }},
		{"s3-protocol-access-key-id", func(secrets *contracts.ProjectSecrets, value string) { secrets.S3ProtocolAccessKeyID = value }, func(secrets contracts.ProjectSecrets) string { return secrets.S3ProtocolAccessKeyID }},
		{"s3-protocol-access-key-secret", func(secrets *contracts.ProjectSecrets, value string) { secrets.S3ProtocolAccessKeySecret = value }, func(secrets contracts.ProjectSecrets) string { return secrets.S3ProtocolAccessKeySecret }},
		{"pooler-tenant-id", func(secrets *contracts.ProjectSecrets, value string) { secrets.PoolerTenantID = value }, func(secrets contracts.ProjectSecrets) string { return secrets.PoolerTenantID }},
	}
}

// loadPersistedSecrets returns the stable project credentials created during
// the first installation. Reconcile/retry must reuse these credentials because
// PostgreSQL stores role passwords in its persistent data volume; generating a
// new POSTGRES_PASSWORD on every retry makes auth, rest, and storage unable to
// authenticate against an existing database.
func (orchestrator *Orchestrator) loadPersistedSecrets(ctx context.Context, projectID string) (contracts.ProjectSecrets, map[string]struct{}, error) {
	var result contracts.ProjectSecrets
	found := make(map[string]struct{})
	for _, spec := range persistedProjectSecretSpecs() {
		envelope, err := orchestrator.store.GetSecret(ctx, projectID, spec.kind)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return contracts.ProjectSecrets{}, nil, err
		}
		found[spec.kind] = struct{}{}
		plain, err := orchestrator.cipher.Decrypt(projectID, spec.kind, envelope)
		if err != nil {
			return contracts.ProjectSecrets{}, nil, fmt.Errorf("decrypt persisted secret %q: %w", spec.kind, err)
		}
		spec.set(&result, string(plain))
	}
	return result, found, nil
}

func (orchestrator *Orchestrator) applyConfiguredStudioPassword(ctx context.Context, projectID string, cfg contracts.ProjectConfiguration, secrets *contracts.ProjectSecrets) error {
	if !cfg.General.StudioPasswordSet {
		return nil
	}
	envelope, err := orchestrator.store.GetSecret(ctx, projectID, "studio.password")
	if errors.Is(err, store.ErrNotFound) {
		// Projects created before Studio credentials were separated stored this
		// value under dashboard-password. Keep that legacy credential usable.
		if secrets.DashboardPassword != "" {
			return nil
		}
		return errors.New("configured Studio password is unavailable")
	}
	if err != nil {
		return err
	}
	plain, err := orchestrator.cipher.Decrypt(projectID, "studio.password", envelope)
	if err != nil {
		return fmt.Errorf("decrypt configured Studio password: %w", err)
	}
	secrets.DashboardPassword = string(plain)
	return nil
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
		result = append(result, "supavisor")
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
