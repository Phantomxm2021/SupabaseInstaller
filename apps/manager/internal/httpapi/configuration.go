package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"supabase-manager/apps/manager/internal/auth"
	"supabase-manager/apps/manager/internal/configuration"
	"supabase-manager/apps/manager/internal/operation"
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
	ManagedTLS   ManagedTLSStager
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
	register("POST /api/projects/{id}/runtime/sync", h.syncOfficialRuntime)
	register("PATCH /api/projects/{id}/configuration/network/tls", h.patchNetworkTLS)
	register("PATCH /api/projects/{id}/configuration/oauth/{provider}", h.patchOAuth)
	register("POST /api/projects/{id}/secrets/{kind}/reveal", h.reveal)
	register("POST /api/projects/{id}/secrets/databasePassword/rotate", h.rotate)
	register("POST /api/projects/{id}/auth-keys/migrate", h.authKeys)
	register("POST /api/projects/{id}/auth-keys/rotate-api", h.authKeys)
	register("POST /api/projects/{id}/auth-keys/rotate-signing", h.authKeys)
}

type configurationHandlers struct{ options ConfigurationOptions }

// syncOfficialRuntime reapplies the project's persisted configuration through
// the currently bundled official Supabase Docker template. It is intentionally
// separate from ordinary settings changes so an administrator explicitly
// chooses when a deployed host is recreated after a Manager template update.
func (h configurationHandlers) syncOfficialRuntime(w http.ResponseWriter, r *http.Request) {
	h.queueOfficialRuntimeSync(w, r)
}

func (h configurationHandlers) get(w http.ResponseWriter, r *http.Request) {
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Server configuration is unavailable")
		return
	}
	snapshot, err := h.options.Orchestrator.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Server was not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CONFIGURATION_GET_FAILED", "Unable to read server configuration")
		return
	}
	// Project URL is derived from the typed public domain. Key material,
	// including anon key, remains behind recent-auth reveal.
	response := struct {
		store.ConfigurationSnapshot
		ProjectURL string `json:"projectUrl"`
	}{ConfigurationSnapshot: snapshot, ProjectURL: "https://" + snapshot.Configuration.General.Domain}
	writeJSON(w, http.StatusOK, response)
}

type sectionPatch struct {
	Value json.RawMessage `json:"value"`
}

// patchNetworkTLS is deliberately separate from the JSON network PATCH route:
// certificate material is write-only and must never enter a configuration
// snapshot, operation payload, or ordinary JSON request. A name-only update
// persists the deterministic host paths for an operator-provisioned pair.
func (h configurationHandlers) patchNetworkTLS(w http.ResponseWriter, r *http.Request) {
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Server configuration is unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxManagedTLSUploadBytes)
	if err := r.ParseMultipartForm(maxManagedTLSUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TLS_UPLOAD", "TLS upload is invalid or too large")
		return
	}
	certificate, certificatePresent, err := multipartFileBytes(r, "certificate")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TLS_UPLOAD", err.Error())
		return
	}
	privateKey, keyPresent, err := multipartFileBytes(r, "privateKey")
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_TLS_UPLOAD", err.Error())
		return
	}
	if certificatePresent != keyPresent {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_TLS_UPLOAD", "Certificate and private key must be uploaded together")
		return
	}
	name := strings.TrimSpace(r.FormValue("certificateName"))
	snapshot, err := h.options.Orchestrator.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.handleConfigError(w, err)
		return
	}
	if snapshot.Configuration.Network.HTTPSMode != contracts.HTTPSModeExternal {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_TLS_CONFIGURATION", "Managed TLS requires external HTTPS mode")
		return
	}
	managedTLS, err := contracts.ManagedTLSPaths(name, snapshot.Configuration.General.SiteURL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_TLS_CONFIGURATION", err.Error())
		return
	}
	if certificatePresent {
		if h.options.ManagedTLS == nil {
			writeError(w, http.StatusServiceUnavailable, "MANAGED_TLS_UNAVAILABLE", "Managed TLS is unavailable")
			return
		}
		parsed, err := url.Parse(snapshot.Configuration.General.SiteURL)
		if err != nil || parsed.Hostname() == "" {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_TLS_CONFIGURATION", "Site URL is required before uploading TLS")
			return
		}
		staged, err := h.options.ManagedTLS.StageManagedTLS(r.Context(), contracts.StageManagedTLSRequest{
			CertificateName: name,
			BaseDomain:      parsed.Hostname(),
			CertificatePEM:  certificate,
			PrivateKeyPEM:   privateKey,
		})
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "TLS_STAGE_FAILED", "Unable to stage the TLS certificate")
			return
		}
		managedTLS = staged.ManagedTLSConfig
	}
	network := snapshot.Configuration.Network
	network.ManagedTLS = &managedTLS
	h.queue(w, r, contracts.ConfigurationPatch{Network: &network})
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
	patch, err := sectionValuePatch(section, envelope.Value)
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
		merged := mergeSMTPAuthPatch(snapshot.Configuration.Auth, patch.Auth.SMTP)
		patch.Auth = &merged
	}
	h.queue(w, r, patch)
}

