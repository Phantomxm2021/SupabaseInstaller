package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"supabase-manager/internal/contracts"
)

func TestHostResourcesRouteReturnsSnapshot(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHostRoutes(mux, hostResourcesProviderStub{})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/host/resources", nil))
	if response.Code != http.StatusOK || response.Body.String() == "" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type hostResourcesProviderStub struct{}

func (hostResourcesProviderStub) HostResources(context.Context) (contracts.HostResources, error) {
	return contracts.HostResources{CPUCores: 4}, nil
}
