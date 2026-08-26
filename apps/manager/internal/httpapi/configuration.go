package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"supabase-manager/apps/manager/internal/auth"
	"supabase-manager/apps/manager/internal/configuration"
	"supabase-manager/apps/manager/internal/project"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/internal/contracts"
)

type ConfigurationOptions struct {
	Orchestrator *configuration.Orchestrator
	Service      *configuration.Orchestrator
	Projects     *project.Service
	Auth         *auth.Service
	PublicOrigin string
}

func RegisterConfigurationRoutes(mux *http.ServeMux, options ConfigurationOptions) {
	if options.Orchestrator == nil {
		options.Orchestrator = options.Service
	}
	h := configurationHandlers{options: options}
	register := func(pattern string, handler http.HandlerFunc) {
		if options.Auth != nil {
			mux.Handle(pattern, ProtectAPI(AuthOptions{Service: options.Auth, PublicOrigin: options.PublicOrigin}, http.HandlerFunc(handler)))
			return
		}
		mux.HandleFunc(pattern, handler)
	}
	register("GET /api/projects/{id}/configuration", h.get)
	register("PATCH /api/projects/{id}/configuration/{section}", h.patch)
	register("PATCH /api/projects/{id}/configuration/oauth/{provider}", h.patchOAuth)
	register("POST /api/projects/{id}/secrets/{kind}/reveal", h.reveal)
	register("POST /api/projects/{id}/secrets/databasePassword/rotate", h.rotate)
}

type configurationHandlers struct{ options ConfigurationOptions }

func (h configurationHandlers) get(w http.ResponseWriter, r *http.Request) {
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Project configuration is unavailable")
		return
	}
	snapshot, err := h.options.Orchestrator.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project was not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CONFIGURATION_GET_FAILED", "Unable to read project configuration")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

type sectionPatch struct {
	ExpectedRevision int64           `json:"expectedRevision"`
	Value            json.RawMessage `json:"value"`
}

func (h configurationHandlers) patch(w http.ResponseWriter, r *http.Request) {
	section := strings.ToLower(r.PathValue("section"))
	if section == "oauth" {
		h.patchOAuth(w, r)
		return
	}
	var envelope sectionPatch
	if err := decodeJSON(w, r, &envelope); err != nil || len(envelope.Value) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body is invalid")
		return
	}
	patch, err := sectionValuePatch(section, envelope.ExpectedRevision, envelope.Value)
	if err != nil {
		writeValidationError(w, err)
		return
	}
	if section == "smtp" {
		snapshot, getErr := h.options.Orchestrator.Get(r.Context(), r.PathValue("id"))
		if getErr != nil {
			h.handleConfigError(w, getErr)
			return
		}
		snapshot.Configuration.Auth.SMTP = patch.Auth.SMTP
		patch.Auth = &snapshot.Configuration.Auth
	}
	h.queue(w, r, patch)
}

func (h configurationHandlers) patchOAuth(w http.ResponseWriter, r *http.Request) {
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Project configuration is unavailable")
		return
	}
	var envelope sectionPatch
	if err := decodeJSON(w, r, &envelope); err != nil || len(envelope.Value) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body is invalid")
		return
	}
	var value contracts.OAuthProviderConfig
	if err := decodeRaw(envelope.Value, &value); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Configuration section is invalid")
		return
	}
	snapshot, err := h.options.Orchestrator.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.handleConfigError(w, err)
		return
	}
	if snapshot.Configuration.Auth.OAuth == nil {
		snapshot.Configuration.Auth.OAuth = map[string]contracts.OAuthProviderConfig{}
	}
	snapshot.Configuration.Auth.OAuth[r.PathValue("provider")] = value
	h.queue(w, r, contracts.ConfigurationPatch{ExpectedRevision: envelope.ExpectedRevision, Auth: &snapshot.Configuration.Auth})
}

