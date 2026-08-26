package health

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDockerSourceListsComposeServicesAndInspectsHealth(t *testing.T) {
	client := &http.Client{Transport: dockerRoundTrip(func(request *http.Request) (*http.Response, error) {
		var body string
		switch request.URL.Path {
		case "/v1.41/containers/json":
			if !strings.Contains(request.URL.Query().Get("filters"), "supabase-manager-bee") {
				t.Errorf("filters = %q", request.URL.Query().Get("filters"))
			}
			body = `[{"Id":"container-1","Labels":{"com.docker.compose.service":"db"}}]`
		case "/v1.41/containers/container-1/json":
			body = `{"State":{"Status":"running","Health":{"Status":"healthy"}}}`
		default:
			t.Fatalf("unexpected Docker API path %s", request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	source := NewDockerSourceWithClient(client)

	containers, err := source.Containers(context.Background(), "supabase-manager-bee")
	if err != nil {
		t.Fatalf("Containers() error = %v", err)
	}
	if len(containers) != 1 || containers[0].Service != "db" || containers[0].Health != "healthy" {
		t.Fatalf("Containers() = %#v", containers)
	}
}

type dockerRoundTrip func(*http.Request) (*http.Response, error)

func (fn dockerRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
