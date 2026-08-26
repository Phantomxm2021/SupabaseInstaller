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
	"twilio":      {"accountSid": "SMS_TWILIO_ACCOUNT_SID", "messageServiceSid": "SMS_TWILIO_MESSAGE_SERVICE_SID", "verifySid": "SMS_TWILIO_VERIFY_MESSAGE_SERVICE_SID"},
	"messagebird": {"originator": "SMS_MESSAGEBIRD_ORIGINATOR"},
	"textlocal":   {"sender": "SMS_TEXTLOCAL_SENDER"},
}

var phoneSecretEnv = map[string][]string{
	"twilio":      {"GOTRUE_SMS_TWILIO_AUTH_TOKEN", "GOTRUE_SMS_TWILIO_VERIFY_AUTH_TOKEN"},
	"messagebird": {"GOTRUE_SMS_MESSAGEBIRD_ACCESS_KEY"},
	"textlocal":   {"GOTRUE_SMS_TEXTLOCAL_API_KEY"},
}

func renderEnvironment(input Input) (string, string, error) {
	cfg := input.Configuration
	if err := validateAuthConfiguration(cfg.Auth); err != nil {
		return "", "", err
	}
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
		"REALTIME_DB_ENC_KEY": firstNonempty(input.RuntimeSecrets["realtime.dbEncryptionKey"], input.Secrets.RealtimeDBEncryptionKey), "OPENAI_API_KEY": "",
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
		"GLOBAL_S3_PROTOCOL": storageProtocol(cfg.Storage.Endpoint),
		"AWS_ACCESS_KEY_ID":  "", "AWS_SECRET_ACCESS_KEY": "",
		"REGION": storageRegion(cfg.Storage), "S3_PROTOCOL_ENABLED": boolString(cfg.Storage.S3CompatibleAPI), "FUNCTIONS_VERIFY_JWT": boolString(cfg.Functions.DefaultJWTVerification),
		"POOLER_PROXY_PORT_TRANSACTION": strconv.Itoa(cfg.Pooler.TransactionPort),
		"POOLER_DEFAULT_POOL_SIZE":      strconv.Itoa(cfg.Pooler.PoolSize), "POOLER_MAX_CLIENT_CONN": strconv.Itoa(cfg.Pooler.MaxClientConnections),
		"POOLER_POOL_MODE": "transaction", "POOLER_DB_POOL_SIZE": strconv.Itoa(cfg.Pooler.PoolSize), "POOLER_TENANT_ID": input.Secrets.PoolerTenantID,
		"POSTGRES_MAX_CONNECTIONS": strconv.Itoa(cfg.Database.MaxConnections), "POSTGRES_SHARED_BUFFERS": cfg.Database.SharedBuffers,
		"REALTIME_MAX_CONNECTIONS": strconv.Itoa(cfg.Realtime.MaxConnections),
		"REALTIME_DB_POOL_SIZE":    strconv.Itoa(cfg.Realtime.DatabasePoolSize), "REALTIME_LOG_LEVEL": string(cfg.Realtime.LogLevel),
		"LOGFLARE_PUBLIC_ACCESS_TOKEN": firstNonempty(input.RuntimeSecrets[SecretLogsPublic], input.Secrets.LogflarePublicAccessToken), "LOGFLARE_PRIVATE_ACCESS_TOKEN": firstNonempty(input.RuntimeSecrets[SecretLogsPrivate], input.Secrets.LogflarePrivateAccessToken),
		"S3_PROTOCOL_ACCESS_KEY_ID": firstNonempty(input.RuntimeSecrets[SecretS3Access], input.Secrets.S3ProtocolAccessKeyID), "S3_PROTOCOL_ACCESS_KEY_SECRET": firstNonempty(input.RuntimeSecrets[SecretS3Secret], input.Secrets.S3ProtocolAccessKeySecret),
		"STORAGE_LOCAL_PATH":            "/var/lib/storage",
		"STORAGE_IMAGE_TRANSFORMATIONS": boolString(cfg.Services.Imgproxy),
		// Values present in the upstream example are deliberately overridden so
		// disabled optional consumers never inherit example credentials.
		"MINIO_ROOT_USER": "", "MINIO_ROOT_PASSWORD": "",
		"STORAGE_TENANT_ID": firstNonempty(input.ProjectID, input.Slug), "PROXY_DOMAIN": domain,
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
	if !cfg.Services.Realtime {
		values["REALTIME_DB_ENC_KEY"] = ""
	}
	if !cfg.Services.Supavisor {
		values["POOLER_TENANT_ID"] = ""
	}
	if !cfg.Services.Logs {
		values["LOGFLARE_PUBLIC_ACCESS_TOKEN"] = ""
		values["LOGFLARE_PRIVATE_ACCESS_TOKEN"] = ""
	}
	if !cfg.Storage.S3CompatibleAPI {
		values["S3_PROTOCOL_ACCESS_KEY_ID"] = ""
		values["S3_PROTOCOL_ACCESS_KEY_SECRET"] = ""
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
	if err := validateDotEnvValues(values); err != nil {
		return "", "", err
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
	if err := validateDotEnvValues(functionValues); err != nil {
		return "", "", err
	}
	return env, renderDotEnv("", functionValues), nil
}

func validateDotEnvValues(values map[string]string) error {
	for key, value := range values {
		for _, r := range value {
			if unicode.IsControl(r) || (r >= 0x80 && r <= 0x9f) {
				return fmt.Errorf("environment.%s: contains unsupported control character", key)
			}
		}
	}
	return nil
}

func requireRuntimeSecrets(input Input) error {
	cfg := input.Configuration
	required := []struct {
		enabled bool
		kind    string
	}{
		{cfg.Auth.SMTP.Enabled && cfg.Auth.SMTP.PasswordSet, SecretSMTPassword}, {cfg.Auth.Phone.Enabled && cfg.Auth.Phone.SecretSet, SecretPhone}, {cfg.Services.Storage && (cfg.Storage.Backend == contracts.StorageBackendS3 || cfg.Storage.Backend == contracts.StorageBackendAWSS3 || cfg.Storage.Backend == contracts.StorageBackendR2) && cfg.Storage.SecretAccessKeySet, SecretStorageKey},
	}
	for _, name := range configuredProviderNames(cfg.Auth.OAuth) {
		if cfg.Auth.OAuth[name].Enabled && cfg.Auth.OAuth[name].SecretSet {
			required = append(required, struct {
				enabled bool
				kind    string
			}{true, "oauth." + name + ".secret"})
		}
	}
	for _, variable := range cfg.Functions.Variables {
		if cfg.Services.Functions && variable.ValueSet {
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

func storageRegion(storage contracts.StorageConfig) string {
	if storage.Backend == contracts.StorageBackendR2 && strings.TrimSpace(storage.Region) == "" {
		return "auto"
	}
	return storage.Region
}

func storageProtocol(endpoint string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(endpoint)), "https://") {
		return "https"
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(endpoint)), "http://") {
		return "http"
	}
	return ""
}

func safeLocalPath(path string) string {
	// The host-side source is always the managed project path. This value is
	// only the container-side path consumed by storage-api.
	return "/var/lib/storage"
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
		if unicode.IsControl(r) || (r >= 0x80 && r <= 0x9f) || strings.ContainsRune("\\\"#$'", r) {
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
	set("GOTRUE_DISABLE_SIGNUP", "DISABLE_SIGNUP")
	set("GOTRUE_MAILER_SECURE_EMAIL_CHANGE_ENABLED", "SECURE_EMAIL_CHANGE_ENABLED")
	if input.Configuration.Auth.Phone.Enabled {
		set("GOTRUE_SMS_PROVIDER", "SMS_PROVIDER")
		for _, envKey := range phoneFieldKeys[input.Configuration.Auth.Phone.Provider] {
			set("GOTRUE_"+envKey, envKey)
		}
		for _, secretKey := range phoneSecretEnv[input.Configuration.Auth.Phone.Provider] {
			env[secretKey] = "${PHONE_SECRET}"
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
		env["GLOBAL_S3_PROTOCOL"] = "${GLOBAL_S3_PROTOCOL}"
		env["GLOBAL_S3_FORCE_PATH_STYLE"] = "${GLOBAL_S3_FORCE_PATH_STYLE}"
		env["AWS_ACCESS_KEY_ID"] = "${AWS_ACCESS_KEY_ID}"
		env["AWS_SECRET_ACCESS_KEY"] = "${AWS_SECRET_ACCESS_KEY}"
		env["REGION"] = "${REGION}"
		env["FILE_STORAGE_BACKEND_PATH"] = "${STORAGE_LOCAL_PATH}"
		env["S3_PROTOCOL_ACCESS_KEY_ID"] = "${S3_PROTOCOL_ACCESS_KEY_ID}"
		env["S3_PROTOCOL_ACCESS_KEY_SECRET"] = "${S3_PROTOCOL_ACCESS_KEY_SECRET}"
		env["S3_PROTOCOL_ENABLED"] = "${S3_PROTOCOL_ENABLED}"
		env["ENABLE_IMAGE_TRANSFORMATION"] = "${STORAGE_IMAGE_TRANSFORMATIONS}"
		if input.Configuration.Services.Imgproxy {
			env["IMGPROXY_URL"] = "http://imgproxy:5001"
		} else {
			env["IMGPROXY_URL"] = ""
		}
		if input.Configuration.Storage.Backend == "" || input.Configuration.Storage.Backend == contracts.StorageBackendLocal {
			services["storage"].(map[string]any)["volumes"] = []string{"./volumes/storage:/var/lib/storage:z"}
		}
	}
	if env := setEnv("functions"); env != nil {
		if _, ok := services["functions"]; ok {
			// Compose runs with the stable project directory; point explicitly at
			// the atomically selected private env file in the current generation.
			services["functions"].(map[string]any)["env_file"] = []string{"./.manager-runtime/current/.env.functions"}
		}
	}
	if env := setEnv("realtime"); env != nil {
		env["DB_ENC_KEY"] = "${REALTIME_DB_ENC_KEY}"
		env["MAX_CONNECTIONS"] = "${REALTIME_MAX_CONNECTIONS}"
		env["DB_POOL_SIZE"] = "${REALTIME_DB_POOL_SIZE}"
		env["LOG_LEVEL"] = "${REALTIME_LOG_LEVEL}"
	}
	if env := setEnv("db"); env != nil {
		command := []any{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf", "-c", "log_min_messages=fatal"}
		if input.Configuration.Database.MaxConnections > 0 {
			command = append(command, "-c", "max_connections="+strconv.Itoa(input.Configuration.Database.MaxConnections))
		}
		if strings.TrimSpace(input.Configuration.Database.SharedBuffers) != "" {
			command = append(command, "-c", "shared_buffers="+input.Configuration.Database.SharedBuffers)
		}
		services["db"].(map[string]any)["command"] = command
	}
	if service, ok := services["supavisor"].(map[string]any); ok {
		sessionPort := input.Configuration.Pooler.SessionPort
		if sessionPort == 0 {
			sessionPort = input.Configuration.Network.PoolerPort
		}
		ports := make([]string, 0, 2)
		if sessionPort > 0 {
			ports = append(ports, fmt.Sprintf("127.0.0.1:%d:5432", sessionPort))
		}
		if input.Configuration.Pooler.TransactionPort > 0 {
			ports = append(ports, fmt.Sprintf("127.0.0.1:%d:6543", input.Configuration.Pooler.TransactionPort))
		}
		if len(ports) > 0 {
			service["ports"] = ports
		}
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