func (h configurationHandlers) patchOAuth(w http.ResponseWriter, r *http.Request) {
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Server configuration is unavailable")
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
	merged := mergeOAuthAuthPatch(snapshot.Configuration.Auth, r.PathValue("provider"), value)
	h.queue(w, r, contracts.ConfigurationPatch{Auth: &merged})
}

func sectionValuePatch(section string, raw []byte) (contracts.ConfigurationPatch, error) {
	p := contracts.ConfigurationPatch{}
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

// mergeSMTPAuthPatch builds the Auth section used by the subsection endpoint.
// The endpoint owns only SMTP; redacted configured OAuth/Phone siblings are
// marked retain internally so aggregate validation can inspect the stored
// secrets without treating the redacted marker as a user command.
func mergeSMTPAuthPatch(base contracts.AuthConfig, incoming contracts.SMTPConfig) contracts.AuthConfig {
	if incoming.Password.Action == "" {
		if strings.TrimSpace(incoming.Password.Value) != "" {
			incoming.Password.Action = "replace"
		} else if base.SMTP.PasswordSet {
			incoming.Password.Action = "retain"
		}
	}
	base.SMTP = incoming
	markUntouchedAuthSecrets(&base, "smtp")
	return base
}

// mergeOAuthAuthPatch builds the Auth section used by one-provider updates.
// Only the selected provider is incoming; every other configured secret is an
// internal retain marker. The selected provider's action is never rewritten.
func mergeOAuthAuthPatch(base contracts.AuthConfig, provider string, incoming contracts.OAuthProviderConfig) contracts.AuthConfig {
	if base.OAuth == nil {
		base.OAuth = map[string]contracts.OAuthProviderConfig{}
	}
	if incoming.Secret.Action == "" {
		if strings.TrimSpace(incoming.Secret.Value) != "" {
			incoming.Secret.Action = "replace"
		} else if existing, ok := base.OAuth[provider]; ok && existing.SecretSet {
			incoming.Secret.Action = "retain"
		}
	}
	base.OAuth[provider] = incoming
	markUntouchedAuthSecrets(&base, "oauth:"+provider)
	return base
}

func markUntouchedAuthSecrets(auth *contracts.AuthConfig, owned string) {
	if owned != "smtp" && auth.SMTP.PasswordSet && auth.SMTP.Password.Action == "" {
		auth.SMTP.Password.Action = "retain"
	}
	if owned != "phone" && auth.Phone.SecretSet && auth.Phone.Secret.Action == "" {
		auth.Phone.Secret.Action = "retain"
	}
	for provider, value := range auth.OAuth {
		if owned == "oauth:"+provider {
			continue
		}
		if value.SecretSet && value.Secret.Action == "" {
			value.Secret.Action = "retain"
			auth.OAuth[provider] = value
		}
	}
}

func decodeRaw(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("section contains multiple JSON values")
		}
		return err
	}
	return nil
}

func (h configurationHandlers) queue(w http.ResponseWriter, r *http.Request, patch contracts.ConfigurationPatch) {
	h.queueConfigurationReconcile(w, r, patch, false)
}

func (h configurationHandlers) queueOfficialRuntimeSync(w http.ResponseWriter, r *http.Request) {
	h.queueConfigurationReconcile(w, r, contracts.ConfigurationPatch{}, true)
}

