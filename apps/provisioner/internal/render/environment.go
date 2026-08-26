package render

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"supabase-manager/internal/contracts"
	"supabase-manager/internal/templates"
)

func renderEnvironment(input Input) (string, string, error) {
	cfg := input.Configuration
	domain, siteURL := cfg.General.Domain, cfg.General.SiteURL
	if domain == "" {
		domain = input.Domain
	}
	if siteURL == "" {
		siteURL = input.SiteURL
	}
	values := map[string]string{
		"ANON_KEY": input.Secrets.AnonKey, "API_EXTERNAL_URL": "https://" + domain + "/auth/v1",
		"DASHBOARD_PASSWORD": input.Secrets.DashboardPassword, "JWT_SECRET": input.Secrets.JWTSecret,
		"POSTGRES_PASSWORD": input.Secrets.DatabasePassword, "SECRET_KEY_BASE": input.Secrets.SecretKeyBase,
		"SERVICE_ROLE_KEY": input.Secrets.ServiceRoleKey, "SITE_URL": siteURL,
		"SUPABASE_PUBLIC_URL": "https://" + domain, "VAULT_ENC_KEY": input.Secrets.VaultEncryptionKey,
		"PG_META_CRYPTO_KEY":       input.Secrets.SecretKeyBase,
		"ADDITIONAL_REDIRECT_URLS": strings.Join(cfg.Auth.RedirectURLs, ","),
		"JWT_EXPIRY":               strconv.Itoa(cfg.Auth.JWTExpiry), "DISABLE_SIGNUP": boolString(cfg.Auth.DisableSignup),
		"ENABLE_EMAIL_SIGNUP":      boolString(cfg.Auth.Email.Enabled),
		"ENABLE_EMAIL_AUTOCONFIRM": boolString(!cfg.Auth.Email.ConfirmEmail),
		"ENABLE_ANONYMOUS_USERS":   boolString(cfg.Auth.AnonymousSignIn),
		"ENABLE_PHONE_SIGNUP":      boolString(cfg.Auth.Phone.Enabled), "ENABLE_PHONE_AUTOCONFIRM": "false",
		"SMTP_ADMIN_EMAIL": cfg.Auth.SMTP.SenderEmail, "SMTP_HOST": cfg.Auth.SMTP.Host,
		"SMTP_PORT": strconv.Itoa(cfg.Auth.SMTP.Port), "SMTP_USER": cfg.Auth.SMTP.Username,
		"SMTP_PASS": input.RuntimeSecrets["smtp.password"], "SMTP_SENDER_NAME": cfg.Auth.SMTP.SenderName,
		"SECURE_EMAIL_CHANGE_ENABLED": boolString(cfg.Auth.Email.SecureEmailChange),
		"STORAGE_BACKEND":             storageBackend(cfg.Storage.Backend), "GLOBAL_S3_BUCKET": cfg.Storage.Bucket,
		"GLOBAL_S3_ENDPOINT": cfg.Storage.Endpoint, "GLOBAL_S3_FORCE_PATH_STYLE": boolString(cfg.Storage.ForcePathStyle),
		"AWS_ACCESS_KEY_ID": cfg.Storage.AccessKeyID, "AWS_SECRET_ACCESS_KEY": input.RuntimeSecrets["storage.secretAccessKey"],
		"REGION": cfg.Storage.Region, "FUNCTIONS_VERIFY_JWT": boolString(cfg.Functions.DefaultJWTVerification),
		"POOLER_PROXY_PORT_TRANSACTION": strconv.Itoa(cfg.Pooler.TransactionPort),
		"POOLER_DEFAULT_POOL_SIZE":      strconv.Itoa(cfg.Pooler.PoolSize), "POOLER_MAX_CLIENT_CONN": strconv.Itoa(cfg.Pooler.MaxClientConnections),
		"POOLER_POOL_MODE": "transaction", "POOLER_DB_POOL_SIZE": "5",
	}
	if cfg.Auth.Phone.Provider != "" {
		values["SMS_PROVIDER"] = cfg.Auth.Phone.Provider
	}
	if cfg.Auth.Phone.Fields != nil {
		for key, value := range cfg.Auth.Phone.Fields {
			values["SMS_"+strings.ToUpper(sanitizeName(key))] = value
		}
	}
	for _, name := range configuredProviderNames(cfg.Auth.OAuth) {
		provider := providerDefinitionFor(name)
		entry := cfg.Auth.OAuth[name]
		values[provider.Name+"_ENABLED"] = boolString(entry.Enabled)
		values[provider.Name+"_CLIENT_ID"] = entry.ClientID
		values[provider.Name+"_SECRET"] = input.RuntimeSecrets["oauth."+name+".secret"]
		for field, envKey := range provider.Special {
			if value, ok := entry.Fields[field]; ok {
				values[envKey] = value
			}
		}
		for field, value := range entry.Fields {
			key := provider.Name + "_" + strings.ToUpper(sanitizeName(field))
			if _, exists := values[key]; !exists {
				values[key] = value
			}
		}
	}
	if cfg.Auth.Phone.Enabled {
		values["PHONE_SECRET"] = input.RuntimeSecrets["phone.secret"]
	}

	env := renderDotEnv(string(templates.EnvExample()), values)
	functionValues := map[string]string{}
	for _, variable := range cfg.Functions.Variables {
		if !validEnvName(variable.Name) || isReservedRuntimeKey(variable.Name) {
			continue
		}
		if variable.ValueSet {
			functionValues[variable.Name] = input.RuntimeSecrets["functions."+variable.Name]
		}
	}
	return env, renderDotEnv("", functionValues), nil
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func storageBackend(value contracts.StorageBackend) string {
	if value == contracts.StorageBackendLocal || value == "" {
		return "file"
	}
	return string(value)
}

func sanitizeName(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func validEnvName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if !(unicode.IsLetter(r) || r == '_' || (i > 0 && unicode.IsDigit(r))) {
			return false
		}
	}
	return true
}

