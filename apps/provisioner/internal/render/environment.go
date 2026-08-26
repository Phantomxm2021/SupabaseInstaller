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

const (
	SecretSMTPassword = "smtp.password"
	SecretPhone       = "phone.secret"
	SecretStorageKey  = "storage.secretAccessKey"
	SecretLogsPublic  = "logs.publicAccessToken"
	SecretLogsPrivate = "logs.privateAccessToken"
	SecretS3Access    = "storage.s3Protocol.accessKey"
	SecretS3Secret    = "storage.s3Protocol.secret"
)

var phoneFieldKeys = map[string]map[string]string{
	"twilio":      {"accountSid": "SMS_TWILIO_ACCOUNT_SID", "authToken": "SMS_TWILIO_AUTH_TOKEN", "messageServiceSid": "SMS_TWILIO_MESSAGE_SERVICE_SID"},
	"messagebird": {"accessKey": "SMS_MESSAGEBIRD_ACCESS_KEY", "originator": "SMS_MESSAGEBIRD_ORIGINATOR"},
}

func renderEnvironment(input Input) (string, string, error) {
	cfg := input.Configuration
	if err := requireRuntimeSecrets(input); err != nil {
		return "", "", err
	}
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
		"PG_META_CRYPTO_KEY":  input.Secrets.SecretKeyBase,
		"REALTIME_DB_ENC_KEY": "", "OPENAI_API_KEY": "",
		"ADDITIONAL_REDIRECT_URLS": strings.Join(cfg.Auth.RedirectURLs, ","),
		"JWT_EXPIRY":               strconv.Itoa(cfg.Auth.JWTExpiry), "DISABLE_SIGNUP": boolString(cfg.Auth.DisableSignup),
		"ENABLE_EMAIL_SIGNUP":      boolString(cfg.Auth.Email.Enabled),
		"ENABLE_EMAIL_AUTOCONFIRM": boolString(!cfg.Auth.Email.ConfirmEmail),
		"ENABLE_ANONYMOUS_USERS":   boolString(cfg.Auth.AnonymousSignIn),
		"ENABLE_PHONE_SIGNUP":      boolString(cfg.Auth.Phone.Enabled), "ENABLE_PHONE_AUTOCONFIRM": "false",
		"SMTP_ADMIN_EMAIL": cfg.Auth.SMTP.SenderEmail, "SMTP_HOST": cfg.Auth.SMTP.Host,
		"SMTP_PORT": strconv.Itoa(cfg.Auth.SMTP.Port), "SMTP_USER": cfg.Auth.SMTP.Username,
		"SMTP_PASS": "", "SMTP_SENDER_NAME": cfg.Auth.SMTP.SenderName,
		"SECURE_EMAIL_CHANGE_ENABLED": boolString(cfg.Auth.Email.SecureEmailChange),
		"STORAGE_BACKEND":             storageBackend(cfg.Storage.Backend), "GLOBAL_S3_BUCKET": cfg.Storage.Bucket,
		"GLOBAL_S3_ENDPOINT": cfg.Storage.Endpoint, "GLOBAL_S3_FORCE_PATH_STYLE": boolString(cfg.Storage.ForcePathStyle),
		"AWS_ACCESS_KEY_ID": "", "AWS_SECRET_ACCESS_KEY": "",
		"REGION": cfg.Storage.Region, "FUNCTIONS_VERIFY_JWT": boolString(cfg.Functions.DefaultJWTVerification),
		"POOLER_PROXY_PORT_TRANSACTION": strconv.Itoa(cfg.Pooler.TransactionPort),
		"POOLER_DEFAULT_POOL_SIZE":      strconv.Itoa(cfg.Pooler.PoolSize), "POOLER_MAX_CLIENT_CONN": strconv.Itoa(cfg.Pooler.MaxClientConnections),
		"POOLER_POOL_MODE": "transaction", "POOLER_DB_POOL_SIZE": "5",
		"POSTGRES_MAX_CONNECTIONS": strconv.Itoa(cfg.Database.MaxConnections), "POSTGRES_SHARED_BUFFERS": cfg.Database.SharedBuffers,
		"POSTGRES_EXTENSIONS": strings.Join(cfg.Database.Extensions, ","), "REALTIME_MAX_CONNECTIONS": strconv.Itoa(cfg.Realtime.MaxConnections),
		"REALTIME_DB_POOL_SIZE": strconv.Itoa(cfg.Realtime.DatabasePoolSize), "REALTIME_LOG_LEVEL": string(cfg.Realtime.LogLevel),
		"LOGFLARE_PUBLIC_ACCESS_TOKEN": "", "LOGFLARE_PRIVATE_ACCESS_TOKEN": "",
		"S3_PROTOCOL_ACCESS_KEY_ID": "", "S3_PROTOCOL_ACCESS_KEY_SECRET": "",
		"STORAGE_LOCAL_PATH":            safeLocalPath(cfg.Storage.LocalPath),
		"STORAGE_IMAGE_TRANSFORMATIONS": boolString(cfg.Services.Imgproxy),
	}
	if cfg.Storage.Backend == contracts.StorageBackendR2 && cfg.Storage.Endpoint == "" && cfg.Storage.AccountID != "" {
		values["GLOBAL_S3_ENDPOINT"] = "https://" + cfg.Storage.AccountID + ".r2.cloudflarestorage.com"
	}
	if cfg.Auth.SMTP.Enabled {
		values["SMTP_PASS"] = input.RuntimeSecrets[SecretSMTPassword]
	}
	if cfg.Storage.Backend == contracts.StorageBackendS3 || cfg.Storage.Backend == contracts.StorageBackendAWSS3 || cfg.Storage.Backend == contracts.StorageBackendR2 {
		values["AWS_ACCESS_KEY_ID"] = cfg.Storage.AccessKeyID
		values["AWS_SECRET_ACCESS_KEY"] = input.RuntimeSecrets[SecretStorageKey]
	}
	if cfg.Services.Logs {
		values["LOGFLARE_PUBLIC_ACCESS_TOKEN"] = input.RuntimeSecrets[SecretLogsPublic]
		values["LOGFLARE_PRIVATE_ACCESS_TOKEN"] = input.RuntimeSecrets[SecretLogsPrivate]
	}
	if cfg.Storage.S3CompatibleAPI {
		values["S3_PROTOCOL_ACCESS_KEY_ID"] = input.RuntimeSecrets[SecretS3Access]
		values["S3_PROTOCOL_ACCESS_KEY_SECRET"] = input.RuntimeSecrets[SecretS3Secret]
	}
	if cfg.Auth.Phone.Provider != "" {
		values["SMS_PROVIDER"] = cfg.Auth.Phone.Provider
	}
	if cfg.Auth.Phone.Fields != nil {
		for key, value := range cfg.Auth.Phone.Fields {
			if envKey, ok := phoneFieldKeys[cfg.Auth.Phone.Provider][key]; ok {
				values[envKey] = value
			}
		}
	}
	for _, name := range configuredProviderNames(cfg.Auth.OAuth) {
		provider := providerDefinitionFor(name)
		entry := cfg.Auth.OAuth[name]
		values[provider.Name+"_ENABLED"] = boolString(entry.Enabled)
		values[provider.Name+"_CLIENT_ID"] = entry.ClientID
		if entry.Enabled {
			values[provider.Name+"_SECRET"] = input.RuntimeSecrets["oauth."+name+".secret"]
		} else {
			values[provider.Name+"_SECRET"] = ""
		}
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
		values["PHONE_SECRET"] = input.RuntimeSecrets[SecretPhone]
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

func requireRuntimeSecrets(input Input) error {
	cfg := input.Configuration
	required := []struct {
		enabled bool
		kind    string
	}{
		{cfg.Auth.SMTP.PasswordSet, SecretSMTPassword}, {cfg.Auth.Phone.SecretSet, SecretPhone}, {cfg.Storage.SecretAccessKeySet, SecretStorageKey},
		{cfg.Services.Logs, SecretLogsPublic}, {cfg.Services.Logs, SecretLogsPrivate}, {cfg.Storage.S3CompatibleAPI, SecretS3Access}, {cfg.Storage.S3CompatibleAPI, SecretS3Secret},
	}
	for _, name := range configuredProviderNames(cfg.Auth.OAuth) {
		if cfg.Auth.OAuth[name].SecretSet {
			required = append(required, struct {
				enabled bool
				kind    string
			}{true, "oauth." + name + ".secret"})
		}
	}
	for _, variable := range cfg.Functions.Variables {
		if variable.ValueSet {
			required = append(required, struct {
				enabled bool
				kind    string
			}{true, "functions." + variable.Name})
		}
	}
	for _, item := range required {
		if item.enabled && strings.TrimSpace(input.RuntimeSecrets[item.kind]) == "" {
			return fmt.Errorf("missing runtime secret %s", item.kind)
		}
	}
	return nil
}

func safeLocalPath(path string) string {
	if path == "" {
		return "/var/lib/storage"
	}
	if strings.HasPrefix(path, "/") && !strings.Contains(path, "..") {
		return path
	}
	return "/var/lib/storage"
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
	return "s3"
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
	needsQuote := value == "" || strings.TrimSpace(value) != value
	for _, r := range value {
		if r < 0x20 || r == 0x7f || strings.ContainsRune("\\\"#$'", r) {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return value
	}
	return strconv.Quote(strings.ReplaceAll(value, "$", "$$"))
}

func renderDotEnv(template string, overrides map[string]string) string {
	if template == "" && len(overrides) == 0 {
		return ""
	}
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
	if input.Configuration.Auth.Phone.Enabled {
		set("GOTRUE_SMS_PROVIDER", "SMS_PROVIDER")
		for _, envKey := range phoneFieldKeys[input.Configuration.Auth.Phone.Provider] {
			set("GOTRUE_SMS_"+strings.TrimPrefix(envKey, "SMS_"), envKey)
		}
		if input.Configuration.Auth.Phone.SecretSet {
			env["GOTRUE_SMS_"+strings.ToUpper(input.Configuration.Auth.Phone.Provider)+"_AUTH_TOKEN"] = "${PHONE_SECRET}"
		}
	}
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

func injectServiceConfiguration(services map[string]any, input Input) error {
	setEnv := func(name string) map[string]any {
		service, ok := services[name].(map[string]any)
		if !ok {
			return nil
		}
		env, ok := service["environment"].(map[string]any)
		if !ok {
			env = map[string]any{}
			service["environment"] = env
		}
		return env
	}
	if env := setEnv("storage"); env != nil {
		env["STORAGE_BACKEND"] = "${STORAGE_BACKEND}"
		env["GLOBAL_S3_BUCKET"] = "${GLOBAL_S3_BUCKET}"
		env["GLOBAL_S3_ENDPOINT"] = "${GLOBAL_S3_ENDPOINT}"
		env["GLOBAL_S3_FORCE_PATH_STYLE"] = "${GLOBAL_S3_FORCE_PATH_STYLE}"
		env["AWS_ACCESS_KEY_ID"] = "${AWS_ACCESS_KEY_ID}"
		env["AWS_SECRET_ACCESS_KEY"] = "${AWS_SECRET_ACCESS_KEY}"
		env["REGION"] = "${REGION}"
		env["FILE_STORAGE_BACKEND_PATH"] = "${STORAGE_LOCAL_PATH}"
		env["S3_PROTOCOL_ACCESS_KEY_ID"] = "${S3_PROTOCOL_ACCESS_KEY_ID}"
		env["S3_PROTOCOL_ACCESS_KEY_SECRET"] = "${S3_PROTOCOL_ACCESS_KEY_SECRET}"
		env["ENABLE_IMAGE_TRANSFORMATION"] = "${STORAGE_IMAGE_TRANSFORMATIONS}"
		if input.Configuration.Services.Imgproxy {
			env["IMGPROXY_URL"] = "http://imgproxy:5001"
		} else {
			env["IMGPROXY_URL"] = ""
		}
	}
	if env := setEnv("functions"); env != nil {
		if _, ok := services["functions"]; ok {
			services["functions"].(map[string]any)["env_file"] = []string{"./.env.functions"}
		}
	}
	if env := setEnv("realtime"); env != nil {
		env["MAX_CONNECTIONS"] = "${REALTIME_MAX_CONNECTIONS}"
		env["DB_POOL_SIZE"] = "${REALTIME_DB_POOL_SIZE}"
		env["LOG_LEVEL"] = "${REALTIME_LOG_LEVEL}"
	}
	if env := setEnv("db"); env != nil {
		env["POSTGRES_MAX_CONNECTIONS"] = "${POSTGRES_MAX_CONNECTIONS}"
		env["POSTGRES_SHARED_BUFFERS"] = "${POSTGRES_SHARED_BUFFERS}"
		env["POSTGRES_EXTENSIONS"] = "${POSTGRES_EXTENSIONS}"
	}
	if service, ok := services["supavisor"].(map[string]any); ok {
		service["ports"] = []string{fmt.Sprintf("127.0.0.1:%d:5432", input.Configuration.Pooler.SessionPort), fmt.Sprintf("127.0.0.1:%d:6543", input.Configuration.Pooler.TransactionPort)}
	}
	if service, ok := services["studio"].(map[string]any); ok && input.Configuration.Network.StudioPort > 0 {
		service["ports"] = []string{fmt.Sprintf("127.0.0.1:%d:3000", input.Configuration.Network.StudioPort)}
	}
	if input.Configuration.Storage.Backend == contracts.StorageBackendR2 && input.Configuration.Storage.Endpoint == "" && input.Configuration.Storage.AccountID != "" {
		if env := setEnv("storage"); env != nil {
			env["GLOBAL_S3_ENDPOINT"] = "${GLOBAL_S3_ENDPOINT}"
		}
	}
	return nil
}

func isReservedRuntimeKey(key string) bool {
	reserved := map[string]bool{"POSTGRES_PASSWORD": true, "JWT_SECRET": true, "ANON_KEY": true, "SERVICE_ROLE_KEY": true, "DASHBOARD_PASSWORD": true, "SECRET_KEY_BASE": true, "VAULT_ENC_KEY": true, "SMTP_PASS": true, "AWS_SECRET_ACCESS_KEY": true, "PHONE_SECRET": true, "SUPABASE_URL": true, "SUPABASE_PUBLIC_URL": true, "FUNCTIONS_VERIFY_JWT": true}
	return reserved[key]
}
