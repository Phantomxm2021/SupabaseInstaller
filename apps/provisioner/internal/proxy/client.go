// Package proxy is the Provisioner's narrow client for the host-owned Nginx
// agent. It never handles paths, TLS files, or raw Nginx source.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Route struct {
	Slug          string `json:"slug"`
	Domain        string `json:"domain"`
	APIPort       int    `json:"apiPort"`
	StudioPort    int    `json:"studioPort"`
	StudioEnabled bool   `json:"studioEnabled"`
}

type Client interface {
	Apply(context.Context, Route) error
	Remove(context.Context, string) error
}

// DisabledClient retains the current manual Nginx deployment mode.
type DisabledClient struct{}

func (DisabledClient) Apply(context.Context, Route) error   { return nil }
func (DisabledClient) Remove(context.Context, string) error { return nil }

type ManagedClient struct {
	token  string
	client *http.Client
}

func NewManagedClient(socketPath, token string) *ManagedClient {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	return &ManagedClient{
		token: token,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

func (c *ManagedClient) Apply(ctx context.Context, route Route) error {
	return c.call(ctx, "/v1/sites/apply", route)
}

func (c *ManagedClient) Remove(ctx context.Context, slug string) error {
	return c.call(ctx, "/v1/sites/remove", struct {
		Slug string `json:"slug"`
	}{Slug: slug})
}

func (c *ManagedClient) call(ctx context.Context, path string, body any) error {
	if c.token == "" {
		return fmt.Errorf("managed nginx proxy token is empty")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode proxy request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://nginx-proxy-agent"+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("create proxy request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("call managed nginx proxy agent: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return nil
	}
	message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	return fmt.Errorf("managed nginx proxy agent returned %s: %s", response.Status, strings.TrimSpace(string(message)))
}