func escapeDotEnv(value string) string {
	for _, r := range value {
		if r == '\n' || r == '\r' || r == '\x00' || r == '\t' {
			return strconv.Quote(value)
		}
	}
	return value
}

func renderDotEnv(template string, overrides map[string]string) string {
	seen := make(map[string]bool, len(overrides))
	var b strings.Builder
	for _, line := range strings.Split(template, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if key, _, ok := strings.Cut(line, "="); ok {
				key = strings.TrimSpace(key)
				if value, replace := overrides[key]; replace {
					line = key + "=" + escapeDotEnv(value)
					seen[key] = true
				}
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	missing := make([]string, 0)
	for key := range overrides {
		if !seen[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	for _, key := range missing {
		b.WriteString(key + "=" + escapeDotEnv(overrides[key]) + "\n")
	}
	return b.String()
}

func injectAuthEnvironment(raw any, input Input) error {
	service, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("auth service is not a mapping")
	}
	env, ok := service["environment"].(map[string]any)
	if !ok {
		env = map[string]any{}
		service["environment"] = env
	}
	set := func(key, value string) { env[key] = "${" + value + "}" }
	set("GOTRUE_MAILER_SECURE_EMAIL_CHANGE_ENABLED", "SECURE_EMAIL_CHANGE_ENABLED")
	for _, name := range configuredProviderNames(input.Configuration.Auth.OAuth) {
		provider := providerDefinitionFor(name)
		set("GOTRUE_EXTERNAL_"+provider.Name+"_ENABLED", provider.Name+"_ENABLED")
		set("GOTRUE_EXTERNAL_"+provider.Name+"_CLIENT_ID", provider.Name+"_CLIENT_ID")
		set("GOTRUE_EXTERNAL_"+provider.Name+"_SECRET", provider.Name+"_SECRET")
		env["GOTRUE_EXTERNAL_"+provider.Name+"_REDIRECT_URI"] = "${API_EXTERNAL_URL}/callback"
		for _, envKey := range provider.Special {
			set("GOTRUE_EXTERNAL_"+provider.Name+"_"+strings.TrimPrefix(envKey, provider.Name+"_"), envKey)
		}
	}
	return nil
}

func isReservedRuntimeKey(key string) bool {
	reserved := map[string]bool{"POSTGRES_PASSWORD": true, "JWT_SECRET": true, "ANON_KEY": true, "SERVICE_ROLE_KEY": true, "DASHBOARD_PASSWORD": true, "SECRET_KEY_BASE": true, "VAULT_ENC_KEY": true, "SMTP_PASS": true, "AWS_SECRET_ACCESS_KEY": true, "PHONE_SECRET": true, "SUPABASE_URL": true, "SUPABASE_PUBLIC_URL": true, "FUNCTIONS_VERIFY_JWT": true}
	return reserved[key]
}
