package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

// ErrStaleConfiguration is returned when a patch was based on an obsolete
// aggregate revision. It aliases the store sentinel so callers can match it
// regardless of which layer performed the optimistic-concurrency check.
var ErrStaleConfiguration = store.ErrStaleConfiguration

// ConfigurationService is the boundary between typed project configuration
// and durable configuration/secret state. Store transactions provide the
// operation boundary; callers can compose this service with an operation
// coordinator explicitly when they need lifecycle events.
type ConfigurationService struct {
	store  *store.Store
	cipher *managersecrets.Cipher
	now    func() time.Time
}

func NewConfigurationService(database *store.Store, cipher *managersecrets.Cipher, now func() time.Time) *ConfigurationService {
	if now == nil {
		now = time.Now
	}
	return &ConfigurationService{store: database, cipher: cipher, now: now}
}

func (s *ConfigurationService) Get(ctx context.Context, projectID string) (store.ConfigurationSnapshot, error) {
	return s.store.GetConfiguration(ctx, projectID)
}

// ResetLegacyAuthConfigurations permanently replaces persisted Auth sections
// created before the current mailer configuration existed.
func (s *ConfigurationService) ResetLegacyAuthConfigurations(ctx context.Context) (int, error) {
	return s.store.ResetLegacyAuthConfigurations(ctx, DefaultConfiguration(contracts.PresetLightweight).Auth)
}

// MigrateFailedPostgreSQL15Configurations replaces stale failed-draft
// configuration with the current PostgreSQL 17-only runtime. It is a one-time
// data migration; healthy PostgreSQL 15 projects are deliberately untouched.
func (s *ConfigurationService) MigrateFailedPostgreSQL15Configurations(ctx context.Context) (int, error) {
	return s.store.MigrateFailedPostgreSQL15Configurations(ctx)
}

// GetDesired returns the latest aggregate, including an unapplied revision.
// It is the correct merge base for durable operations queued behind runtime
// reconciliation.
func (s *ConfigurationService) GetDesired(ctx context.Context, projectID string) (store.ConfigurationSnapshot, error) {
	return s.store.GetDesiredConfiguration(ctx, projectID)
}

// PreparePatch computes and validates a mutation without writing. Queueing
// uses this to place the exact encrypted mutation and command payload in the
// same SQLite transaction as the revision and operation row.
func (s *ConfigurationService) PreparePatch(ctx context.Context, projectID string, patch contracts.ConfigurationPatch) (contracts.ProjectConfiguration, error) {
	base, err := s.store.GetDesiredConfiguration(ctx, projectID)
	if err != nil {
		return contracts.ProjectConfiguration{}, err
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return contracts.ProjectConfiguration{}, err
	}
	cfg := base.Configuration
	if patch.Configuration != nil {
		cfg = *patch.Configuration
		// Pre-domain-model snapshots did not always persist General in the
		// aggregate. Preserve the stored projection while they are edited; an
		// explicit General patch still requires a base domain.
		if cfg.General.SiteURL == "" {
			cfg.General.SiteURL = project.SiteURL
		}
		if cfg.General.Domain == "" {
			cfg.General.Domain = project.Domain
		}
		if cfg.General.SupabaseVersion == "" {
			cfg.General.SupabaseVersion = project.SupabaseVersion
		}
	}
	if patch.General != nil {
		cfg.General = *patch.General
	}
	if patch.Services != nil {
		cfg.Services = *patch.Services
		// Service switches are authoritative. Keep legacy subsection booleans
		// coherent without requiring the Services request to carry redacted
		// Auth/Database secret leaves.
		cfg.Auth.Enabled = cfg.Services.Auth
		cfg.Database.DirectPort = cfg.Services.DirectDB
	}
	if patch.Auth != nil {
		cfg.Auth = *patch.Auth
	}
	if patch.Storage != nil {
		cfg.Storage = *patch.Storage
	}
	if patch.Realtime != nil {
		cfg.Realtime = *patch.Realtime
	}
	if patch.Functions != nil {
		cfg.Functions = *patch.Functions
	}
	if patch.Database != nil {
		cfg.Database = *patch.Database
	}
	if patch.Pooler != nil {
		cfg.Pooler = *patch.Pooler
	}
	if patch.Network != nil {
		cfg.Network = *patch.Network
	}
	if err := requireExplicitSecretActionsForPatch(patch); err != nil {
		return contracts.ProjectConfiguration{}, err
	}
	// GetDesiredConfiguration is redacted by design. Restore retain markers
	// only for secret leaves from sections omitted by this patch so aggregate
	// validation can inspect the stored secret without treating an untouched
	// redacted marker as an incoming command.
	restoreUntouchedSecretActions(&cfg, &base.Configuration, patch)
	if err := NormalizeProjectAddress(project.Slug, &cfg.General); err != nil {
		return contracts.ProjectConfiguration{}, err
	}
	legacyCaddy := base.Configuration.Network.HTTPSMode == contracts.HTTPSModeCaddy && cfg.Network.HTTPSMode == contracts.HTTPSModeCaddy
	if err := validateConfiguration(cfg, legacyCaddy, false); err != nil {
		return contracts.ProjectConfiguration{}, err
	}
	return cfg, nil
}

