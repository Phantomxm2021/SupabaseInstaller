package render

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"supabase-manager/internal/authkeys"
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

const defaultStorageUploadFileSizeLimit int64 = 50 * 1024 * 1024

func effectiveAuthSiteURL(general contracts.GeneralConfig) string {
	if strings.TrimSpace(general.AuthSiteURL) != "" {
		return general.AuthSiteURL
	}
	return "https://" + general.Domain
}

func effectiveJWTExpiry(value int) int {
	if value == 0 {
		return 3600
	}
	return value
}

func renderEnvironment(input Input) (string, string, error) {
	cfg := input.Configuration
	storageFileSizeLimit := cfg.Storage.UploadFileSizeLimit
	if storageFileSizeLimit == 0 {
		storageFileSizeLimit = defaultStorageUploadFileSizeLimit
	}
	if err := validateAuthConfiguration(cfg.Auth); err != nil {
		return "", "", err
	}
	if err := requireRuntimeSecrets(input); err != nil {
		return "", "", err
	}
	bundle := authkeys.Bundle{SupabasePublishableKey: input.Secrets.SupabasePublishableKey, SupabaseSecretKey: input.Secrets.SupabaseSecretKey, AnonKeyAsymmetric: input.Secrets.AnonKeyAsymmetric, ServiceRoleKeyAsymmetric: input.Secrets.ServiceRoleKeyAsymmetric, JWTKeys: input.Secrets.JWTKeys, JWTJWKS: input.Secrets.JWTJWKS}
	keyCount := 0
	for _, value := range []string{bundle.SupabasePublishableKey, bundle.SupabaseSecretKey, bundle.AnonKeyAsymmetric, bundle.ServiceRoleKeyAsymmetric, bundle.JWTKeys, bundle.JWTJWKS} {
		if value != "" {
			keyCount++
		}
	}
	if keyCount != 0 && (keyCount != 6 || bundle.Validate(input.Secrets.JWTSecret) != nil) {
		return "", "", fmt.Errorf("invalid asymmetric auth key bundle")
	}
	domain := cfg.General.Domain
	authSiteURL := effectiveAuthSiteURL(cfg.General)
	values := map[string]string{
		"ANON_KEY": input.Secrets.AnonKey, "API_EXTERNAL_URL": "https://" + domain + "/auth/v1",
		"SUPABASE_PUBLISHABLE_KEY": input.Secrets.SupabasePublishableKey, "SUPABASE_SECRET_KEY": input.Secrets.SupabaseSecretKey,
		"ANON_KEY_ASYMMETRIC": input.Secrets.AnonKeyAsymmetric, "SERVICE_ROLE_KEY_ASYMMETRIC": input.Secrets.ServiceRoleKeyAsymmetric,
		"JWT_KEYS": input.Secrets.JWTKeys, "JWT_JWKS": input.Secrets.JWTJWKS,
		"DASHBOARD_USERNAME":     firstNonempty(strings.TrimSpace(cfg.General.StudioUsername), "supabase"),
		"STUDIO_DEFAULT_PROJECT": firstNonempty(input.ProjectName, "Default Server"),
		"DASHBOARD_PASSWORD":     input.Secrets.DashboardPassword, "JWT_SECRET": input.Secrets.JWTSecret,
		"POSTGRES_PASSWORD": input.Secrets.DatabasePassword, "SECRET_KEY_BASE": input.Secrets.SecretKeyBase,
		"SERVICE_ROLE_KEY": input.Secrets.ServiceRoleKey, "SITE_URL": authSiteURL,
		"SUPABASE_PUBLIC_URL": "https://" + domain, "VAULT_ENC_KEY": input.Secrets.VaultEncryptionKey,
		"PG_META_CRYPTO_KEY":  input.Secrets.SecretKeyBase,
		"REALTIME_DB_ENC_KEY": firstNonempty(input.RuntimeSecrets["realtime.dbEncryptionKey"], input.Secrets.RealtimeDBEncryptionKey), "OPENAI_API_KEY": "",
		"ADDITIONAL_REDIRECT_URLS": strings.Join(cfg.Auth.RedirectURLs, ","),
		"JWT_EXPIRY":               strconv.Itoa(effectiveJWTExpiry(cfg.Auth.JWTExpiry)), "DISABLE_SIGNUP": boolString(cfg.Auth.DisableSignup),
		"ENABLE_EMAIL_SIGNUP":      boolString(cfg.Auth.Email.Enabled),
		"ENABLE_EMAIL_AUTOCONFIRM": boolString(!cfg.Auth.Email.ConfirmEmail),
		"ENABLE_ANONYMOUS_USERS":   boolString(cfg.Auth.AnonymousSignIn),
		"MANUAL_LINKING_ENABLED":   boolString(cfg.Auth.ManualLinking),
		"ENABLE_PHONE_SIGNUP":      boolString(cfg.Auth.Phone.Enabled), "ENABLE_PHONE_AUTOCONFIRM": "false",
		"SMTP_ADMIN_EMAIL": cfg.Auth.SMTP.SenderEmail, "SMTP_HOST": cfg.Auth.SMTP.Host,
		"SMTP_PORT": strconv.Itoa(cfg.Auth.SMTP.Port), "SMTP_USER": cfg.Auth.SMTP.Username,
		"SMTP_PASS": "", "SMTP_SENDER_NAME": cfg.Auth.SMTP.SenderName,
		"SECURE_EMAIL_CHANGE_ENABLED":    boolString(cfg.Auth.Email.SecureEmailChange),
		"SECURE_PASSWORD_CHANGE_ENABLED": boolString(cfg.Auth.Email.SecurePasswordChange),
		"REQUIRE_CURRENT_PASSWORD":       boolString(cfg.Auth.Email.RequireCurrentPassword),
		"PREVENT_LEAKED_PASSWORDS":       boolString(cfg.Auth.Email.PreventLeakedPasswords),
		"PASSWORD_MIN_LENGTH":            strconv.Itoa(emailIntOrDefault(cfg.Auth.Email.MinimumPasswordLength, 6)),
		"PASSWORD_REQUIRED_CHARACTERS":   cfg.Auth.Email.PasswordRequirements,
		"MAILER_OTP_EXP":                 strconv.Itoa(emailIntOrDefault(cfg.Auth.Email.EmailOTPExpiration, 3600)),
		"MAILER_OTP_LENGTH":              strconv.Itoa(emailIntOrDefault(cfg.Auth.Email.EmailOTPLength, 8)),
		"RATE_LIMIT_EMAIL_SENT":          strconv.Itoa(rateLimitOrDefault(cfg.Auth.RateLimits.EmailSent, 30)),
		"RATE_LIMIT_SMS_SENT":            strconv.Itoa(rateLimitOrDefault(cfg.Auth.RateLimits.SMSSent, 30)),
		"RATE_LIMIT_TOKEN_REFRESH":       strconv.Itoa(rateLimitOrDefault(cfg.Auth.RateLimits.TokenRefresh, 150)),
		"RATE_LIMIT_VERIFY":              strconv.Itoa(rateLimitOrDefault(cfg.Auth.RateLimits.TokenVerification, 30)),
		"RATE_LIMIT_ANONYMOUS_USERS":     strconv.Itoa(rateLimitOrDefault(cfg.Auth.RateLimits.AnonymousUsers, 30)),
		"RATE_LIMIT_OTP":                 strconv.Itoa(rateLimitOrDefault(cfg.Auth.RateLimits.SignupsAndSignins, 30)),
		"MFA_TOTP_ENROLL_ENABLED":        boolString(cfg.Auth.MFA.TOTPEnrollEnabled),
		"MFA_TOTP_VERIFY_ENABLED":        boolString(cfg.Auth.MFA.TOTPVerifyEnabled),
		"MFA_PHONE_ENROLL_ENABLED":       boolString(cfg.Auth.MFA.PhoneEnrollEnabled),
		"MFA_PHONE_VERIFY_ENABLED":       boolString(cfg.Auth.MFA.PhoneVerifyEnabled),
		"MFA_MAX_ENROLLED_FACTORS":       strconv.Itoa(rateLimitOrDefault(cfg.Auth.MFA.MaxEnrolledFactors, 10)),
		"MFA_PHONE_OTP_LENGTH":           strconv.Itoa(rateLimitOrDefault(cfg.Auth.MFA.PhoneOTPLength, 6)),
		"STORAGE_BACKEND":                storageBackend(cfg.Storage.Backend), "GLOBAL_S3_BUCKET": cfg.Storage.Bucket,
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
		"STORAGE_FILE_SIZE_LIMIT":       strconv.FormatInt(storageFileSizeLimit, 10),
		"STORAGE_IMAGE_TRANSFORMATIONS": boolString(cfg.Services.Imgproxy),
		// Values present in the upstream example are deliberately overridden so
		// disabled optional consumers never inherit example credentials.
		"MINIO_ROOT_USER": "", "MINIO_ROOT_PASSWORD": "",
		"STORAGE_TENANT_ID": firstNonempty(input.ProjectID, input.Slug), "PROXY_DOMAIN": domain,
	}
	if cfg.Storage.Backend == contracts.StorageBackendR2 {
		values["GLOBAL_S3_FORCE_PATH_STYLE"] = "true"
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
		for field, envKey := range provider.Fields {
			if value, ok := entry.Fields[field]; ok {
				values[envKey] = value
			} else if oauthBooleanField(field) {
				// GoTrue parses these environment variables strictly as booleans.
				// The Compose renderer wires every provider field, so leaving an
				// absent optional value unresolved produces an empty string and
				// crashes Auth during startup. Make the safe default explicit.
				values[envKey] = "false"
			}
		}
		for field, value := range entry.Fields {
			if _, mapped := provider.Special[field]; mapped {
				continue
			}
			if _, mapped := provider.Fields[field]; mapped {
				continue
			}
			key := provider.Name + "_" + strings.ToUpper(sanitizeName(field))
			if _, exists := values[key]; !exists {
				values[key] = value
			}
		}
	}
	if cfg.Auth.Phone.Enabled {
		values["PHONE_SECRET"] = input.RuntimeSecrets[SecretPhone]
	}
	for key, value := range mailerEnvironmentValues(cfg.Auth.Mailer) {
		values[key] = value
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

func oauthBooleanField(field string) bool {
	return field == "skipNonceChecks" || field == "allowUsersWithoutEmail"
}

func mailerEnvironmentValues(mailer contracts.MailerConfig) map[string]string {
	values := make(map[string]string)
	setTemplate := func(name string, template contracts.EmailTemplateConfig) {
		values["MAILER_SUBJECT_"+name] = template.Subject
		values["MAILER_TEMPLATE_"+name] = "http://auth-templates:8080/" + strings.ToLower(name) + ".html"
	}
	setTemplate("CONFIRMATION", mailer.Templates.Confirmation)
	setTemplate("INVITE", mailer.Templates.Invite)
	setTemplate("MAGIC_LINK", mailer.Templates.MagicLink)
	setTemplate("EMAIL_CHANGE", mailer.Templates.EmailChange)
	setTemplate("RECOVERY", mailer.Templates.Recovery)
	setTemplate("REAUTHENTICATION", mailer.Templates.Reauthentication)
	setNotification := func(name string, notification contracts.EmailNotificationConfig) {
		values["MAILER_NOTIFICATIONS_"+name+"_ENABLED"] = boolString(notification.Enabled)
		setTemplate(name+"_NOTIFICATION", notification.Template)
	}
	setNotification("PASSWORD_CHANGED", mailer.Notifications.PasswordChanged)
	setNotification("EMAIL_CHANGED", mailer.Notifications.EmailChanged)
	setNotification("PHONE_CHANGED", mailer.Notifications.PhoneChanged)
	setNotification("IDENTITY_LINKED", mailer.Notifications.IdentityLinked)
	setNotification("IDENTITY_UNLINKED", mailer.Notifications.IdentityUnlinked)
	setNotification("MFA_FACTOR_ENROLLED", mailer.Notifications.MFAFactorEnrolled)
	setNotification("MFA_FACTOR_UNENROLLED", mailer.Notifications.MFAFactorUnenrolled)
	return values
}

func mailerTemplateFiles(mailer contracts.MailerConfig) map[string][]byte {
	files := make(map[string][]byte)
	set := func(name string, template contracts.EmailTemplateConfig) {
		files[strings.ToLower(name)+".html"] = []byte(template.Body)
	}
	set("CONFIRMATION", mailer.Templates.Confirmation)
	set("INVITE", mailer.Templates.Invite)
	set("MAGIC_LINK", mailer.Templates.MagicLink)
	set("EMAIL_CHANGE", mailer.Templates.EmailChange)
	set("RECOVERY", mailer.Templates.Recovery)
	set("REAUTHENTICATION", mailer.Templates.Reauthentication)
	set("PASSWORD_CHANGED_NOTIFICATION", mailer.Notifications.PasswordChanged.Template)
	set("EMAIL_CHANGED_NOTIFICATION", mailer.Notifications.EmailChanged.Template)
	set("PHONE_CHANGED_NOTIFICATION", mailer.Notifications.PhoneChanged.Template)
	set("IDENTITY_LINKED_NOTIFICATION", mailer.Notifications.IdentityLinked.Template)
	set("IDENTITY_UNLINKED_NOTIFICATION", mailer.Notifications.IdentityUnlinked.Template)
	set("MFA_FACTOR_ENROLLED_NOTIFICATION", mailer.Notifications.MFAFactorEnrolled.Template)
	set("MFA_FACTOR_UNENROLLED_NOTIFICATION", mailer.Notifications.MFAFactorUnenrolled.Template)
	return files
}

func rateLimitOrDefault(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func emailIntOrDefault(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
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
	set("GOTRUE_SECURITY_MANUAL_LINKING_ENABLED", "MANUAL_LINKING_ENABLED")
	set("GOTRUE_MAILER_SECURE_EMAIL_CHANGE_ENABLED", "SECURE_EMAIL_CHANGE_ENABLED")
	set("GOTRUE_SECURITY_UPDATE_PASSWORD_REQUIRE_REAUTHENTICATION", "SECURE_PASSWORD_CHANGE_ENABLED")
	set("GOTRUE_SECURITY_UPDATE_PASSWORD_REQUIRE_CURRENT_PASSWORD", "REQUIRE_CURRENT_PASSWORD")
	set("GOTRUE_PASSWORD_HIBP_ENABLED", "PREVENT_LEAKED_PASSWORDS")
	set("GOTRUE_PASSWORD_MIN_LENGTH", "PASSWORD_MIN_LENGTH")
	set("GOTRUE_PASSWORD_REQUIRED_CHARACTERS", "PASSWORD_REQUIRED_CHARACTERS")
	set("GOTRUE_MAILER_OTP_EXP", "MAILER_OTP_EXP")
	set("GOTRUE_MAILER_OTP_LENGTH", "MAILER_OTP_LENGTH")
	for gotrueKey, envKey := range map[string]string{
		"GOTRUE_RATE_LIMIT_EMAIL_SENT": "RATE_LIMIT_EMAIL_SENT", "GOTRUE_RATE_LIMIT_SMS_SENT": "RATE_LIMIT_SMS_SENT",
		"GOTRUE_RATE_LIMIT_TOKEN_REFRESH": "RATE_LIMIT_TOKEN_REFRESH", "GOTRUE_RATE_LIMIT_VERIFY": "RATE_LIMIT_VERIFY",
		"GOTRUE_RATE_LIMIT_ANONYMOUS_USERS": "RATE_LIMIT_ANONYMOUS_USERS", "GOTRUE_RATE_LIMIT_OTP": "RATE_LIMIT_OTP",
		"GOTRUE_MFA_TOTP_ENROLL_ENABLED": "MFA_TOTP_ENROLL_ENABLED", "GOTRUE_MFA_TOTP_VERIFY_ENABLED": "MFA_TOTP_VERIFY_ENABLED",
		"GOTRUE_MFA_PHONE_ENROLL_ENABLED": "MFA_PHONE_ENROLL_ENABLED", "GOTRUE_MFA_PHONE_VERIFY_ENABLED": "MFA_PHONE_VERIFY_ENABLED",
		"GOTRUE_MFA_MAX_ENROLLED_FACTORS": "MFA_MAX_ENROLLED_FACTORS", "GOTRUE_MFA_PHONE_OTP_LENGTH": "MFA_PHONE_OTP_LENGTH",
	} {
		set(gotrueKey, envKey)
	}
	for gotrueKey, envKey := range map[string]string{
		"GOTRUE_MAILER_SUBJECTS_CONFIRMATION": "MAILER_SUBJECT_CONFIRMATION", "GOTRUE_MAILER_SUBJECTS_INVITE": "MAILER_SUBJECT_INVITE",
		"GOTRUE_MAILER_SUBJECTS_MAGIC_LINK": "MAILER_SUBJECT_MAGIC_LINK", "GOTRUE_MAILER_SUBJECTS_EMAIL_CHANGE": "MAILER_SUBJECT_EMAIL_CHANGE",
		"GOTRUE_MAILER_SUBJECTS_RECOVERY": "MAILER_SUBJECT_RECOVERY", "GOTRUE_MAILER_SUBJECTS_REAUTHENTICATION": "MAILER_SUBJECT_REAUTHENTICATION",
		"GOTRUE_MAILER_TEMPLATES_CONFIRMATION": "MAILER_TEMPLATE_CONFIRMATION", "GOTRUE_MAILER_TEMPLATES_INVITE": "MAILER_TEMPLATE_INVITE",
		"GOTRUE_MAILER_TEMPLATES_MAGIC_LINK": "MAILER_TEMPLATE_MAGIC_LINK", "GOTRUE_MAILER_TEMPLATES_EMAIL_CHANGE": "MAILER_TEMPLATE_EMAIL_CHANGE",
		"GOTRUE_MAILER_TEMPLATES_RECOVERY": "MAILER_TEMPLATE_RECOVERY", "GOTRUE_MAILER_TEMPLATES_REAUTHENTICATION": "MAILER_TEMPLATE_REAUTHENTICATION",
		"GOTRUE_MAILER_NOTIFICATIONS_PASSWORD_CHANGED_ENABLED": "MAILER_NOTIFICATIONS_PASSWORD_CHANGED_ENABLED", "GOTRUE_MAILER_NOTIFICATIONS_EMAIL_CHANGED_ENABLED": "MAILER_NOTIFICATIONS_EMAIL_CHANGED_ENABLED",
		"GOTRUE_MAILER_NOTIFICATIONS_PHONE_CHANGED_ENABLED": "MAILER_NOTIFICATIONS_PHONE_CHANGED_ENABLED", "GOTRUE_MAILER_NOTIFICATIONS_IDENTITY_LINKED_ENABLED": "MAILER_NOTIFICATIONS_IDENTITY_LINKED_ENABLED",
		"GOTRUE_MAILER_NOTIFICATIONS_IDENTITY_UNLINKED_ENABLED": "MAILER_NOTIFICATIONS_IDENTITY_UNLINKED_ENABLED", "GOTRUE_MAILER_NOTIFICATIONS_MFA_FACTOR_ENROLLED_ENABLED": "MAILER_NOTIFICATIONS_MFA_FACTOR_ENROLLED_ENABLED",
		"GOTRUE_MAILER_NOTIFICATIONS_MFA_FACTOR_UNENROLLED_ENABLED": "MAILER_NOTIFICATIONS_MFA_FACTOR_UNENROLLED_ENABLED",
		"GOTRUE_MAILER_SUBJECTS_PASSWORD_CHANGED_NOTIFICATION":      "MAILER_SUBJECT_PASSWORD_CHANGED_NOTIFICATION", "GOTRUE_MAILER_SUBJECTS_EMAIL_CHANGED_NOTIFICATION": "MAILER_SUBJECT_EMAIL_CHANGED_NOTIFICATION",
		"GOTRUE_MAILER_SUBJECTS_PHONE_CHANGED_NOTIFICATION": "MAILER_SUBJECT_PHONE_CHANGED_NOTIFICATION", "GOTRUE_MAILER_SUBJECTS_IDENTITY_LINKED_NOTIFICATION": "MAILER_SUBJECT_IDENTITY_LINKED_NOTIFICATION",
		"GOTRUE_MAILER_SUBJECTS_IDENTITY_UNLINKED_NOTIFICATION": "MAILER_SUBJECT_IDENTITY_UNLINKED_NOTIFICATION", "GOTRUE_MAILER_SUBJECTS_MFA_FACTOR_ENROLLED_NOTIFICATION": "MAILER_SUBJECT_MFA_FACTOR_ENROLLED_NOTIFICATION",
		"GOTRUE_MAILER_SUBJECTS_MFA_FACTOR_UNENROLLED_NOTIFICATION": "MAILER_SUBJECT_MFA_FACTOR_UNENROLLED_NOTIFICATION",
		"GOTRUE_MAILER_TEMPLATES_PASSWORD_CHANGED_NOTIFICATION":     "MAILER_TEMPLATE_PASSWORD_CHANGED_NOTIFICATION", "GOTRUE_MAILER_TEMPLATES_EMAIL_CHANGED_NOTIFICATION": "MAILER_TEMPLATE_EMAIL_CHANGED_NOTIFICATION",
		"GOTRUE_MAILER_TEMPLATES_PHONE_CHANGED_NOTIFICATION": "MAILER_TEMPLATE_PHONE_CHANGED_NOTIFICATION", "GOTRUE_MAILER_TEMPLATES_IDENTITY_LINKED_NOTIFICATION": "MAILER_TEMPLATE_IDENTITY_LINKED_NOTIFICATION",
		"GOTRUE_MAILER_TEMPLATES_IDENTITY_UNLINKED_NOTIFICATION": "MAILER_TEMPLATE_IDENTITY_UNLINKED_NOTIFICATION", "GOTRUE_MAILER_TEMPLATES_MFA_FACTOR_ENROLLED_NOTIFICATION": "MAILER_TEMPLATE_MFA_FACTOR_ENROLLED_NOTIFICATION",
		"GOTRUE_MAILER_TEMPLATES_MFA_FACTOR_UNENROLLED_NOTIFICATION": "MAILER_TEMPLATE_MFA_FACTOR_UNENROLLED_NOTIFICATION",
	} {
		set(gotrueKey, envKey)
	}
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
		for _, envKey := range provider.Fields {
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
		env["FILE_SIZE_LIMIT"] = "${STORAGE_FILE_SIZE_LIMIT}"
		if input.Configuration.Storage.Backend == contracts.StorageBackendR2 {
			env["TUS_ALLOW_S3_TAGS"] = "false"
		}
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
	reserved := map[string]bool{"POSTGRES_PASSWORD": true, "JWT_SECRET": true, "ANON_KEY": true, "SERVICE_ROLE_KEY": true, "DASHBOARD_USERNAME": true, "DASHBOARD_PASSWORD": true, "SECRET_KEY_BASE": true, "VAULT_ENC_KEY": true, "SMTP_PASS": true, "AWS_SECRET_ACCESS_KEY": true, "PHONE_SECRET": true, "SUPABASE_URL": true, "SUPABASE_PUBLIC_URL": true, "FUNCTIONS_VERIFY_JWT": true}
	return reserved[key]
}
