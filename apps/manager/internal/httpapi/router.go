package httpapi

import (
	"net/http"

	"supabase-manager/apps/manager/internal/configuration"
	"supabase-manager/apps/manager/internal/operation"
)

type RouterOptions struct {
	Auth          AuthOptions
	Projects      ProjectOptions
	HostResources HostResourcesProvider
	Operations    *operation.Service
	Configuration *configuration.Orchestrator
	Config        *configuration.Orchestrator
}

func NewRouter(options RouterOptions) http.Handler {
	public := http.NewServeMux()
	RegisterAuthRoutes(public, options.Auth)

	protected := http.NewServeMux()
	RegisterProjectRoutes(protected, options.Projects)
	RegisterHostRoutes(protected, options.HostResources)
	RegisterOperationRoutes(protected, options.Operations)
	configManager := options.Configuration
	if configManager == nil {
		configManager = options.Config
	}
	if configManager == nil {
		configManager = options.Projects.Configuration
	}
	RegisterConfigurationRoutes(protected, ConfigurationOptions{Orchestrator: configManager, Auth: options.Auth.Service, PublicOrigin: options.Auth.PublicOrigin, Projects: options.Projects.Projects})
	public.Handle("/api/projects", ProtectAPI(options.Auth, protected))
	public.Handle("/api/projects/", ProtectAPI(options.Auth, protected))
	public.Handle("/api/host/", ProtectAPI(options.Auth, protected))
	public.Handle("/api/operations/", ProtectAPI(options.Auth, protected))
	return public
}
