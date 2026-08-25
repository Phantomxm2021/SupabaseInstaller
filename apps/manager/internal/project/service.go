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
	draft = NormalizeDraft(draft)
	if err := ValidateDraft(draft); err != nil {
		return Project{}, err
	}
	now := s.now()
	project := contracts.Project{
		ID: s.id(), Name: strings.TrimSpace(draft.Name), Slug: draft.Slug, Domain: draft.Domain,
		SiteURL: draft.SiteURL, Status: contracts.ProjectStatusDraft, Health: contracts.HealthUnknown,
		SupabaseVersion: draft.SupabaseVersion, Preset: draft.Preset, Services: draft.Configuration.Services,
		CreatedAt: now, UpdatedAt: now,
	}
	if project.ID == "" {
		return Project{}, fmt.Errorf("project ID generator returned an empty ID")
	}
	mutations, err := s.encryptConfigurationSecrets(project.ID, draft.Configuration)
	if err != nil {
		return Project{}, err
	}
	if err := s.store.CreateProjectWithSecrets(ctx, project, draft.Configuration, mutations); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Project{}, ErrConflict
		}
		return Project{}, err
	}
	return project, nil
}

func (s *Service) encryptConfigurationSecrets(projectID string, cfg contracts.ProjectConfiguration) ([]store.SecretMutation, error) {
	mutations := make([]store.SecretMutation, 0)
	add := func(kind string, input contracts.SecretInput) error {
		switch input.Action {
		case "", "retain":
			if input.Action == "retain" {
				return fmt.Errorf("%s cannot retain a secret during project creation", kind)
			}
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
		case "remove":
		default:
			return fmt.Errorf("%s: unknown secret action %q", kind, input.Action)
		}
		return nil
	}
	if err := add("smtp.password", cfg.Auth.SMTP.Password); err != nil {
		return nil, err
	}
	if err := add("phone.secret", cfg.Auth.Phone.Secret); err != nil {
		return nil, err
	}
	for provider, oauth := range cfg.Auth.OAuth {
		if err := add("oauth."+provider+".secret", oauth.Secret); err != nil {
			return nil, err
		}
	}
	if err := add("storage.secretAccessKey", cfg.Storage.SecretAccessKey); err != nil {
		return nil, err
	}
	for _, variable := range cfg.Functions.Variables {
		if err := add("functions."+variable.Name, variable.Value); err != nil {
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
