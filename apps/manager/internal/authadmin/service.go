// Package authadmin proxies narrowly scoped project GoTrue Admin APIs.
package authadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type User struct {
	ID           string         `json:"id"`
	Email        string         `json:"email"`
	Phone        string         `json:"phone"`
	CreatedAt    string         `json:"created_at"`
	UserMetadata map[string]any `json:"user_metadata"`
	Identities   []Identity     `json:"identities"`
}

type Identity struct {
	Provider string `json:"provider"`
}

type OAuthClient struct {
	ClientID                string   `json:"client_id"`
	Name                    string   `json:"name"`
	RedirectURIs            []string `json:"redirect_uris"`
	ClientType              string   `json:"client_type"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	CreatedAt               string   `json:"created_at"`
}

type CreateUserInput struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	EmailConfirm bool   `json:"email_confirm"`
}

type InviteUserInput struct {
	Email string `json:"email"`
}

type CreateOAuthClientInput struct {
	Name                    string   `json:"name"`
	RedirectURIs            []string `json:"redirect_uris"`
	ClientType              string   `json:"client_type"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

type GatewayURL func(contracts.NetworkConfig) string

type Service struct {
	store   *store.Store
	cipher  *managersecrets.Cipher
	http    *http.Client
	gateway GatewayURL
}

func New(database *store.Store, cipher *managersecrets.Cipher, client *http.Client, gateway GatewayURL) *Service {
	if client == nil {
		client = http.DefaultClient
	}
	if gateway == nil {
		gateway = GatewayAtHost("host.docker.internal")
	}
	return &Service{store: database, cipher: cipher, http: client, gateway: gateway}
}

func GatewayAtHost(host string) GatewayURL {
	return func(network contracts.NetworkConfig) string {
		return "http://" + host + ":" + strconv.Itoa(network.APIPort) + "/auth/v1"
	}
}

func (s *Service) ListUsers(ctx context.Context, projectID, search string) ([]User, error) {
	query := url.Values{"page": {"1"}, "per_page": {"100"}}
	var response struct {
		Users []User `json:"users"`
	}
	if err := s.request(ctx, projectID, http.MethodGet, "/admin/users?"+query.Encode(), nil, &response); err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return response.Users, nil
	}
	filtered := make([]User, 0, len(response.Users))
	for _, user := range response.Users {
		if strings.Contains(strings.ToLower(user.Email), needle) || strings.Contains(strings.ToLower(user.Phone), needle) {
			filtered = append(filtered, user)
		}
	}
	return filtered, nil
}

func (s *Service) CreateUser(ctx context.Context, projectID string, input CreateUserInput) (User, error) {
	var user User
	err := s.request(ctx, projectID, http.MethodPost, "/admin/users", input, &user)
	return user, err
}

func (s *Service) InviteUser(ctx context.Context, projectID string, input InviteUserInput) (User, error) {
	var user User
	err := s.request(ctx, projectID, http.MethodPost, "/admin/invite", input, &user)
	return user, err
}

func (s *Service) ListOAuthClients(ctx context.Context, projectID string) ([]OAuthClient, error) {
	var response struct {
		Clients []OAuthClient `json:"clients"`
	}
	if err := s.request(ctx, projectID, http.MethodGet, "/admin/oauth/clients?page=1&per_page=100", nil, &response); err != nil {
		return nil, err
	}
	return response.Clients, nil
}

func (s *Service) CreateOAuthClient(ctx context.Context, projectID string, input CreateOAuthClientInput) (OAuthClient, error) {
	var client OAuthClient
	err := s.request(ctx, projectID, http.MethodPost, "/admin/oauth/clients", input, &client)
	return client, err
}

type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func (s *Service) request(ctx context.Context, projectID, method, path string, input, output any) error {
	if s.store == nil || s.cipher == nil {
		return &Error{Status: http.StatusServiceUnavailable, Code: "AUTH_ADMIN_UNAVAILABLE", Message: "Authentication administration is unavailable"}
	}
	snapshot, err := s.store.GetConfiguration(ctx, projectID)
	if errors.Is(err, store.ErrNotFound) {
		return &Error{Status: http.StatusNotFound, Code: "PROJECT_NOT_FOUND", Message: "Server was not found"}
	}
	if err != nil {
		return &Error{Status: http.StatusInternalServerError, Code: "CONFIGURATION_GET_FAILED", Message: "Unable to read server configuration"}
	}
	if !snapshot.Configuration.Services.Auth || snapshot.Configuration.Network.APIPort == 0 {
		return &Error{Status: http.StatusServiceUnavailable, Code: "AUTH_UNAVAILABLE", Message: "Authentication is not running for this server"}
	}
	envelope, err := s.store.GetSecret(ctx, projectID, "service-role-key")
	if err != nil {
		return &Error{Status: http.StatusServiceUnavailable, Code: "AUTH_ADMIN_UNAVAILABLE", Message: "Server administrator credentials are unavailable"}
	}
	key, err := s.cipher.Decrypt(projectID, "service-role-key", envelope)
	if err != nil {
		return &Error{Status: http.StatusInternalServerError, Code: "AUTH_ADMIN_UNAVAILABLE", Message: "Server administrator credentials are unavailable"}
	}
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return &Error{Status: http.StatusBadRequest, Code: "INVALID_REQUEST", Message: "Authentication request is invalid"}
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.gateway(snapshot.Configuration.Network), "/")+path, body)
	if err != nil {
		return &Error{Status: http.StatusInternalServerError, Code: "AUTH_ADMIN_UNAVAILABLE", Message: "Authentication service is unavailable"}
	}
	request.Header.Set("Authorization", "Bearer "+string(key))
	request.Header.Set("apikey", string(key))
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := s.http.Do(request)
	if err != nil {
		return &Error{Status: http.StatusServiceUnavailable, Code: "AUTH_UNAVAILABLE", Message: "Authentication service is unavailable"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		code, message := "AUTH_ADMIN_REQUEST_FAILED", "Authentication administration request failed"
		if strings.Contains(path, "/admin/oauth/") && response.StatusCode == http.StatusNotFound {
			code, message = "OAUTH_SERVER_DISABLED", "OAuth Server is disabled for this server"
		}
		return &Error{Status: response.StatusCode, Code: code, Message: message}
	}
	if output != nil && response.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(output); err != nil {
			return fmt.Errorf("decode GoTrue admin response: %w", err)
		}
	}
	return nil
}