func (h configurationHandlers) queueConfigurationReconcile(w http.ResponseWriter, r *http.Request, patch contracts.ConfigurationPatch, officialRuntimeSync bool) {
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Server configuration is unavailable")
		return
	}
	var queued operation.Operation
	var snapshot store.ConfigurationSnapshot
	var err error
	if officialRuntimeSync {
		queued, snapshot, err = h.options.Orchestrator.QueueOfficialRuntimeSync(r.Context(), r.PathValue("id"))
	} else {
		queued, snapshot, err = h.options.Orchestrator.QueuePatch(r.Context(), r.PathValue("id"), patch)
	}
	if err != nil {
		h.handleConfigError(w, err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		if h.options.Projects == nil {
			h.options.Orchestrator.Release(r.PathValue("id"))
			return
		}
		p, e := h.options.Projects.Get(ctx, r.PathValue("id"))
		if e == nil {
			if officialRuntimeSync {
				_, _ = h.options.Orchestrator.RunOfficialRuntimeSync(ctx, p, queued, snapshot)
			} else {
				_, _ = h.options.Orchestrator.Run(ctx, p, queued, snapshot)
			}
		} else {
			h.options.Orchestrator.Release(r.PathValue("id"))
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"projectId": r.PathValue("id"), "operationId": queued.ID, "revision": snapshot.Revision})
}

func (h configurationHandlers) handleConfigError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConfigurationBusy) {
		writeError(w, http.StatusConflict, "CONFIGURATION_BUSY", "Another configuration operation is active")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Server was not found")
		return
	}
	if errors.Is(err, store.ErrConfigurationConflict) {
		writeError(w, http.StatusConflict, "CONFIGURATION_CONFLICT", "Configuration conflicts with another server")
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
	if h.options.Projects != nil {
		if _, err := h.options.Projects.Get(r.Context(), r.PathValue("id")); err != nil {
			h.handleConfigError(w, err)
			return
		}
	}
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Server configuration is unavailable")
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
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Server configuration is unavailable")
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
			h.options.Orchestrator.Release(r.PathValue("id"))
			return
		}
		p, e := h.options.Projects.Get(ctx, r.PathValue("id"))
		if e == nil {
			_, _ = h.options.Orchestrator.RunDatabasePasswordRotation(ctx, p, queued, snapshot, newPassword)
		} else {
			h.options.Orchestrator.Release(r.PathValue("id"))
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"projectId": r.PathValue("id"), "operationId": queued.ID, "revision": snapshot.Revision})
}

func (h configurationHandlers) authKeys(w http.ResponseWriter, r *http.Request) {
	if h.options.Orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "CONFIGURATION_UNAVAILABLE", "Server configuration is unavailable")
		return
	}
	identity, ok := IdentityFromContext(r.Context())
	if !ok || h.options.Auth == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication is required")
		return
	}
	var input struct {
		Password           string `json:"password"`
		ConfirmProjectName string `json:"confirmProjectName"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Request body is invalid")
		return
	}
	if err := h.options.Auth.VerifyPassword(r.Context(), identity, input.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "RECENT_AUTH_REQUIRED", "Administrator password confirmation is required")
		return
	}
	p, err := h.options.Projects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.handleConfigError(w, err)
		return
	}
	path := r.URL.Path
	kind := "MIGRATE_AUTH_KEYS"
	if strings.HasSuffix(path, "/rotate-api") {
		kind = "ROTATE_API_KEYS"
	}
	if strings.HasSuffix(path, "/rotate-signing") {
		kind = "ROTATE_SIGNING_KEYS"
		if input.ConfirmProjectName != p.Name {
			writeError(w, http.StatusUnprocessableEntity, "PROJECT_NAME_CONFIRMATION_REQUIRED", "Exact project name confirmation is required; signing rotation invalidates all ES256 sessions")
			return
		}
	}
	op, snapshot, candidate, err := h.options.Orchestrator.QueueAuthKeysOperation(r.Context(), p.ID, kind)
	if err != nil {
		h.handleConfigError(w, err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		_, _ = h.options.Orchestrator.RunAuthKeys(ctx, p, op, snapshot, candidate)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"projectId": p.ID, "operationId": op.ID})
}
