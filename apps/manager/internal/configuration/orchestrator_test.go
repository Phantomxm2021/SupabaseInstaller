package configuration

import (
	"testing"

	"supabase-manager/internal/contracts"
)

func TestEnabledServicesUsesAuthoritativeSupavisorName(t *testing.T) {
	cfg := contracts.ProjectConfiguration{Services: contracts.Services{Database: true, Gateway: true, Supavisor: true, Logs: true, Vector: true}}
	got := enabledServices(cfg)
	for _, want := range []string{"db", "api-gw", "supavisor", "analytics", "vector"} {
		found := false
		for _, item := range got {
			if item == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing service %q in %v", want, got)
		}
	}
	for _, item := range got {
		if item == "pooler" {
			t.Fatal("legacy pooler service leaked into authoritative projection")
		}
	}
}

func TestSameServicesAcceptsConcreteGatewayProjection(t *testing.T) {
	if !sameServices([]string{"db", "envoy"}, []string{"db", "api-gw"}) {
		t.Fatal("gateway implementation should normalize to api-gw")
	}
}
