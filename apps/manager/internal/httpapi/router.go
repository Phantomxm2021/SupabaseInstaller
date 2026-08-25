package httpapi

import (
	"net/http"

	"supabase-manager/apps/manager/internal/operation"
)

type RouterOptions struct {
	Auth       AuthOptions
	Projects   ProjectOptions
	Operations *operation.Service
}

func NewRouter(options RouterOptions) http.Handler {
	public := http.NewServeMux()
	RegisterAuthRoutes(public, options.Auth)

	protected := http.NewServeMux()
	RegisterProjectRoutes(protected, options.Projects)
	RegisterOperationRoutes(protected, options.Operations)
	public.Handle("/api/projects", ProtectAPI(options.Auth, protected))
	public.Handle("/api/projects/", ProtectAPI(options.Auth, protected))
	public.Handle("/api/operations/", ProtectAPI(options.Auth, protected))
	return public
}
