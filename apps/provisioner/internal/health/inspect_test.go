package health

import (
	"context"
	"testing"

	"supabase-manager/internal/contracts"
)

func TestProjectIsDegradedWhenOptionalServiceIsUnhealthy(t *testing.T) {
	source := staticSource{containers: []Container{
		{Service: "db", State: "running", Health: "healthy"},
		{Service: "api-gw", State: "running", Health: "healthy"},
		{Service: "auth", State: "running", Health: "healthy"},
		{Service: "realtime", State: "running", Health: "unhealthy"},
	}}
	inspector := NewInspector(source)
	report, err := inspector.Project(context.Background(), ProjectRef{Slug: "bee", Enabled: []string{"db", "api-gw", "auth", "realtime"}})
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	if report.Health != contracts.HealthDegraded {
		t.Fatalf("project health = %s, want DEGRADED", report.Health)
	}
}

func TestProjectIsUnhealthyWhenDatabaseIsUnhealthy(t *testing.T) {
	source := staticSource{containers: []Container{{Service: "db", State: "running", Health: "unhealthy"}, {Service: "api-gw", State: "running", Health: "healthy"}}}
	report, _ := NewInspector(source).Project(context.Background(), ProjectRef{Slug: "bee", Enabled: []string{"db", "api-gw"}})
	if report.Health != contracts.HealthUnhealthy {
		t.Fatalf("project health = %s, want UNHEALTHY", report.Health)
	}
}

type staticSource struct{ containers []Container }

func (source staticSource) Containers(context.Context, string) ([]Container, error) {
	return source.containers, nil
}
