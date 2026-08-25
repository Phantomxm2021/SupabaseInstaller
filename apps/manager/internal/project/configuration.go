package project

import (
	"fmt"
	"net/mail"
	"regexp"
	"sort"
	"strings"

	"supabase-manager/internal/contracts"
)

// ValidationError preserves field paths for API clients and form rendering.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Fields) == 0 {
		return "configuration is invalid"
	}
	keys := make([]string, 0, len(e.Fields))
	for key := range e.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", key, e.Fields[key]))
	}
	return strings.Join(parts, "; ")
}

func (e *ValidationError) add(field, message string) {
	if e.Fields == nil {
		e.Fields = make(map[string]string)
	}
	if _, exists := e.Fields[field]; !exists {
		e.Fields[field] = message
	}
}

// DefaultConfiguration returns a complete safe configuration for a preset.
func DefaultConfiguration(preset contracts.Preset) contracts.ProjectConfiguration {
	return contracts.ProjectConfiguration{
		General:  contracts.GeneralConfig{SupabaseVersion: "self-hosted/v0.8.0"},
		Services: ApplyPreset(preset),
		Auth: contracts.AuthConfig{
			Enabled: true,
			Email:   contracts.EmailAuthConfig{Enabled: true, AllowSignup: true},
			SMTP:    contracts.SMTPConfig{Port: 587},
		},
		Storage:   contracts.StorageConfig{Backend: contracts.StorageBackendLocal},
		Realtime:  contracts.RealtimeConfig{MaximumConnections: 100, DatabasePoolSize: 5, LogLevel: contracts.LogLevelInfo},
		Functions: contracts.FunctionsConfig{DefaultJWTVerification: true, Directory: "./functions"},
		Database:  contracts.DatabaseConfig{Version: "15", MaximumConnections: 100},
		Network:   contracts.NetworkConfig{Gateway: contracts.GatewayEnvoy, HTTPSMode: contracts.HTTPSModeExternal},
	}
}

// ApplyConfigurationPreset builds the aggregate and applies all service
// dependency closure rules in one deterministic operation.
func ApplyConfigurationPreset(preset contracts.Preset) contracts.ProjectConfiguration {
	cfg := DefaultConfiguration(contracts.PresetLightweight)
	cfg.Services = ApplyPreset(preset)
	if cfg.Services.Studio {
		cfg.Services.PostgresMeta = true
	}
	if cfg.Services.Imgproxy {
		cfg.Services.Storage = true
	}
	if cfg.Services.Logs {
		cfg.Services.Vector = true
	} else {
		cfg.Services.Vector = false
	}
	return cfg
}

var envNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

var reservedFunctionVariables = map[string]struct{}{
	"ANON_KEY": {}, "SERVICE_ROLE_KEY": {}, "JWT_SECRET": {}, "SUPABASE_URL": {},
	"SUPABASE_PUBLIC_URL": {}, "FUNCTIONS_VERIFY_JWT": {}, "POSTGRES_PASSWORD": {},
}

func ValidateConfiguration(cfg contracts.ProjectConfiguration) error {
	validation := &ValidationError{Fields: make(map[string]string)}
	if strings.TrimSpace(cfg.General.Domain) != "" && !validDomain(cfg.General.Domain) {
		validation.add("general.domain", "must be a hostname without a scheme or path")
	}
	if strings.TrimSpace(cfg.General.SiteURL) != "" && !validAbsoluteHTTPURL(cfg.General.SiteURL) {
		validation.add("general.siteUrl", "must be an absolute http or https URL")
	}
	if cfg.General.SupabaseVersion == "" || strings.EqualFold(cfg.General.SupabaseVersion, "latest") || strings.EqualFold(cfg.General.SupabaseVersion, "master") {
		validation.add("general.supabaseVersion", "must be a pinned supported version")
	}
	validateServicesConfiguration(cfg.Services, validation)
	validateAuth(cfg.Auth, validation)
	validateStorage(cfg.Storage, validation)
	validateRealtime(cfg.Realtime, validation)
	validateFunctions(cfg.Functions, validation)
	validateDatabase(cfg.Database, validation)
	validatePooler(cfg.Pooler, validation)
	validateNetwork(cfg.Network, validation)
	if len(validation.Fields) == 0 {
		return nil
	}
	return validation
}

