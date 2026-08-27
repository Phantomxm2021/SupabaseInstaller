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

func TestDockerSourceChecksPublishedHostPort(t *testing.T) {
	client := &http.Client{Transport: dockerRoundTrip(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/v1.41/containers/json" || request.URL.RawQuery != "" {
			t.Fatalf("unexpected Docker API request: %s?%s", request.URL.Path, request.URL.RawQuery)
		}
		body := `[{"Ports":[{"IP":"127.0.0.1","PrivatePort":3000,"PublicPort":8001,"Type":"tcp"},{"PrivatePort":5432,"Type":"tcp"}]}]`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	source := NewDockerSourceWithClient(client)

	available, err := source.HostPortAvailable(context.Background(), 8001)
	if err != nil {
		t.Fatalf("HostPortAvailable(8001) error = %v", err)
	}
	if available {
		t.Fatal("HostPortAvailable(8001) = true, want occupied")
	}
	available, err = source.HostPortAvailable(context.Background(), 8002)
	if err != nil {
		t.Fatalf("HostPortAvailable(8002) error = %v", err)
	}
	if !available {
		t.Fatal("HostPortAvailable(8002) = false, want available")
	}
}

type dockerRoundTrip func(*http.Request) (*http.Response, error)

func (fn dockerRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
