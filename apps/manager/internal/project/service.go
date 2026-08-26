package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

var ErrConflict = errors.New("project conflicts with an existing project")

type Service struct {
	store  *store.Store
	id     func() string
	now    func() time.Time
	cipher *managersecrets.Cipher
}

func NewService(store *store.Store, id func() string, now func() time.Time) *Service {
	return &Service{store: store, id: id, now: now}
}

func NewServiceWithCipher(store *store.Store, id func() string, now func() time.Time, cipher *managersecrets.Cipher) *Service {
	return &Service{store: store, id: id, now: now, cipher: cipher}
}

func (s *Service) Create(ctx context.Context, draft Draft) (Project, error) {
	if err := ValidateDraft(draft); err != nil {
		return Project{}, err
	}
	configuration := draft.Configuration
	now := s.now()
	project := contracts.Project{
		ID: s.id(), Name: strings.TrimSpace(draft.Name), Slug: draft.Slug, Domain: configuration.General.Domain,
		SiteURL: configuration.General.SiteURL, Status: contracts.ProjectStatusDraft, Health: contracts.HealthUnknown,
		SupabaseVersion: configuration.General.SupabaseVersion, Preset: draft.Preset, Services: configuration.Services, ConfigurationRevision: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if project.ID == "" {
		return Project{}, fmt.Errorf("project ID generator returned an empty ID")
	}
	persistedConfiguration, err := cloneConfiguration(configuration)
	if err != nil {
		return Project{}, err
	}
	mutations, err := s.encryptConfigurationSecrets(project.ID, &persistedConfiguration)
	if err != nil {
		return Project{}, err
	}
	if err := s.store.CreateProjectWithSecrets(ctx, project, persistedConfiguration, mutations); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Project{}, ErrConflict
		}
		return Project{}, err
	}
	return project, nil
}

func cloneConfiguration(cfg contracts.ProjectConfiguration) (contracts.ProjectConfiguration, error) {
	payload, err := json.Marshal(cfg)
	if err != nil {
		return contracts.ProjectConfiguration{}, fmt.Errorf("clone configuration: %w", err)
	}
	var clone contracts.ProjectConfiguration
	if err := json.Unmarshal(payload, &clone); err != nil {
		return contracts.ProjectConfiguration{}, fmt.Errorf("clone configuration: %w", err)
	}
	return clone, nil
}

func (s *Service) encryptConfigurationSecrets(projectID string, cfg *contracts.ProjectConfiguration) ([]store.SecretMutation, error) {
	mutations := make([]store.SecretMutation, 0)
	add := func(kind string, input *contracts.SecretInput, set *bool) error {
		// Secret presence is server-derived; client-provided markers are never
		// trusted during initial creation.
		*set = false
		switch input.Action {
		case "":
		case "replace":
			if input.Value == "" {
				return fmt.Errorf("%s: replace requires a value", kind)
			}
			if s.cipher == nil {
				return fmt.Errorf("%s: secret cipher is required for replacement", kind)
			}
			envelope, err := s.cipher.Encrypt(projectID, kind, []byte(input.Value))
			if err != nil {
				return fmt.Errorf("encrypt %s: %w", kind, err)
			}
			mutations = append(mutations, store.SecretMutation{Kind: kind, Envelope: envelope})
			*set = true
		case "retain", "remove":
			return fmt.Errorf("%s cannot use %s during project creation", kind, input.Action)
		default:
			return fmt.Errorf("%s: unknown secret action %q", kind, input.Action)
		}
		input.Action = ""
		input.Value = ""
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

func (s *Service) Get(ctx context.Context, id string) (Project, error) {
	return s.store.GetProject(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]Project, error) {
	return s.store.ListProjects(ctx)
}