func validateServicesConfiguration(services contracts.Services, validation *ValidationError) {
	if !services.Database {
		validation.add("services.database", "PostgreSQL is required")
	}
	if services.Studio && !services.PostgresMeta {
		validation.add("services.postgresMeta", "postgres-meta is required by Studio")
	}
	if (services.Auth || services.REST || services.Studio || services.Realtime || services.Storage) && !services.Gateway {
		validation.add("services.gateway", "API Gateway is required by enabled public services")
	}
	if services.Imgproxy && !services.Storage {
		validation.add("services.imgproxy", "Image Transformation requires Storage")
	}
	if services.Vector != services.Logs {
		validation.add("services.vector", "Logs and Vector must be enabled together")
	}
}

func validateAuth(auth contracts.AuthConfig, validation *ValidationError) {
	validateSecretInput(auth.SMTP.Password, "auth.smtp.password", validation)
	if auth.SMTP.Enabled {
		if strings.TrimSpace(auth.SMTP.Host) == "" {
			validation.add("auth.smtp.host", "is required when SMTP is enabled")
		}
		validatePort(auth.SMTP.Port, "auth.smtp.port", validation)
		if _, err := mail.ParseAddress(auth.SMTP.SenderEmail); err != nil || strings.TrimSpace(auth.SMTP.SenderEmail) == "" {
			validation.add("auth.smtp.senderEmail", "must be a valid email address")
		}
		if strings.TrimSpace(auth.SMTP.SenderName) == "" {
			validation.add("auth.smtp.senderName", "is required when SMTP is enabled")
		}
	}
	for index, redirect := range auth.RedirectURLs {
		if !validAbsoluteHTTPURL(redirect) {
			validation.add(fmt.Sprintf("auth.redirectUrls[%d]", index), "must be an absolute http or https URL")
		}
	}
	known := make(map[string]struct{}, len(contracts.OAuthProviderNames))
	for _, provider := range contracts.OAuthProviderNames {
		known[provider] = struct{}{}
	}
	for provider, config := range auth.OAuth {
		field := "auth.oauth." + provider
		if _, ok := known[provider]; !ok {
			validation.add(field, "is not a supported OAuth provider")
			continue
		}
		if !config.Enabled {
			continue
		}
		if strings.TrimSpace(config.ClientID) == "" {
			validation.add(field+".clientId", "is required when provider is enabled")
		}
		validateSecretInput(config.Secret, field+".secret", validation)
		for _, required := range oauthRequiredFields(provider) {
			if strings.TrimSpace(config.Fields[required]) == "" {
				validation.add(field+".fields."+required, "is required for this provider")
			}
		}
	}
}

func oauthRequiredFields(provider string) []string {
	switch provider {
	case "azure":
		return []string{"tenantUrl"}
	case "keycloak":
		return []string{"realmUrl"}
	}
	return nil
}

func validateStorage(storage contracts.StorageConfig, validation *ValidationError) {
	switch storage.Backend {
	case contracts.StorageBackendLocal:
	case contracts.StorageBackendS3, contracts.StorageBackendAWSS3, contracts.StorageBackendR2:
		if strings.TrimSpace(storage.Bucket) == "" {
			validation.add("storage.bucket", "is required for an object storage backend")
		}
		if storage.Backend != contracts.StorageBackendR2 && strings.TrimSpace(storage.Region) == "" {
			validation.add("storage.region", "is required for this object storage backend")
		}
		if storage.Endpoint != "" && !validAbsoluteHTTPURL(storage.Endpoint) {
			validation.add("storage.endpoint", "must be an absolute http or https URL")
		}
	default:
		validation.add("storage.backend", "must be local, s3, aws-s3, or r2")
	}
	validateSecretInput(storage.SecretAccessKey, "storage.secretAccessKey", validation)
}

