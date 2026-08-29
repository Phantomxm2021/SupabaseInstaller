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

type serviceSource interface {
	ContainersForServices(context.Context, string, []string) ([]Container, error)
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

// HostResources delegates the read-only host snapshot to the Docker-backed
// source while keeping the existing project health interface unchanged.
func (i *Inspector) HostResources(ctx context.Context, projectRoot string) (contracts.HostResources, error) {
	source, ok := i.source.(interface {
		HostResources(context.Context, string) (contracts.HostResources, error)
	})
	if !ok {
		return contracts.HostResources{}, fmt.Errorf("host resource inspection is unavailable")
	}
	return source.HostResources(ctx, projectRoot)
}

// HostPortAvailable delegates host port inspection to the Docker-backed
// source. It is intentionally separate from project health inspection because
// it is used during Manager-side allocation before a project exists.
func (i *Inspector) HostPortAvailable(ctx context.Context, port int) (bool, error) {
	source, ok := i.source.(interface {
		HostPortAvailable(context.Context, int) (bool, error)
	})
	if !ok {
		return false, fmt.Errorf("host port inspection is unavailable")
	}
	return source.HostPortAvailable(ctx, port)
}

const projectProbeTimeout = 10 * time.Second

func NewInspector(source Source) *Inspector {
	return &Inspector{source: source, now: time.Now}
}

func (i *Inspector) Project(ctx context.Context, project ProjectRef) (Report, error) {
	probeCtx, cancel := context.WithTimeout(ctx, projectProbeTimeout)
	defer cancel()
	var containers []Container
	var err error
	if filtered, ok := i.source.(serviceSource); ok {
		containers, err = filtered.ContainersForServices(probeCtx, "supabase-manager-"+project.Slug, project.Enabled)
	} else {
		containers, err = i.source.Containers(probeCtx, "supabase-manager-"+project.Slug)
	}
	if err != nil {
		return Report{}, fmt.Errorf("list server containers: %w", err)
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