func (s *ConfigurationService) PrepareSecretMutations(ctx context.Context, projectID string, cfg *contracts.ProjectConfiguration) ([]store.SecretMutation, error) {
	return s.secretMutations(ctx, projectID, cfg)
}

func requireExplicitSecretActionsForPatch(patch contracts.ConfigurationPatch) error {
	if patch.Configuration != nil {
		return requireExplicitSecretActions(*patch.Configuration)
	}
	if patch.Auth != nil {
		if err := requireExplicitAuthSecretActions(patch.Auth); err != nil {
			return err
		}
	}
	if patch.Storage != nil && patch.Storage.SecretAccessKeySet && patch.Storage.SecretAccessKey.Action == "" {
		return fmt.Errorf("storage.secretAccessKey requires explicit retain, remove, or replace action")
	}
	if patch.Functions != nil {
		for index, variable := range patch.Functions.Variables {
			if variable.ValueSet && variable.Value.Action == "" {
				return fmt.Errorf("functions.variables[%d].value requires explicit retain, remove, or replace action", index)
			}
		}
	}
	return nil
}

func requireExplicitSecretActions(cfg contracts.ProjectConfiguration) error {
	if cfg.General.StudioPasswordSet && cfg.General.StudioPassword.Action == "" {
		return fmt.Errorf("general.studioPassword requires explicit retain, remove, or replace action")
	}
	if err := requireExplicitAuthSecretActions(&cfg.Auth); err != nil {
		return err
	}
	if cfg.Storage.SecretAccessKeySet && cfg.Storage.SecretAccessKey.Action == "" {
		return fmt.Errorf("storage.secretAccessKey requires explicit retain, remove, or replace action")
	}
	for index, variable := range cfg.Functions.Variables {
		if variable.ValueSet && variable.Value.Action == "" {
			return fmt.Errorf("functions.variables[%d].value requires explicit retain, remove, or replace action", index)
		}
	}
	return nil
}

func requireExplicitAuthSecretActions(auth *contracts.AuthConfig) error {
	if auth.SMTP.PasswordSet && auth.SMTP.Password.Action == "" {
		return fmt.Errorf("auth.smtp.password requires explicit retain, remove, or replace action")
	}
	if auth.Phone.SecretSet && auth.Phone.Secret.Action == "" {
		return fmt.Errorf("auth.phone.secret requires explicit retain, remove, or replace action")
	}
	for provider, oauth := range auth.OAuth {
		if oauth.SecretSet && oauth.Secret.Action == "" {
			return fmt.Errorf("auth.oauth.%s.secret requires explicit retain, remove, or replace action", provider)
		}
	}
	return nil
}

