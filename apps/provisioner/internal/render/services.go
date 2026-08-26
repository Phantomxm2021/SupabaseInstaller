package render

import "supabase-manager/internal/contracts"

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