func sectionValuePatch(section string, expected int64, raw []byte) (contracts.ConfigurationPatch, error) {
	p := contracts.ConfigurationPatch{ExpectedRevision: expected}
	switch section {
	case "general":
		var v contracts.GeneralConfig
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.General = &v
	case "services":
		var v contracts.Services
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.Services = &v
	case "auth":
		var v contracts.AuthConfig
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.Auth = &v
	case "smtp":
		var v contracts.SMTPConfig
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.Auth = &contracts.AuthConfig{SMTP: v}
	case "storage":
		var v contracts.StorageConfig
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.Storage = &v
	case "realtime":
		var v contracts.RealtimeConfig
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.Realtime = &v
	case "functions":
		var v contracts.FunctionsConfig
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.Functions = &v
	case "database":
		var v contracts.DatabaseConfig
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.Database = &v
	case "pooler", "connectionpooler":
		var v contracts.PoolerConfig
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.Pooler = &v
	case "network":
		var v contracts.NetworkConfig
		if err := decodeRaw(raw, &v); err != nil {
			return p, err
		}
		p.Network = &v
	default:
		return p, &project.ValidationError{Fields: map[string]string{"section": "unsupported configuration section"}}
	}
	return p, nil
}

func decodeRaw(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (h configurationHandlers) queue(w http.ResponseWriter, r *http.Request, patch contracts.ConfigurationPatch) {
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Project configuration is unavailable")
		return
	}
	queued, snapshot, err := h.options.Orchestrator.QueuePatch(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		h.handleConfigError(w, err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		if h.options.Projects != nil {
			p, e := h.options.Projects.Get(ctx, r.PathValue("id"))
			if e == nil {
				_, _ = h.options.Orchestrator.Run(ctx, p, queued, snapshot)
			}
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"projectId": r.PathValue("id"), "operationId": queued.ID, "revision": snapshot.Revision})
}

func (h configurationHandlers) handleConfigError(w http.ResponseWriter, err error) {
	if errors.Is(err, project.ErrStaleConfiguration) || errors.Is(err, store.ErrStaleConfiguration) {
		writeError(w, http.StatusConflict, "CONFIGURATION_STALE", "Project configuration revision is stale")
		return
	}
	writeValidationError(w, err)
}

func writeValidationError(w http.ResponseWriter, err error) {
	var validation *project.ValidationError
	if errors.As(err, &validation) {
		writeJSON(w, http.StatusUnprocessableEntity, contracts.ErrorEnvelope{Error: contracts.APIError{Code: "INVALID_CONFIGURATION", Message: "Configuration is invalid", Fields: validation.Fields}})
		return
	}
	writeError(w, http.StatusUnprocessableEntity, "INVALID_CONFIGURATION", err.Error())
}

func (h configurationHandlers) reveal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	identity, ok := IdentityFromContext(r.Context())
	if !ok || h.options.Auth == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body is invalid")
		return
	}
	if err := h.options.Auth.VerifyPassword(r.Context(), identity, input.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "RECENT_AUTH_REQUIRED", "Administrator password confirmation is required")
		return
	}
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Project configuration is unavailable")
		return
	}
	value, err := h.options.Orchestrator.Reveal(r.Context(), r.PathValue("id"), r.PathValue("kind"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "SECRET_NOT_FOUND", "Secret is not available")
		return
	}
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "SECRET_REVEAL_FAILED", "Unable to reveal secret")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"kind": r.PathValue("kind"), "value": value})
}

func (h configurationHandlers) rotate(w http.ResponseWriter, r *http.Request) {
	// Rotation is deliberately routed through the durable operation API. The
	// implementation is supplied by the configuration orchestrator when the
	// runtime supports database password changes.
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Project configuration is unavailable")
		return
	}
	identity, ok := IdentityFromContext(r.Context())
	if !ok || h.options.Auth == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body is invalid")
		return
	}
	if err := h.options.Auth.VerifyPassword(r.Context(), identity, input.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "RECENT_AUTH_REQUIRED", "Administrator password confirmation is required")
		return
	}
	queued, snapshot, newPassword, err := h.options.Orchestrator.QueueDatabasePasswordRotation(r.Context(), r.PathValue("id"))
	if err != nil {
		h.handleConfigError(w, err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		if h.options.Projects == nil {
			return
		}
		p, e := h.options.Projects.Get(ctx, r.PathValue("id"))
		if e == nil {
			_, _ = h.options.Orchestrator.RunDatabasePasswordRotation(ctx, p, queued, snapshot, newPassword)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"projectId": r.PathValue("id"), "operationId": queued.ID, "revision": snapshot.Revision})
}
