package project

import (
	"fmt"
	"net/mail"
	"net/url"
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
			Enabled:    true,
			Email:      contracts.EmailAuthConfig{Enabled: true, AllowSignup: true},
			SMTP:       contracts.SMTPConfig{Port: 587},
			Mailer:     defaultMailerConfig(),
			RateLimits: contracts.RateLimitConfig{EmailSent: 30, SMSSent: 30, TokenRefresh: 150, TokenVerification: 30, AnonymousUsers: 30, SignupsAndSignins: 30},
			MFA:        contracts.MFAConfig{TOTPEnrollEnabled: true, TOTPVerifyEnabled: true, MaxEnrolledFactors: 10, PhoneOTPLength: 6},
		},
		Storage:   contracts.StorageConfig{Backend: contracts.StorageBackendLocal},
		Realtime:  contracts.RealtimeConfig{MaxConnections: 100, DatabasePoolSize: 5, LogLevel: contracts.LogLevelInfo},
		Functions: contracts.FunctionsConfig{DefaultJWTVerification: true, Directory: "./functions"},
		Database:  contracts.DatabaseConfig{Version: "15", MaxConnections: 100},
		Pooler:    contracts.PoolerConfig{PoolSize: 20, MaxClientConnections: 100},
		Network:   contracts.NetworkConfig{Gateway: contracts.GatewayEnvoy, HTTPSMode: contracts.HTTPSModeExternal},
	}
}

func defaultMailerTemplate(subject string) contracts.EmailTemplateConfig {
	return contracts.EmailTemplateConfig{Subject: subject}
}