func validateRealtime(realtime contracts.RealtimeConfig, validation *ValidationError) {
	if realtime.MaximumConnections < 0 || realtime.MaximumConnections > 100000 {
		validation.add("realtime.maximumConnections", "must be between 0 and 100000")
	}
	if realtime.MaxConnections < 0 || realtime.MaxConnections > 100000 {
		validation.add("realtime.maxConnections", "must be between 0 and 100000")
	}
	if realtime.DatabasePoolSize < 0 || realtime.DatabasePoolSize > 10000 {
		validation.add("realtime.databasePoolSize", "must be between 0 and 10000")
	}
	if realtime.LogLevel != "" && realtime.LogLevel != contracts.LogLevelDebug && realtime.LogLevel != contracts.LogLevelInfo && realtime.LogLevel != contracts.LogLevelWarn && realtime.LogLevel != contracts.LogLevelError {
		validation.add("realtime.logLevel", "must be debug, info, warn, or error")
	}
}

func validateFunctions(functions contracts.FunctionsConfig, validation *ValidationError) {
	for index, variable := range functions.Variables {
		field := fmt.Sprintf("functions.variables[%d].name", index)
		if !envNamePattern.MatchString(variable.Name) {
			validation.add(field, "must match ^[A-Z_][A-Z0-9_]*$")
		}
		if _, reserved := reservedFunctionVariables[variable.Name]; reserved || strings.HasPrefix(variable.Name, "SUPABASE_") {
			validation.add(field, "is reserved by Supabase/runtime")
		}
		validateSecretInput(variable.Value, fmt.Sprintf("functions.variables[%d].value", index), validation)
	}
}

func validateDatabase(database contracts.DatabaseConfig, validation *ValidationError) {
	if database.MaximumConnections < 0 || database.MaximumConnections > 100000 {
		validation.add("database.maximumConnections", "must be between 0 and 100000")
	}
	if database.MaxConnections < 0 || database.MaxConnections > 100000 {
		validation.add("database.maxConnections", "must be between 0 and 100000")
	}
	if database.DirectPortNumber != 0 {
		validatePort(database.DirectPortNumber, "database.directPortNumber", validation)
	}
}

func validatePooler(pooler contracts.PoolerConfig, validation *ValidationError) {
	if pooler.TransactionPort != 0 {
		validatePort(pooler.TransactionPort, "pooler.transactionPort", validation)
	}
	if pooler.SessionPort != 0 {
		validatePort(pooler.SessionPort, "pooler.sessionPort", validation)
	}
	if pooler.PoolSize < 0 || pooler.PoolSize > 100000 {
		validation.add("pooler.poolSize", "must be between 0 and 100000")
	}
	if pooler.MaximumClients < 0 || pooler.MaximumClients > 100000 {
		validation.add("pooler.maximumClients", "must be between 0 and 100000")
	}
	if pooler.MaxClients < 0 || pooler.MaxClients > 100000 {
		validation.add("pooler.maxClients", "must be between 0 and 100000")
	}
	if pooler.MaxClientConnections < 0 || pooler.MaxClientConnections > 100000 {
		validation.add("pooler.maxClientConnections", "must be between 0 and 100000")
	}
}

func validateNetwork(network contracts.NetworkConfig, validation *ValidationError) {
	if network.Gateway != "" && network.Gateway != contracts.GatewayEnvoy && network.Gateway != contracts.GatewayKong {
		validation.add("network.gateway", "must be envoy or kong")
	}
	if network.HTTPSMode != "" && network.HTTPSMode != contracts.HTTPSModeExternal && network.HTTPSMode != contracts.HTTPSModeCaddy && network.HTTPSMode != contracts.HTTPSModeManual {
		validation.add("network.httpsMode", "must be external, caddy, or manual")
	}
	for field, port := range map[string]int{"apiPort": network.APIPort, "studioPort": network.StudioPort, "directDatabasePort": network.DirectDatabasePort, "poolerPort": network.PoolerPort} {
		if port != 0 {
			validatePort(port, "network."+field, validation)
		}
	}
}

func validatePort(port int, field string, validation *ValidationError) {
	if port < 1 || port > 65535 {
		validation.add(field, "must be between 1 and 65535")
	}
}

func validateSecretInput(input contracts.SecretInput, field string, validation *ValidationError) {
	switch input.Action {
	case "", "retain", "remove":
	case "replace":
		if input.Value == "" {
			validation.add(field, "replace requires a value")
		}
	default:
		validation.add(field+".action", "must be retain, replace, or remove")
	}
}

var _ error = (*ValidationError)(nil)
