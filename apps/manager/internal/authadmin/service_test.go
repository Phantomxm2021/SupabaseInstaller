package authadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

func TestListUsersForMissingProjectReturnsServerTerminology(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, err := managersecrets.NewCipher(bytes.Repeat([]byte{6}, 32))
	if err != nil {
		t.Fatal(err)
	}
	_, err = New(database, cipher, nil, nil).ListUsers(context.Background(), "missing", "")
	var authErr *Error
	if !errors.As(err, &authErr) || authErr.Code != "PROJECT_NOT_FOUND" || authErr.Message != "Server was not found" {
		t.Fatalf("ListUsers() error = %#v, want PROJECT_NOT_FOUND with server terminology", err)
	}
}

func TestListUsersUsesProjectServiceRoleKeyWithoutReturningIt(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, err := managersecrets.NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	cfg := contracts.ProjectConfiguration{Revision: 1, General: contracts.GeneralConfig{Domain: "bee.example.com"}, Services: contracts.Services{Auth: true}, Network: contracts.NetworkConfig{APIPort: 8001}}
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: cfg.General.Domain, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	envelope, err := cipher.Encrypt(project.ID, "service-role-key", []byte("service-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.PutSecret(context.Background(), project.ID, "service-role-key", envelope); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/admin/users" || r.Header.Get("Authorization") != "Bearer service-key" || r.Header.Get("apikey") != "service-key" {
			t.Fatalf("unexpected GoTrue request: %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"users":[{"id":"u1","email":"a@example.com","identities":[{"provider":"email"}]}]}`))
	}))
	defer server.Close()
	service := New(database, cipher, server.Client(), func(contracts.NetworkConfig) string { return server.URL + "/auth/v1" })
	users, err := service.ListUsers(context.Background(), project.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Email != "a@example.com" {
		t.Fatalf("users = %#v", users)
	}
}

func TestCreateOAuthClientUsesProjectServiceRoleKey(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, err := managersecrets.NewCipher(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatal(err)
	}
	cfg := contracts.ProjectConfiguration{Revision: 1, General: contracts.GeneralConfig{Domain: "bee.example.com"}, Services: contracts.Services{Auth: true}, Network: contracts.NetworkConfig{APIPort: 8001}}
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: cfg.General.Domain, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	envelope, err := cipher.Encrypt(project.ID, "service-role-key", []byte("service-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.PutSecret(context.Background(), project.ID, "service-role-key", envelope); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/admin/oauth/clients" || r.Header.Get("Authorization") != "Bearer service-key" {
			t.Fatalf("unexpected GoTrue request: %s %s", r.Method, r.URL.Path)
		}
		var input CreateOAuthClientInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.Name != "Dashboard" || input.ClientType != "confidential" || len(input.RedirectURIs) != 1 {
			t.Fatalf("OAuth input = %#v", input)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"client_id":"client-1","name":"Dashboard","client_type":"confidential"}`))
	}))
	defer server.Close()
	service := New(database, cipher, server.Client(), func(contracts.NetworkConfig) string { return server.URL + "/auth/v1" })
	client, err := service.CreateOAuthClient(context.Background(), project.ID, CreateOAuthClientInput{Name: "Dashboard", RedirectURIs: []string{"https://app.example.test/callback"}, ClientType: "confidential", TokenEndpointAuthMethod: "client_secret_basic"})
	if err != nil {
		t.Fatal(err)
	}
	if client.ClientID != "client-1" || client.Name != "Dashboard" {
		t.Fatalf("client = %#v", client)
	}
}

func TestInviteUserUsesProjectServiceRoleKey(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cipher, err := managersecrets.NewCipher(bytes.Repeat([]byte{8}, 32))
	if err != nil {
		t.Fatal(err)
	}
	cfg := contracts.ProjectConfiguration{Revision: 1, General: contracts.GeneralConfig{Domain: "bee.example.com"}, Services: contracts.Services{Auth: true}, Network: contracts.NetworkConfig{APIPort: 8001}}
	project := contracts.Project{ID: "bee", Name: "Bee", Slug: "bee", Domain: cfg.General.Domain, Services: cfg.Services, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := database.CreateProject(context.Background(), project, cfg); err != nil {
		t.Fatal(err)
	}
	envelope, err := cipher.Encrypt(project.ID, "service-role-key", []byte("service-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.PutSecret(context.Background(), project.ID, "service-role-key", envelope); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/admin/invite" || r.Header.Get("Authorization") != "Bearer service-key" {
			t.Fatalf("unexpected GoTrue request: %s %s", r.Method, r.URL.Path)
		}
		var input InviteUserInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if input.Email != "invitee@example.test" {
			t.Fatalf("invite = %#v", input)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"invitee","email":"invitee@example.test"}`))
	}))
	defer server.Close()
	service := New(database, cipher, server.Client(), func(contracts.NetworkConfig) string { return server.URL + "/auth/v1" })
	user, err := service.InviteUser(context.Background(), project.ID, InviteUserInput{Email: "invitee@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "invitee@example.test" {
		t.Fatalf("user = %#v", user)
	}
}