func defaultMailerConfig() contracts.MailerConfig {
	return contracts.MailerConfig{
		Templates: contracts.EmailTemplatesConfig{
			Confirmation: defaultMailerTemplate("Confirm your signup"), Invite: defaultMailerTemplate("You have been invited"), MagicLink: defaultMailerTemplate("Your magic link"),
			EmailChange: defaultMailerTemplate("Confirm email change"), Recovery: defaultMailerTemplate("Reset password"), Reauthentication: defaultMailerTemplate("Confirm reauthentication"),
		},
		Notifications: contracts.EmailNotificationsConfig{
			PasswordChanged: contracts.EmailNotificationConfig{Template: defaultMailerTemplate("Your password was changed")}, EmailChanged: contracts.EmailNotificationConfig{Template: defaultMailerTemplate("Your email address was changed")}, PhoneChanged: contracts.EmailNotificationConfig{Template: defaultMailerTemplate("Your phone number was changed")},
			IdentityLinked: contracts.EmailNotificationConfig{Template: defaultMailerTemplate("A sign-in method was linked")}, IdentityUnlinked: contracts.EmailNotificationConfig{Template: defaultMailerTemplate("A sign-in method was removed")}, MFAFactorEnrolled: contracts.EmailNotificationConfig{Template: defaultMailerTemplate("An MFA method was added")}, MFAFactorUnenrolled: contracts.EmailNotificationConfig{Template: defaultMailerTemplate("An MFA method was removed")},
		},
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
	if cfg.General.SupabaseVersion != "self-hosted/v0.8.0" {
		validation.add("general.supabaseVersion", "must be self-hosted/v0.8.0")
	}
	validateServicesConfiguration(cfg.Services, validation)
	if cfg.Services.Auth != cfg.Auth.Enabled {
		validation.add("auth.enabled", "must match services.auth")
	}
	validateAuth(cfg.Auth, validation)
	validateStorage(cfg.Storage, validation)
	validateRealtime(cfg.Realtime, validation)
	validateFunctions(cfg.Functions, validation)
	validateDatabase(cfg.Database, validation)
	validatePooler(cfg.Pooler, validation)
	validateNetwork(cfg.Network, validation)
	if cfg.Network.HTTPSMode == contracts.HTTPSModeCaddy && !cfg.Services.Gateway {
		validation.add("services.gateway", "Caddy HTTPS requires API Gateway")
	}
	if len(validation.Fields) == 0 {
		return nil
	}
	return validation
}

// ValidateStoredConfiguration validates the redacted canonical state emitted
// after command actions have been consumed into encrypted mutations. A stored
// configured secret has an empty action by design; command validation remains
// strict at PreparePatch/Save boundaries.
func ValidateStoredConfiguration(cfg contracts.ProjectConfiguration) error {
	if cfg.Auth.SMTP.PasswordSet && cfg.Auth.SMTP.Password.Action == "" {
		cfg.Auth.SMTP.Password.Action = "retain"
	}
	if cfg.Auth.Phone.SecretSet && cfg.Auth.Phone.Secret.Action == "" {
		cfg.Auth.Phone.Secret.Action = "retain"
	}
	for provider, value := range cfg.Auth.OAuth {
		if value.SecretSet && value.Secret.Action == "" {
			value.Secret.Action = "retain"
			cfg.Auth.OAuth[provider] = value
		}
	}
	if cfg.Storage.SecretAccessKeySet && cfg.Storage.SecretAccessKey.Action == "" {
		cfg.Storage.SecretAccessKey.Action = "retain"
	}
	for index := range cfg.Functions.Variables {
		if cfg.Functions.Variables[index].ValueSet && cfg.Functions.Variables[index].Value.Action == "" {
			cfg.Functions.Variables[index].Value.Action = "retain"
		}
	}
	return ValidateConfiguration(cfg)
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
	if services.Functions && !services.Gateway {
		validation.add("services.gateway", "API Gateway is required by enabled Functions")
	}
	if services.Storage && (!services.Database || !services.REST) {
		validation.add("services.storage", "Storage requires database and REST")
	}
	if services.Realtime && !services.Database {
		validation.add("services.realtime", "Realtime requires database")
	}
	if services.Supavisor && !services.Database {
		validation.add("services.supavisor", "Supavisor requires database")
	}
	if services.DirectDB && !services.Database {
		validation.add("services.directDb", "Direct database requires database")
	}
	if services.Logs && (!services.Database || !services.Vector) {
		validation.add("services.logs", "Logs requires database and Vector")
	}
	if services.Vector && !services.Logs {
		validation.add("services.vector", "Vector requires Logs")
	}
	if services.Imgproxy && !services.Storage {
		validation.add("services.imgproxy", "Image Transformation requires Storage")
	}
	if services.Vector != services.Logs {
		validation.add("services.vector", "Logs and Vector must be enabled together")
	}
}

func validateAuth(auth contracts.AuthConfig, validation *ValidationError) {
	if auth.JWTExpiry < 0 || auth.JWTExpiry > 31536000 {
		validation.add("auth.jwtExpiry", "must be between 0 and 31536000 seconds")
	}
	if auth.DisableSignup != !auth.Email.AllowSignup {
		validation.add("auth.disableSignup", "must equal the inverse of auth.email.allowSignup")
		validation.add("auth.email.allowSignup", "must equal the inverse of auth.disableSignup")
	}
	if auth.DisableSignup && (auth.Phone.Enabled || auth.AnonymousSignIn || hasEnabledOAuth(auth.OAuth)) {
		validation.add("auth.disableSignup", "cannot disable global signup while phone, anonymous, or OAuth signup is enabled")
	}
	if auth.Email.SecureEmailChange != auth.Email.DoubleConfirmChanges {
		validation.add("auth.email.secureEmailChange", "must match doubleConfirmChanges for the pinned runtime capability")
		validation.add("auth.email.doubleConfirmChanges", "must match secureEmailChange for the pinned runtime capability")
	}
	validateRateLimits(auth.RateLimits, validation)
	validateMFA(auth.MFA, validation)
	validateMailer(auth.Mailer, validation)
	validateSecretInput(auth.SMTP.Password, "auth.smtp.password", validation)
	validatePhone(auth.Phone, validation)
	if auth.SMTP.Enabled {
		if strings.TrimSpace(auth.SMTP.Host) == "" {
			validation.add("auth.smtp.host", "is required when SMTP is enabled")
		}
		if strings.TrimSpace(auth.SMTP.Username) == "" {
			validation.add("auth.smtp.username", "is required when SMTP is enabled")
		}
		if !auth.SMTP.PasswordSet && auth.SMTP.Password.Action != "replace" {
			validation.add("auth.smtp.password", "must retain an existing password or replace it")
		}
		if auth.SMTP.PasswordSet && auth.SMTP.Password.Action != "retain" && auth.SMTP.Password.Action != "replace" {
			validation.add("auth.smtp.password", "an existing password must use retain or replace")
		}
		if auth.SMTP.Password.Action == "remove" {
			validation.add("auth.smtp.password", "cannot be removed while SMTP is enabled")
		}
		validatePort(auth.SMTP.Port, "auth.smtp.port", validation)
		parsedEmail, emailErr := mail.ParseAddress(auth.SMTP.SenderEmail)
		if emailErr != nil || parsedEmail.Name != "" || parsedEmail.Address != auth.SMTP.SenderEmail || strings.TrimSpace(auth.SMTP.SenderEmail) == "" {
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
		for key := range config.Fields {
			if !providerFieldAllowed(provider, key) {
				validation.add(field+".fields."+key, "is not supported for this provider")
			}
		}
		if !config.Enabled {
			continue
		}
		if strings.TrimSpace(config.ClientID) == "" {
			validation.add(field+".clientId", "is required when provider is enabled")
		}
		if !config.SecretSet && config.Secret.Action != "replace" {
			validation.add(field+".secret", "must retain an existing secret or replace it")
		}
		if config.SecretSet && config.Secret.Action != "retain" && config.Secret.Action != "replace" {
			validation.add(field+".secret", "an existing secret must use retain or replace")
		}
		if config.Secret.Action == "remove" {
			validation.add(field+".secret", "cannot be removed while provider is enabled")
		}
		validateSecretInput(config.Secret, field+".secret", validation)
		for _, required := range oauthRequiredFields(provider) {
			if strings.TrimSpace(config.Fields[required]) == "" {
				validation.add(field+".fields."+required, "is required for this provider")
			}
			if value := strings.TrimSpace(config.Fields[required]); value != "" {
				if _, err := url.ParseRequestURI(value); err != nil || !validAbsoluteHTTPURL(value) {
					validation.add(field+".fields."+required, "must be an absolute http or https URL")
				}
			}
		}
		for key, value := range config.Fields {
			if strings.HasSuffix(key, "Url") && strings.TrimSpace(value) != "" && !validAbsoluteHTTPURL(value) {
				validation.add(field+".fields."+key, "must be an absolute http or https URL")
			}
		}
	}
}

func validateRateLimits(limits contracts.RateLimitConfig, validation *ValidationError) {
	for field, value := range map[string]int{
		"emailSent": limits.EmailSent, "smsSent": limits.SMSSent, "tokenRefresh": limits.TokenRefresh,
		"tokenVerification": limits.TokenVerification, "anonymousUsers": limits.AnonymousUsers, "signupsAndSignins": limits.SignupsAndSignins,
	} {
		if value < 1 || value > 1000000 {
			validation.add("auth.rateLimits."+field, "must be between 1 and 1000000")
		}
	}
}

func validateMFA(mfa contracts.MFAConfig, validation *ValidationError) {
	if mfa.MaxEnrolledFactors < 1 || mfa.MaxEnrolledFactors > 100 {
		validation.add("auth.mfa.maxEnrolledFactors", "must be between 1 and 100")
	}
	if mfa.PhoneOTPLength < 4 || mfa.PhoneOTPLength > 10 {
		validation.add("auth.mfa.phoneOtpLength", "must be between 4 and 10 digits")
	}
}

var allowedMailerTemplateVariables = map[string]struct{}{
	".ConfirmationURL": {}, ".Token": {}, ".TokenHash": {}, ".SiteURL": {},
	".Email": {}, ".Data": {}, ".RedirectTo": {},
}

var mailerTemplateActionPattern = regexp.MustCompile(`{{\s*([^{}]+?)\s*}}`)

func validateMailer(mailer contracts.MailerConfig, validation *ValidationError) {
	templates := []struct {
		path     string
		template contracts.EmailTemplateConfig
	}{
		{"auth.mailer.templates.confirmation", mailer.Templates.Confirmation}, {"auth.mailer.templates.invite", mailer.Templates.Invite},
		{"auth.mailer.templates.magicLink", mailer.Templates.MagicLink}, {"auth.mailer.templates.emailChange", mailer.Templates.EmailChange},
		{"auth.mailer.templates.recovery", mailer.Templates.Recovery}, {"auth.mailer.templates.reauthentication", mailer.Templates.Reauthentication},
		{"auth.mailer.notifications.passwordChanged", mailer.Notifications.PasswordChanged.Template},
		{"auth.mailer.notifications.emailChanged", mailer.Notifications.EmailChanged.Template},
		{"auth.mailer.notifications.phoneChanged", mailer.Notifications.PhoneChanged.Template},
		{"auth.mailer.notifications.identityLinked", mailer.Notifications.IdentityLinked.Template},
		{"auth.mailer.notifications.identityUnlinked", mailer.Notifications.IdentityUnlinked.Template},
		{"auth.mailer.notifications.mfaFactorEnrolled", mailer.Notifications.MFAFactorEnrolled.Template},
		{"auth.mailer.notifications.mfaFactorUnenrolled", mailer.Notifications.MFAFactorUnenrolled.Template},
	}
	for _, item := range templates {
		validateMailerTemplate(item.template, item.path, validation)
	}
}

func validateMailerTemplate(template contracts.EmailTemplateConfig, field string, validation *ValidationError) {
	if len(template.Subject) > 255 {
		validation.add(field+".subject", "must be at most 255 characters")
	}
	if strings.ContainsAny(template.Subject, "\r\n") {
		validation.add(field+".subject", "must not contain a newline")
	}
	if template.TemplateURL != "" {
		parsed, err := url.ParseRequestURI(template.TemplateURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			validation.add(field+".templateUrl", "must be an absolute http or https URL")
		}
	}
	validateMailerTemplateVariables(template.Subject, field+".subject", validation)
}

func validateMailerTemplateVariables(value, field string, validation *ValidationError) {
	matches := mailerTemplateActionPattern.FindAllStringSubmatch(value, -1)
	if strings.Count(value, "{{") != len(matches) || strings.Count(value, "}}") != len(matches) {
		validation.add(field, "contains an invalid Go template action")
		return
	}
	for _, match := range matches {
		action := strings.TrimSpace(match[1])
		if _, allowed := allowedMailerTemplateVariables[action]; !allowed {
			validation.add(field, "may only use documented mailer template variables")
			return
		}
	}
}

func hasEnabledOAuth(providers map[string]contracts.OAuthProviderConfig) bool {
	for _, provider := range providers {
		if provider.Enabled {
			return true
		}
	}
	return false
}

func validatePhone(phone contracts.PhoneAuthConfig, validation *ValidationError) {
	if !phone.Enabled && phone.Provider == "" && len(phone.Fields) == 0 && !phone.SecretSet && phone.Secret.Action == "" {
		return
	}
	if phone.Provider != "twilio" && phone.Provider != "messagebird" && phone.Provider != "textlocal" {
		validation.add("auth.phone.provider", "must be twilio, messagebird, or textlocal")
	}
	for field := range phone.Fields {
		if !phoneFieldAllowed(phone.Provider, field) {
			validation.add("auth.phone.fields."+field, "is not supported for this provider")
		}
	}
	if phone.Enabled {
		for _, required := range phoneRequiredFields(phone.Provider) {
			if strings.TrimSpace(phone.Fields[required]) == "" {
				validation.add("auth.phone.fields."+required, "is required for this provider")
			}
		}
		if !phone.SecretSet && phone.Secret.Action != "replace" {
			validation.add("auth.phone.secret", "must retain an existing secret or replace it")
		}
		if phone.SecretSet && phone.Secret.Action != "retain" && phone.Secret.Action != "replace" {
			validation.add("auth.phone.secret", "an existing secret must use retain or replace")
		}
		if phone.Secret.Action == "remove" {
			validation.add("auth.phone.secret", "cannot be removed while Phone Auth is enabled")
		}
	}
	validateSecretInput(phone.Secret, "auth.phone.secret", validation)
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

func providerFieldAllowed(provider, field string) bool {
	switch provider {
	case "azure":
		return field == "tenantUrl"
	case "github":
		return field == "enterpriseUrl"
	case "gitlab":
		return field == "selfHostedUrl"
	case "keycloak":
		return field == "realmUrl"
	default:
		return false
	}
}

func phoneFieldAllowed(provider, field string) bool {
	switch provider {
	case "twilio":
		return field == "accountSid" || field == "messageServiceSid" || field == "verifySid"
	case "messagebird":
		return field == "originator"
	case "textlocal":
		return field == "sender"
	default:
		return false
	}
}

func phoneRequiredFields(provider string) []string {
	switch provider {
	case "twilio":
		return []string{"accountSid", "messageServiceSid"}
	case "messagebird":
		return []string{"originator"}
	case "textlocal":
		return []string{"sender"}
	default:
		return nil
	}
}

func validateStorage(storage contracts.StorageConfig, validation *ValidationError) {
	switch storage.Backend {
	case contracts.StorageBackendLocal:
		// A backend transition may carry an explicit remove command for the
		// previous encrypted key. secretMutations applies that command after
		// validation, so it must remain valid here.
		removingPreviousKey := storage.SecretAccessKeySet && storage.SecretAccessKey.Action == "remove"
		if storage.Bucket != "" || storage.Region != "" || storage.Endpoint != "" || storage.AccountID != "" || storage.AccessKeyID != "" || (storage.SecretAccessKeySet && !removingPreviousKey) || (storage.SecretAccessKey.Action != "" && !removingPreviousKey) {
			validation.add("storage.backend", "local storage cannot include object-storage credentials")
		}
	case contracts.StorageBackendS3, contracts.StorageBackendAWSS3, contracts.StorageBackendR2:
		if strings.TrimSpace(storage.Bucket) == "" {
			validation.add("storage.bucket", "is required for an object storage backend")
		}
		if storage.Backend != contracts.StorageBackendR2 && strings.TrimSpace(storage.Region) == "" {
			validation.add("storage.region", "is required for this object storage backend")
		}
		if strings.TrimSpace(storage.AccessKeyID) == "" {
			validation.add("storage.accessKeyId", "is required for an object storage backend")
		}
		if !storage.SecretAccessKeySet && storage.SecretAccessKey.Action != "replace" {
			validation.add("storage.secretAccessKey", "must retain an existing secret or replace it")
		}
		if storage.SecretAccessKey.Action == "remove" {
			validation.add("storage.secretAccessKey", "cannot be removed for an enabled object storage backend")
		}
		if storage.Backend == contracts.StorageBackendS3 && strings.TrimSpace(storage.Endpoint) == "" {
			validation.add("storage.endpoint", "is required for generic S3")
		}
		if storage.Backend == contracts.StorageBackendR2 && strings.TrimSpace(storage.AccountID) == "" {
			validation.add("storage.accountId", "is required for Cloudflare R2")
		}
		if storage.Backend == contracts.StorageBackendR2 && storage.Endpoint != "" {
			validation.add("storage.endpoint", "R2 endpoint is derived from accountId")
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
	if realtime.MaxConnections < 1 || realtime.MaxConnections > 100000 {
		validation.add("realtime.maxConnections", "must be between 1 and 100000")
	}
	if realtime.DatabasePoolSize < 1 || realtime.DatabasePoolSize > 10000 {
		validation.add("realtime.databasePoolSize", "must be between 1 and 10000")
	}
	if realtime.LogLevel != contracts.LogLevelDebug && realtime.LogLevel != contracts.LogLevelInfo && realtime.LogLevel != contracts.LogLevelWarn && realtime.LogLevel != contracts.LogLevelError {
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
	if database.MaxConnections < 1 || database.MaxConnections > 100000 {
		validation.add("database.maxConnections", "must be between 1 and 100000")
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
	if pooler.PoolSize < 1 || pooler.PoolSize > 100000 {
		validation.add("pooler.poolSize", "must be between 1 and 100000")
	}
	if pooler.MaxClientConnections < 1 || pooler.MaxClientConnections > 100000 {
		validation.add("pooler.maxClientConnections", "must be between 1 and 100000")
	}
}

func validateNetwork(network contracts.NetworkConfig, validation *ValidationError) {
	if network.Gateway != contracts.GatewayEnvoy && network.Gateway != contracts.GatewayKong {
		validation.add("network.gateway", "must be envoy or kong")
	}
	if network.HTTPSMode != contracts.HTTPSModeExternal && network.HTTPSMode != contracts.HTTPSModeCaddy {
		validation.add("network.httpsMode", "must be external or caddy; manual HTTPS is unsupported by the pinned renderer")
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
