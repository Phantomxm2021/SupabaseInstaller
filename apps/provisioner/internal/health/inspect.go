package health

import (
	"context"
	"fmt"
	"time"

	"supabase-manager/internal/contracts"
)

type Container struct {
	Service string
	State   string
	Health  string
}

type Source interface {
	Containers(ctx context.Context, composeProject string) ([]Container, error)
}

type ProjectRef struct {
	Slug    string
	Enabled []string
}

type Report struct {
	Health    contracts.HealthStatus
	Services  []contracts.ServiceState
	CheckedAt time.Time
}

type Inspector struct {
	source Source
	now    func() time.Time
}

func NewInspector(source Source) *Inspector {
	return &Inspector{source: source, now: time.Now}
}

func (i *Inspector) Project(ctx context.Context, project ProjectRef) (Report, error) {
	containers, err := i.source.Containers(ctx, "supabase-manager-"+project.Slug)
	if err != nil {
		return Report{}, fmt.Errorf("list project containers: %w", err)
	}
	byService := make(map[string]Container, len(containers))
	for _, container := range containers {
		byService[container.Service] = container
	}
	if len(containers) == 0 {
		return Report{Health: contracts.HealthStopped, CheckedAt: i.now()}, nil
	}
	projectHealth := contracts.HealthHealthy
	services := make([]contracts.ServiceState, 0, len(project.Enabled))
	for _, name := range project.Enabled {
		container, exists := byService[name]
		health := deriveContainerHealth(container, exists)
		services = append(services, contracts.ServiceState{Name: name, Health: health, Status: container.State})
		if health == contracts.HealthHealthy {
			continue
		}
		if isCore(name) {
			projectHealth = contracts.HealthUnhealthy
		} else if projectHealth != contracts.HealthUnhealthy {
			projectHealth = contracts.HealthDegraded
		}
	}
	return Report{Health: projectHealth, Services: services, CheckedAt: i.now()}, nil
}

func deriveContainerHealth(container Container, exists bool) contracts.HealthStatus {
	if !exists {
		return contracts.HealthUnhealthy
	}
	if container.State == "created" || container.State == "restarting" {
		return contracts.HealthStarting
	}
	if container.State != "running" {
		return contracts.HealthUnhealthy
	}
	switch container.Health {
	case "healthy", "":
		return contracts.HealthHealthy
	case "starting":
		return contracts.HealthStarting
	default:
		return contracts.HealthUnhealthy
	}
}

func isCore(service string) bool {
	switch service {
	case "db", "api-gw", "envoy", "kong", "auth", "rest", "meta", "studio":
		return true
	default:
		return false
	}
}
