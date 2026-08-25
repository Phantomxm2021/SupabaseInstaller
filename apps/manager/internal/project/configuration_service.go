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

func (s *ConfigurationService) Save(ctx context.Context, projectID string, expected int64, cfg contracts.ProjectConfiguration) (store.ConfigurationSnapshot, error) {
	return s.save(ctx, projectID, expected, cfg)
}

func (s *ConfigurationService) Patch(ctx context.Context, projectID string, patch contracts.ConfigurationPatch) (store.ConfigurationSnapshot, error) {
	base, err := s.store.GetConfiguration(ctx, projectID)
	if err != nil {
		return store.ConfigurationSnapshot{}, err
	}
	cfg := base.Configuration
	if patch.Configuration != nil {
		cfg = *patch.Configuration
	}
	if patch.General != nil {
		cfg.General = *patch.General
	}
	if patch.Services != nil {
		cfg.Services = *patch.Services
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
	return s.save(ctx, projectID, patch.ExpectedRevision, cfg)
}

// Update and ApplyPatch are aliases kept for handlers that use command-style
// naming while the API's canonical operation is Patch.
func (s *ConfigurationService) Update(ctx context.Context, projectID string, patch contracts.ConfigurationPatch) (store.ConfigurationSnapshot, error) {
	return s.Patch(ctx, projectID, patch)
}

func (s *ConfigurationService) ApplyPatch(ctx context.Context, projectID string, patch contracts.ConfigurationPatch) (store.ConfigurationSnapshot, error) {
	return s.Patch(ctx, projectID, patch)
}

func (s *ConfigurationService) save(ctx context.Context, projectID string, expected int64, cfg contracts.ProjectConfiguration) (store.ConfigurationSnapshot, error) {
	normalizeRetainedSecrets(&cfg)
	if err := ValidateConfiguration(cfg); err != nil {
		return store.ConfigurationSnapshot{}, err
	}
	mutations, err := s.secretMutations(ctx, projectID, &cfg)
	if err != nil {
		return store.ConfigurationSnapshot{}, err
	}
	return s.store.SaveConfigurationWithSecrets(ctx, projectID, expected, cfg, s.now(), mutations)
}

// Snapshots intentionally omit secret actions. When a client submits an
// unchanged redacted snapshot, the corresponding set flag means retain.
func normalizeRetainedSecrets(cfg *contracts.ProjectConfiguration) {
	if cfg.Auth.SMTP.PasswordSet && cfg.Auth.SMTP.Password.Action == "" {
		cfg.Auth.SMTP.Password.Action = "retain"
	}
	if cfg.Auth.Phone.SecretSet && cfg.Auth.Phone.Secret.Action == "" {
		cfg.Auth.Phone.Secret.Action = "retain"
	}
	for provider, oauth := range cfg.Auth.OAuth {
		if oauth.SecretSet && oauth.Secret.Action == "" {
			oauth.Secret.Action = "retain"
			cfg.Auth.OAuth[provider] = oauth
		}
	}
	if cfg.Storage.SecretAccessKeySet && cfg.Storage.SecretAccessKey.Action == "" {
		cfg.Storage.SecretAccessKey.Action = "retain"
	}
	for index := range cfg.Functions.Variables {
		if cfg.Functions.Variables[index].ValueSet && cfg.Functions.Variables[index].Value.Action == "" {
			cfg.Functions.Variables[index].Value.Action = "retain"
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
	for index := range cfg.Functions.Variables {
		variable := &cfg.Functions.Variables[index]
		if err := add("functions."+variable.Name, &variable.Value, &variable.ValueSet); err != nil {
			return nil, err
		}
	}
	return mutations, nil
}