func restoreUntouchedSecretActions(cfg, base *contracts.ProjectConfiguration, patch contracts.ConfigurationPatch) {
	allIncoming := patch.Configuration != nil
	if base.General.StudioPasswordSet && cfg.General.StudioPassword.Action == "" {
		cfg.General.StudioPassword.Action = "retain"
	}
	if !allIncoming && patch.Auth == nil {
		if base.Auth.SMTP.PasswordSet && cfg.Auth.SMTP.Password.Action == "" {
			cfg.Auth.SMTP.Password.Action = "retain"
		}
		if base.Auth.Phone.SecretSet && cfg.Auth.Phone.Secret.Action == "" {
			cfg.Auth.Phone.Secret.Action = "retain"
		}
		for provider, oauth := range cfg.Auth.OAuth {
			if baseOAuth, exists := base.Auth.OAuth[provider]; exists && baseOAuth.SecretSet && oauth.Secret.Action == "" {
				oauth.Secret.Action = "retain"
				cfg.Auth.OAuth[provider] = oauth
			}
		}
	}
	if !allIncoming && patch.Storage == nil && base.Storage.SecretAccessKeySet && cfg.Storage.SecretAccessKey.Action == "" {
		cfg.Storage.SecretAccessKey.Action = "retain"
	}
	if !allIncoming && patch.Functions == nil {
		for index := range cfg.Functions.Variables {
			if index < len(base.Functions.Variables) && base.Functions.Variables[index].ValueSet && cfg.Functions.Variables[index].Value.Action == "" {
				cfg.Functions.Variables[index].Value.Action = "retain"
			}
		}
	}
}

func (s *ConfigurationService) secretMutations(ctx context.Context, projectID string, cfg *contracts.ProjectConfiguration) ([]store.SecretMutation, error) {
	mutations := make([]store.SecretMutation, 0)
	add := func(kind string, input *contracts.SecretInput, set *bool) error {
		action := strings.ToLower(strings.TrimSpace(input.Action))
		if action == "" {
			input.Value = ""
			return nil
		}
		switch action {
		case "retain":
			if _, err := s.store.GetSecret(ctx, projectID, kind); err != nil {
				return fmt.Errorf("retain %s: %w", kind, err)
			}
		case "replace":
			if input.Value == "" {
				return fmt.Errorf("%s: replace requires a value", kind)
			}
			if s.cipher == nil {
				return errors.New("secret cipher is required for replacement")
			}
			envelope, err := s.cipher.Encrypt(projectID, kind, []byte(input.Value))
			if err != nil {
				return fmt.Errorf("encrypt %s: %w", kind, err)
			}
			mutations = append(mutations, store.SecretMutation{Kind: kind, Envelope: envelope})
			*set = true
		case "remove":
			mutations = append(mutations, store.SecretMutation{Kind: kind, Delete: true})
			*set = false
		default:
			return fmt.Errorf("%s: unknown secret action %q", kind, input.Action)
		}
		input.Value = ""
		input.Action = ""
		return nil
	}
	if err := add("studio.password", &cfg.General.StudioPassword, &cfg.General.StudioPasswordSet); err != nil {
		return nil, err
	}
	if err := add("smtp.password", &cfg.Auth.SMTP.Password, &cfg.Auth.SMTP.PasswordSet); err != nil {
		return nil, err
	}
	if err := add("phone.secret", &cfg.Auth.Phone.Secret, &cfg.Auth.Phone.SecretSet); err != nil {
		return nil, err
	}
	for provider, oauth := range cfg.Auth.OAuth {
		if err := add("oauth."+provider+".secret", &oauth.Secret, &oauth.SecretSet); err != nil {
			return nil, err
		}
		cfg.Auth.OAuth[provider] = oauth
	}
	if err := add("storage.secretAccessKey", &cfg.Storage.SecretAccessKey, &cfg.Storage.SecretAccessKeySet); err != nil {
		return nil, err
	}
	keptVariables := cfg.Functions.Variables[:0]
	for index := range cfg.Functions.Variables {
		variable := &cfg.Functions.Variables[index]
		removeVariable := strings.EqualFold(strings.TrimSpace(variable.Value.Action), "remove")
		if err := add("functions."+variable.Name, &variable.Value, &variable.ValueSet); err != nil {
			return nil, err
		}
		if !removeVariable {
			keptVariables = append(keptVariables, *variable)
		}
	}
	cfg.Functions.Variables = keptVariables
	return mutations, nil
}
