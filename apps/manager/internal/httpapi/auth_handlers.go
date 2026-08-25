package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"supabase-manager/apps/manager/internal/auth"
	"supabase-manager/internal/contracts"
)

const SessionCookieName = "supabase_manager_session"

type AuthOptions struct {
	Service       *auth.Service
	PublicOrigin  string
	SecureCookies bool
}

type authHandlers struct {
	options AuthOptions
}

func RegisterAuthRoutes(mux *http.ServeMux, options AuthOptions) {
	handlers := authHandlers{options: options}
	mux.HandleFunc("GET /api/setup/status", handlers.setupStatus)
	mux.Handle("POST /api/setup", handlers.sameOrigin(http.HandlerFunc(handlers.setup)))
	mux.Handle("POST /api/session", handlers.sameOrigin(http.HandlerFunc(handlers.login)))
	mux.HandleFunc("GET /api/session", handlers.session)
	mux.Handle("DELETE /api/session", handlers.sameOrigin(http.HandlerFunc(handlers.logout)))
}

func (h authHandlers) setupStatus(response http.ResponseWriter, request *http.Request) {
	required, err := h.options.Service.SetupRequired(request.Context())
	if err != nil {
		writeError(response, http.StatusInternalServerError, "INTERNAL", "Unable to read setup status")
		return
	}
	writeJSON(response, http.StatusOK, contracts.SetupStatusResponse{Required: required})
}

func (h authHandlers) setup(response http.ResponseWriter, request *http.Request) {
	var input contracts.SetupRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_JSON", "Request body is invalid")
		return
	}
	result, err := h.options.Service.Bootstrap(request.Context(), input.Username, input.Password)
	switch {
	case errors.Is(err, auth.ErrAlreadyBootstrapped):
		writeError(response, http.StatusConflict, "ALREADY_BOOTSTRAPPED", "Manager setup has already completed")
	case err != nil:
		writeError(response, http.StatusUnprocessableEntity, "INVALID_SETUP", err.Error())
	default:
		writeJSON(response, http.StatusCreated, contracts.SetupResponse{RecoveryCodes: result.RecoveryCodes})
	}
}

func (h authHandlers) login(response http.ResponseWriter, request *http.Request) {
	var input contracts.LoginRequest
	if err := decodeJSON(response, request, &input); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_JSON", "Request body is invalid")
		return
	}
	session, err := h.options.Service.Login(request.Context(), input.Username, input.Password)
	if err != nil {
		writeError(response, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Username or password is incorrect")
		return
	}
	h.setSessionCookie(response, session.Token, session.ExpiresAt)
	identity, err := h.options.Service.Authenticate(request.Context(), session.Token)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "SESSION_FAILED", "Unable to create session")
		return
	}
	writeJSON(response, http.StatusCreated, contracts.SessionResponse{
		Username: identity.Username, MustChangePassword: identity.MustChangePassword, CSRFToken: session.CSRFToken,
	})
}

func (h authHandlers) session(response http.ResponseWriter, request *http.Request) {
	token, ok := sessionToken(request)
	if !ok {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
		return
	}
	identity, err := h.options.Service.Authenticate(request.Context(), token)
	if err != nil {
		writeError(response, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
		return
	}
	csrf, err := h.options.Service.RefreshCSRF(request.Context(), token)
	if err != nil {
		writeError(response, http.StatusInternalServerError, "SESSION_FAILED", "Unable to refresh session")
		return
	}
	writeJSON(response, http.StatusOK, contracts.SessionResponse{Username: identity.Username, MustChangePassword: identity.MustChangePassword, CSRFToken: csrf})
}

func (h authHandlers) logout(response http.ResponseWriter, request *http.Request) {
	token, ok := sessionToken(request)
	if ok {
		if err := h.options.Service.ValidateCSRF(request.Context(), token, request.Header.Get("X-CSRF-Token")); err != nil {
			writeError(response, http.StatusForbidden, "INVALID_CSRF", "CSRF validation failed")
			return
		}
		_ = h.options.Service.Logout(request.Context(), token)
	}
	h.setSessionCookie(response, "", time.Unix(0, 0))
	response.WriteHeader(http.StatusNoContent)
}

func (h authHandlers) setSessionCookie(response http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(response, &http.Cookie{
		Name: SessionCookieName, Value: token, Path: "/", Expires: expiresAt,
		MaxAge: maxAge(expiresAt), HttpOnly: true, Secure: h.options.SecureCookies, SameSite: http.SameSiteStrictMode,
	})
}

func maxAge(expiresAt time.Time) int {
	seconds := int(time.Until(expiresAt).Seconds())
	if seconds < 0 {
		return -1
	}
	return seconds
}

func sessionToken(request *http.Request) (string, bool) {
	cookie, err := request.Cookie(SessionCookieName)
	return func() string {
		if err != nil {
			return ""
		}
		return cookie.Value
	}(), err == nil && cookie.Value != ""
}

func decodeJSON(response http.ResponseWriter, request *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func writeJSON(response http.ResponseWriter, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}

func writeError(response http.ResponseWriter, status int, code, message string) {
	writeJSON(response, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
