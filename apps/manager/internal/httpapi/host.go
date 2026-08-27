package httpapi

import (
	"context"
	"net/http"

	"supabase-manager/internal/contracts"
)

type HostResourcesProvider interface {
	HostResources(context.Context) (contracts.HostResources, error)
}

func RegisterHostRoutes(mux *http.ServeMux, provider HostResourcesProvider) {
	mux.HandleFunc("GET /api/host/resources", func(response http.ResponseWriter, request *http.Request) {
		if provider == nil {
			writeError(response, http.StatusServiceUnavailable, "HOST_RESOURCES_UNAVAILABLE", "Host resource inspection is unavailable")
			return
		}
		resources, err := provider.HostResources(request.Context())
		if err != nil {
			writeError(response, http.StatusServiceUnavailable, "HOST_RESOURCES_UNAVAILABLE", "Host resource inspection failed")
			return
		}
		writeJSON(response, http.StatusOK, resources)
	})
}
