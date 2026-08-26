package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const dockerAPIVersion = "v1.41"

type DockerSource struct {
	client *http.Client
}

func NewDockerSource(dockerHost string) (*DockerSource, error) {
	const unixPrefix = "unix://"
	if !strings.HasPrefix(dockerHost, unixPrefix) {
		return nil, fmt.Errorf("only a local Unix Docker Socket is supported")
	}
	socketPath := strings.TrimPrefix(dockerHost, unixPrefix)
	transport := &http.Transport{
		// Docker Desktop's Unix-socket proxy can leave persistent response
		// bodies open while Compose is recreating containers. Every inspection
		// is independent, so avoid reusing those connections across health
		// probes and rollback boundaries.
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", socketPath)
		},
	}
	return NewDockerSourceWithClient(&http.Client{Transport: transport, Timeout: 15 * time.Second}), nil
}

func NewDockerSourceWithClient(client *http.Client) *DockerSource {
	return &DockerSource{client: client}
}

func (source *DockerSource) Containers(ctx context.Context, composeProject string) ([]Container, error) {
	return source.containers(ctx, composeProject, nil)
}

// ContainersForServices limits Docker inspection to services participating in
// the requested health probe. This keeps rollback probes bounded to auth (or
// another affected service) instead of inspecting the whole project.
func (source *DockerSource) ContainersForServices(ctx context.Context, composeProject string, enabled []string) ([]Container, error) {
	allowed := make(map[string]struct{}, len(enabled))
	for _, service := range enabled {
		allowed[service] = struct{}{}
	}
	return source.containers(ctx, composeProject, allowed)
}

func (source *DockerSource) containers(ctx context.Context, composeProject string, allowed map[string]struct{}) ([]Container, error) {
	filters, _ := json.Marshal(map[string][]string{"label": {"com.docker.compose.project=" + composeProject}})
	query := url.Values{"all": {"1"}, "filters": {string(filters)}}
	var summaries []struct {
		ID     string            `json:"Id"`
		Labels map[string]string `json:"Labels"`
	}
	if err := source.get(ctx, "/"+dockerAPIVersion+"/containers/json?"+query.Encode(), &summaries); err != nil {
		return nil, err
	}
	containers := make([]Container, 0, len(summaries))
	for _, summary := range summaries {
		if allowed != nil {
			if _, ok := allowed[summary.Labels["com.docker.compose.service"]]; !ok {
				continue
			}
		}
		var inspected struct {
			State struct {
				Status string `json:"Status"`
				Health *struct {
					Status string `json:"Status"`
				} `json:"Health"`
			} `json:"State"`
		}
		if err := source.get(ctx, "/"+dockerAPIVersion+"/containers/"+url.PathEscape(summary.ID)+"/json", &inspected); err != nil {
			return nil, err
		}
		health := ""
		if inspected.State.Health != nil {
			health = inspected.State.Health.Status
		}
		containers = append(containers, Container{Service: summary.Labels["com.docker.compose.service"], State: inspected.State.Status, Health: health})
	}
	return containers, nil
}

func (source *DockerSource) get(ctx context.Context, path string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return fmt.Errorf("create Docker API request: %w", err)
	}
	response, err := source.client.Do(request)
	if err != nil {
		return fmt.Errorf("call Docker API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Docker API returned %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("decode Docker API response: %w", err)
	}
	return nil
}
