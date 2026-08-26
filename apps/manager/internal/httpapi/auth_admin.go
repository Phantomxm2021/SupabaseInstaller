package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"supabase-manager/apps/manager/internal/authadmin"
)

func (handlers projectHandlers) listUsers(response http.ResponseWriter, request *http.Request) {
	if handlers.options.AuthAdmin == nil {
		writeError(response, http.StatusServiceUnavailable, "AUTH_ADMIN_UNAVAILABLE", "Authentication administration is unavailable")
		return
	}
	users, err := handlers.options.AuthAdmin.ListUsers(request.Context(), request.PathValue("id"), request.URL.Query().Get("search"))
	if err != nil {
		writeAuthAdminError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"users": users})
}

func (handlers projectHandlers) createUser(response http.ResponseWriter, request *http.Request) {
	if handlers.options.AuthAdmin == nil {
		writeError(response, http.StatusServiceUnavailable, "AUTH_ADMIN_UNAVAILABLE", "Authentication administration is unavailable")
		return
	}
	var input authadmin.CreateUserInput
	if err := decodeJSON(response, request, &input); err != nil || strings.TrimSpace(input.Email) == "" || strings.TrimSpace(input.Password) == "" {
		writeError(response, http.StatusBadRequest, "INVALID_USER", "Email and password are required")
		return
	}
	user, err := handlers.options.AuthAdmin.CreateUser(request.Context(), request.PathValue("id"), input)
	if err != nil {
		writeAuthAdminError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, user)
}

func (handlers projectHandlers) inviteUser(response http.ResponseWriter, request *http.Request) {
	if handlers.options.AuthAdmin == nil {
		writeError(response, http.StatusServiceUnavailable, "AUTH_ADMIN_UNAVAILABLE", "Authentication administration is unavailable")
		return
	}
	var input authadmin.InviteUserInput
	if err := decodeJSON(response, request, &input); err != nil || strings.TrimSpace(input.Email) == "" {
		writeError(response, http.StatusBadRequest, "INVALID_INVITATION", "Email is required")
		return
	}
	user, err := handlers.options.AuthAdmin.InviteUser(request.Context(), request.PathValue("id"), input)
	if err != nil {
		writeAuthAdminError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, user)
}

func (handlers projectHandlers) listOAuthApps(response http.ResponseWriter, request *http.Request) {
	if handlers.options.AuthAdmin == nil {
		writeError(response, http.StatusServiceUnavailable, "AUTH_ADMIN_UNAVAILABLE", "Authentication administration is unavailable")
		return
	}
	clients, err := handlers.options.AuthAdmin.ListOAuthClients(request.Context(), request.PathValue("id"))
	if err != nil {
		writeAuthAdminError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{"clients": clients})
}

func (handlers projectHandlers) createOAuthApp(response http.ResponseWriter, request *http.Request) {
	if handlers.options.AuthAdmin == nil {
		writeError(response, http.StatusServiceUnavailable, "AUTH_ADMIN_UNAVAILABLE", "Authentication administration is unavailable")
		return
	}
	var input authadmin.CreateOAuthClientInput
	if err := decodeJSON(response, request, &input); err != nil || strings.TrimSpace(input.Name) == "" || len(input.RedirectURIs) == 0 {
		writeError(response, http.StatusBadRequest, "INVALID_OAUTH_APP", "Name and at least one redirect URI are required")
		return
	}
	client, err := handlers.options.AuthAdmin.CreateOAuthClient(request.Context(), request.PathValue("id"), input)
	if err != nil {
		writeAuthAdminError(response, err)
		return
	}
	writeJSON(response, http.StatusCreated, client)
}

func writeAuthAdminError(response http.ResponseWriter, err error) {
	var admin *authadmin.Error
	if errors.As(err, &admin) {
		writeError(response, admin.Status, admin.Code, admin.Message)
		return
	}
	writeError(response, http.StatusBadGateway, "AUTH_ADMIN_REQUEST_FAILED", "Authentication administration request failed")
}
