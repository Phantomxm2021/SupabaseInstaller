package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

var ErrConflict = errors.New("project conflicts with an existing project")

type Service struct {
	store *store.Store
	id    func() string
	now   func() time.Time
}

func NewService(store *store.Store, id func() string, now func() time.Time) *Service {
	return &Service{store: store, id: id, now: now}
}

func (s *Service) Create(ctx context.Context, draft Draft) (Project, error) {
	if err := ValidateDraft(draft); err != nil {
		return Project{}, err
	}
	now := s.now()
	project := contracts.Project{
		ID: s.id(), Name: strings.TrimSpace(draft.Name), Slug: draft.Slug, Domain: draft.Domain,
		SiteURL: draft.SiteURL, Status: contracts.ProjectStatusDraft, Health: contracts.HealthUnknown,
		SupabaseVersion: draft.SupabaseVersion, Preset: draft.Preset, Services: draft.Services,
		CreatedAt: now, UpdatedAt: now,
	}
	if project.ID == "" {
		return Project{}, fmt.Errorf("project ID generator returned an empty ID")
	}
	if err := s.store.CreateProject(ctx, project); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Project{}, ErrConflict
		}
		return Project{}, err
	}
	return project, nil
}

func (s *Service) Get(ctx context.Context, id string) (Project, error) {
	return s.store.GetProject(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]Project, error) {
	return s.store.ListProjects(ctx)
}
