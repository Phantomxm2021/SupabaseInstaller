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

	"supabase-manager/internal/contracts"
)

type Route struct {
	Slug               string `json:"slug"`
	Domain             string `json:"domain"`
	APIPort            int    `json:"apiPort"`
	StudioPort         int    `json:"studioPort"`
	StudioEnabled      bool   `json:"studioEnabled"`
	StudioUsername     string `json:"studioUsername"`
	StudioPassword     string `json:"studioPassword"`
	CertificateFile    string `json:"certificateFile,omitempty"`
	CertificateKeyFile string `json:"certificateKeyFile,omitempty"`
}

type Client interface {
	Apply(context.Context, Route) error
	Remove(context.Context, string) error
}

type CertificateStager interface {
	StageCertificate(context.Context, contracts.StageManagedTLSRequest) (contracts.StageManagedTLSResponse, error)
}

// DisabledClient retains the current manual Nginx deployment mode.
type DisabledClient struct{}

func (DisabledClient) Apply(context.Context, Route) error   { return nil }
func (DisabledClient) Remove(context.Context, string) error { return nil }
func (DisabledClient) StageCertificate(context.Context, contracts.StageManagedTLSRequest) (contracts.StageManagedTLSResponse, error) {
	return contracts.StageManagedTLSResponse{}, fmt.Errorf("managed nginx proxy is disabled")
}

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

func (c *ManagedClient) StageCertificate(ctx context.Context, input contracts.StageManagedTLSRequest) (contracts.StageManagedTLSResponse, error) {
	var output contracts.StageManagedTLSResponse
	if err := c.callJSON(ctx, "/v1/certificates/stage", input, &output); err != nil {
		return contracts.StageManagedTLSResponse{}, err
	}
	return output, nil
}

func (c *ManagedClient) call(ctx context.Context, path string, body any) error {
	return c.callJSON(ctx, path, body, nil)
}

func (c *ManagedClient) callJSON(ctx context.Context, path string, body any, output any) error {
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
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices && output != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(output); err != nil {
			return fmt.Errorf("decode managed nginx proxy response: %w", err)
		}
		return nil
	}
	message, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	return fmt.Errorf("managed nginx proxy agent returned %s: %s", response.Status, strings.TrimSpace(string(message)))
}
