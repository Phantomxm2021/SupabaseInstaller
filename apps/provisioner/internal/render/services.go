package render

import "supabase-manager/internal/contracts"

import "fmt"

func validateServiceConfiguration(config contracts.ProjectConfiguration) error {
	s := config.Services
	if s.Functions && !s.Gateway {
		return fmt.Errorf("services.functions: requires services.gateway")
	}
	if s.Storage && (!s.Database || !s.REST) {
		return fmt.Errorf("services.storage: requires services.database and services.rest")
	}
	if s.Realtime && !s.Database {
		return fmt.Errorf("services.realtime: requires services.database")
	}
	if s.Supavisor && !s.Database {
		return fmt.Errorf("services.supavisor: requires services.database")
	}
	if s.Logs && (!s.Database || !s.Vector) {
		return fmt.Errorf("services.logs: requires services.database and services.vector")
	}
	if s.Vector && !s.Logs {
		return fmt.Errorf("services.vector: requires services.logs")
	}
	if s.Studio && !s.PostgresMeta {
		return fmt.Errorf("services.studio: requires services.postgresMeta")
	}
	if s.Imgproxy && !s.Storage {
		return fmt.Errorf("services.imgproxy: requires services.storage")
	}
	return nil
}

// selectServices maps product capabilities to names in the pinned Compose
// document. It intentionally does not retain unrelated template services.
func selectServices(config contracts.Services, available map[string]any) map[string]bool {
	selected := make(map[string]bool)
	add := func(name string, enabled bool) {
		if enabled && available[name] != nil {
			selected[name] = true
		}
	}
	add("db", config.Database)
	if config.Gateway {
		switch {
		case available["api-gw"] != nil:
			selected["api-gw"] = true
		case available["envoy"] != nil:
			selected["envoy"] = true
		case available["kong"] != nil:
			selected["kong"] = true
		}
	}
	add("auth", config.Auth)
	add("rest", config.REST)
	add("realtime", config.Realtime)
	add("storage", config.Storage)
	add("imgproxy", config.Imgproxy)
	add("meta", config.PostgresMeta)
	add("studio", config.Studio)
	add("functions", config.Functions)
	add("supavisor", config.Supavisor)
	add("analytics", config.Logs)
	add("logflare", config.Logs)
	add("vector", config.Vector)
	if config.Functions {
		add("deno-cache", true)
	}
	if config.Supavisor {
		add("db-config", true)
	}
	// The pinned template's feature services rely on these dependencies. They
	// are selected explicitly only when the capability needing them is on.
	return selected
}
