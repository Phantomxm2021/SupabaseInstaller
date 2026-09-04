package render

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	provisionersecrets "supabase-manager/apps/provisioner/internal/secrets"
	"supabase-manager/internal/contracts"

	"gopkg.in/yaml.v3"
)

// Input is the complete desired state plus private values needed to render it.
type Input struct {
	ProjectID           string
	ProjectName         string
	Slug                string
	APIPort             int
	Configuration       contracts.ProjectConfiguration
	Secrets             provisionersecrets.ProjectSecrets
	RuntimeSecrets      map[string]string
	TemplateCompose     []byte
	TemplateEnv         []byte
	TemplateFiles       map[string][]byte
	ProvisionerImageRef string
}

type OutputFiles struct {
	Env                    string
	FunctionsEnv           string
	Compose                string
	MailerTemplates        map[string][]byte
	ComposeProjectName     string
	EnabledComposeServices []string
}

// testTemplateCompose is intentionally unset in production. Unit tests may
// inject a small fixture without reintroducing a compiled runtime template.
var testTemplateCompose []byte
var testTemplateEnv []byte
var testTemplateFiles map[string][]byte

func Project(input Input) (OutputFiles, error) {
	if input.Configuration.Services.Functions && input.ProvisionerImageRef == "" && testTemplateCompose != nil {
		input.ProvisionerImageRef = "supabase-provisioner:test"
		if input.ProjectID == "" {
			input.ProjectID = "test-" + input.Slug
		}
	}
	if input.Configuration.Services.Functions && (strings.TrimSpace(input.ProvisionerImageRef) == "" || strings.Contains(input.ProvisionerImageRef, "${")) {
		return OutputFiles{}, fmt.Errorf("concrete provisioner image reference is required when Functions are enabled")
	}
	if input.Configuration.Services.Functions && strings.TrimSpace(input.ProjectID) == "" {
		return OutputFiles{}, fmt.Errorf("project ID is required when Functions are enabled")
	}
	if input.APIPort == 0 {
		input.APIPort = input.Configuration.Network.APIPort
	}
	if input.Configuration.Network.APIPort != 0 && input.APIPort != 0 && input.Configuration.Network.APIPort != input.APIPort {
		return OutputFiles{}, fmt.Errorf("network.apiPort: does not match requested API port")
	}
	if input.Slug == "" || input.APIPort < 1 || input.APIPort > 65535 {
		return OutputFiles{}, fmt.Errorf("slug and valid API port are required")
	}
	if input.Configuration.General.Domain == "" || input.Configuration.General.SiteURL == "" {
		return OutputFiles{}, fmt.Errorf("domain and site URL are required")
	}
	if input.Configuration.Network.HTTPSMode == contracts.HTTPSModeCaddy {
		return OutputFiles{}, fmt.Errorf("network.httpsMode: legacy Caddy HTTPS requires migration to an external reverse proxy")
	}
	if err := validateAuthConfiguration(input.Configuration.Auth); err != nil {
		return OutputFiles{}, err
	}
	if err := validateStorageConfiguration(input.Configuration.Storage); err != nil {
		return OutputFiles{}, err
	}
	if input.Configuration.Network.InternalGatewayPort != 0 && input.Configuration.Network.InternalGatewayPort != 8000 {
		return OutputFiles{}, fmt.Errorf("network.internalGatewayPort: pinned gateway only supports port 8000")
	}
	if input.Configuration.Database.Extensions != nil && len(input.Configuration.Database.Extensions) > 0 {
		return OutputFiles{}, fmt.Errorf("database.extensions: unsupported by pinned renderer")
	}
	directDB := input.Configuration.Database.DirectPort || input.Configuration.Services.DirectDB
	if directDB && (input.Configuration.Database.DirectPortNumber < 1 || input.Configuration.Database.DirectPortNumber > 65535) {
		if input.Configuration.Network.DirectDatabasePort < 1 || input.Configuration.Network.DirectDatabasePort > 65535 {
			return OutputFiles{}, fmt.Errorf("database.directPortNumber: required when directPort is enabled")
		}
	}
	if !directDB && input.Configuration.Database.DirectPortNumber != 0 {
		return OutputFiles{}, fmt.Errorf("database.directPortNumber: set directPort=true to expose direct database")
	}
	if !directDB && input.Configuration.Network.DirectDatabasePort != 0 {
		return OutputFiles{}, fmt.Errorf("network.directDatabasePort: set database.directPort=true to expose direct database")
	}
	if directDB && input.Configuration.Database.DirectPortNumber != 0 && input.Configuration.Network.DirectDatabasePort != 0 && input.Configuration.Database.DirectPortNumber != input.Configuration.Network.DirectDatabasePort {
		return OutputFiles{}, fmt.Errorf("network.directDatabasePort: must match database.directPortNumber")
	}
	if input.Configuration.Services.Supavisor && input.Configuration.Pooler.SessionPort > 0 && input.Configuration.Network.PoolerPort > 0 && input.Configuration.Pooler.SessionPort != input.Configuration.Network.PoolerPort {
		return OutputFiles{}, fmt.Errorf("network.poolerPort: must match pooler.sessionPort when both are set")
	}
	if input.Configuration.Services.Supavisor && (input.Configuration.Pooler.SessionPort < 1 || input.Configuration.Pooler.SessionPort > 65535 || input.Configuration.Pooler.TransactionPort < 1 || input.Configuration.Pooler.TransactionPort > 65535) {
		return OutputFiles{}, fmt.Errorf("pooler.sessionPort/transactionPort: valid host ports are required when Supavisor is enabled")
	}
	if err := validateGeneratedSecrets(input); err != nil {
		return OutputFiles{}, err
	}
	if input.Configuration.Auth.Phone.Enabled && input.Configuration.Auth.Phone.Provider != "" {
		if _, ok := phoneFieldKeys[input.Configuration.Auth.Phone.Provider]; !ok {
			return OutputFiles{}, fmt.Errorf("auth.phone.provider: unsupported pinned provider %q", input.Configuration.Auth.Phone.Provider)
		}
		for field := range input.Configuration.Auth.Phone.Fields {
			if _, ok := phoneFieldKeys[input.Configuration.Auth.Phone.Provider][field]; !ok {
				return OutputFiles{}, fmt.Errorf("auth.phone.fields.%s: unsupported or secret field", field)
			}
		}
	}
	var compose map[string]any
	if len(input.TemplateCompose) == 0 {
		input.TemplateCompose = testTemplateCompose
		input.TemplateEnv = testTemplateEnv
		if len(testTemplateFiles) > 0 {
			compose, err := LoadOfficialCompose(input.Configuration, testTemplateFiles)
			if err != nil {
				return OutputFiles{}, err
			}
			input.TemplateCompose, err = yaml.Marshal(compose)
			if err != nil {
				return OutputFiles{}, err
			}
		}
	}
	if len(input.TemplateCompose) == 0 {
		return OutputFiles{}, fmt.Errorf("official Supabase Compose template is required")
	}
	if err := yaml.NewDecoder(bytes.NewReader(input.TemplateCompose)).Decode(&compose); err != nil {
		return OutputFiles{}, fmt.Errorf("decode pinned Compose: %w", err)
	}
	services, ok := compose["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return OutputFiles{}, fmt.Errorf("pinned Compose has no services")
	}
	if err := validateServiceConfiguration(input.Configuration); err != nil {
		return OutputFiles{}, err
	}
	if err := validateSelectedServices(input.Configuration, services); err != nil {
		return OutputFiles{}, err
	}
	selected := selectServices(input.Configuration.Services, services)
	if input.Configuration.Network.HTTPSMode == contracts.HTTPSModeCaddy && services["caddy"] != nil {
		selected["caddy"] = true
	}
	filtered := make(map[string]any, len(selected))
	for name := range selected {
		raw, exists := services[name]
		if !exists {
			continue
		}
		service, ok := raw.(map[string]any)
		if !ok {
			return OutputFiles{}, fmt.Errorf("service %s is not a mapping", name)
		}
		image, imageOK := service["image"].(string)
		if !imageOK {
			return OutputFiles{}, fmt.Errorf("service %s has no pinned image", name)
		}
		if err := validatePinnedImage(name, image); err != nil {
			return OutputFiles{}, err
		}
		delete(service, "container_name")
		pruneDependencies(service, selected)
		if isGateway(name) && input.Configuration.Network.HTTPSMode != contracts.HTTPSModeCaddy {
			service["ports"] = []string{fmt.Sprintf("127.0.0.1:%d:8000", input.APIPort)}
		}
		if name == "db" && directDB {
			port := input.Configuration.Database.DirectPortNumber
			if port < 1 || port > 65535 {
				port = input.Configuration.Network.DirectDatabasePort
			}
			if port > 0 && port <= 65535 {
				service["ports"] = []string{fmt.Sprintf("127.0.0.1:%d:5432", port)}
			}
		}
		filtered[name] = service
	}
	for _, required := range requiredServices(input.Configuration.Services) {
		if _, ok := filtered[required]; !ok {
			return OutputFiles{}, fmt.Errorf("pinned Compose is missing selected service %s", required)
		}
	}
	if len(filtered) == 0 {
		return OutputFiles{}, fmt.Errorf("no selected services are available in pinned Compose")
	}
	compose["name"] = "supabase-manager-" + input.Slug
	compose["services"] = filtered
	if auth, ok := filtered["auth"]; ok {
		if err := injectAuthEnvironment(auth, input); err != nil {
			return OutputFiles{}, err
		}
		filtered["auth-templates"] = map[string]any{
			"image": "busybox:1.37.0", "restart": "unless-stopped",
			"command": []string{"httpd", "-f", "-p", "8080", "-h", "/srv/templates"},
			"volumes": []string{"./.manager-runtime/current/templates:/srv/templates:ro"},
		}
	}
	if err := injectServiceConfiguration(filtered, input); err != nil {
		return OutputFiles{}, err
	}
	if input.Configuration.Services.Functions {
		injectFunctionLogCollection(filtered, input)
	}
	encoded, err := yaml.Marshal(compose)
	if err != nil {
		return OutputFiles{}, fmt.Errorf("encode Compose: %w", err)
	}
	env, functionsEnv, err := renderEnvironment(input)
	if err != nil {
		return OutputFiles{}, err
	}
	enabled := make([]string, 0, len(filtered))
	for name := range filtered {
		enabled = append(enabled, name)
	}
	sort.Strings(enabled)
	return OutputFiles{Env: env, FunctionsEnv: functionsEnv, Compose: string(encoded), MailerTemplates: mailerTemplateFiles(input.Configuration.Auth.Mailer), ComposeProjectName: "supabase-manager-" + input.Slug, EnabledComposeServices: enabled}, nil
}

func injectFunctionLogCollection(services map[string]any, input Input) {
	const collector = "function-log-collector"
	services[collector] = map[string]any{
		"image":   input.ProvisionerImageRef,
		"restart": "unless-stopped",
		"environment": map[string]string{
			"PROVISIONER_MODE":            "function-log-collector",
			"FUNCTION_LOG_PROJECT_ID":     input.ProjectID,
			"FUNCTION_LOG_DATABASE_PATH":  "/var/lib/function-logs/function-logs.db",
			"FUNCTION_LOG_FUNCTIONS_ROOT": "/srv/functions",
			"FUNCTION_LOG_PROJECT_ENV":    "/srv/runtime/.env",
			"FUNCTION_LOG_FUNCTIONS_ENV":  "/srv/runtime/.env.functions",
			"FUNCTION_LOG_LISTEN_ADDR":    "0.0.0.0:8081",
		},
		"volumes": []string{
			"./.manager-runtime/function-logs:/var/lib/function-logs",
			"./volumes/functions:/srv/functions:ro",
			"./.manager-runtime/current:/srv/runtime:ro",
		},
		"healthcheck": map[string]any{
			"test":     []string{"CMD", "wget", "-q", "--spider", "http://127.0.0.1:8081/health/live"},
			"interval": "5s", "timeout": "3s", "retries": 20, "start_period": "2s",
		},
	}
	functions := services["functions"].(map[string]any)
	environment, _ := functions["environment"].(map[string]any)
	if environment == nil {
		environment = map[string]any{}
	}
	environment["FUNCTION_LOG_PROJECT_ID"] = input.ProjectID
	functions["environment"] = environment
	command, _ := functions["command"].([]any)
	if command == nil {
		if stringsCommand, ok := functions["command"].([]string); ok {
			command = make([]any, len(stringsCommand))
			for i := range stringsCommand {
				command[i] = stringsCommand[i]
			}
		}
	}
	command = append(command, "--event-worker", "/opt/supabase-manager/event-worker")
	functions["command"] = command
	volumes, _ := functions["volumes"].([]any)
	if volumes == nil {
		if stringVolumes, ok := functions["volumes"].([]string); ok {
			volumes = make([]any, len(stringVolumes))
			for i := range stringVolumes {
				volumes[i] = stringVolumes[i]
			}
		}
	}
	functions["volumes"] = append(volumes, "./.manager-runtime/current/function-logs/event-worker:/opt/supabase-manager/event-worker:ro")
	dependencies, _ := functions["depends_on"].(map[string]any)
	if dependencies == nil {
		dependencies = map[string]any{}
	}
	// Preserve deterministic startup order, but never make user Functions depend
	// on the best-effort collector becoming healthy.
	dependencies[collector] = map[string]any{"condition": "service_started"}
	functions["depends_on"] = dependencies
}

func validateAuthConfiguration(auth contracts.AuthConfig) error {
	if auth.DisableSignup != !auth.Email.AllowSignup {
		return fmt.Errorf("auth.disableSignup/auth.email.allowSignup: disableSignup must equal !allowSignup")
	}
	if auth.DisableSignup && (auth.Phone.Enabled || auth.AnonymousSignIn || hasEnabledOAuth(auth.OAuth)) {
		return fmt.Errorf("auth.disableSignup: cannot disable global signup while phone, anonymous, or OAuth signup is enabled")
	}
	if auth.Email.SecureEmailChange != auth.Email.DoubleConfirmChanges {
		return fmt.Errorf("auth.email.secureEmailChange/auth.email.doubleConfirmChanges: pinned runtime supports one shared capability")
	}
	return nil
}

func validateGeneratedSecrets(input Input) error {
	// A zero-value Input is useful to decode/inspect templates in compatibility
	// callers. Once the persisted base secret set is present, enabled consumers
	// must also carry their generated credentials; never allow image defaults.
	basePresent := input.Secrets.JWTSecret != "" || input.Secrets.DatabasePassword != "" || input.Secrets.SecretKeyBase != ""
	if !basePresent {
		return nil
	}
	checks := []struct {
		enabled bool
		value   string
		field   string
	}{
		{input.Configuration.Services.Realtime, firstNonempty(input.RuntimeSecrets["realtime.dbEncryptionKey"], input.Secrets.RealtimeDBEncryptionKey), "projectSecrets.realtimeDbEncryptionKey"},
		{input.Configuration.Services.Logs, firstNonempty(input.RuntimeSecrets[SecretLogsPublic], input.Secrets.LogflarePublicAccessToken), "projectSecrets.logflarePublicAccessToken"},
		{input.Configuration.Services.Logs, firstNonempty(input.RuntimeSecrets[SecretLogsPrivate], input.Secrets.LogflarePrivateAccessToken), "projectSecrets.logflarePrivateAccessToken"},
		{input.Configuration.Storage.S3CompatibleAPI, firstNonempty(input.RuntimeSecrets[SecretS3Access], input.Secrets.S3ProtocolAccessKeyID), "projectSecrets.s3ProtocolAccessKeyID"},
		{input.Configuration.Storage.S3CompatibleAPI, firstNonempty(input.RuntimeSecrets[SecretS3Secret], input.Secrets.S3ProtocolAccessKeySecret), "projectSecrets.s3ProtocolAccessKeySecret"},
		{input.Configuration.Services.Supavisor, input.Secrets.PoolerTenantID, "projectSecrets.poolerTenantID"},
	}
	for _, check := range checks {
		if check.enabled && strings.TrimSpace(check.value) == "" {
			return fmt.Errorf("%s: generated credential is required", check.field)
		}
	}
	return nil
}

func validateStorageConfiguration(storage contracts.StorageConfig) error {
	switch storage.Backend {
	case "", contracts.StorageBackendLocal, contracts.StorageBackendS3, contracts.StorageBackendAWSS3, contracts.StorageBackendR2:
	default:
		return fmt.Errorf("storage.backend: unsupported backend %q", storage.Backend)
	}
	switch storage.LocalPath {
	case "", "./volumes/storage", "volumes/storage", "/var/lib/storage":
	default:
		return fmt.Errorf("storage.localPath: must refer to managed ./volumes/storage")
	}
	if storage.Backend == contracts.StorageBackendR2 && storage.Endpoint == "" && storage.AccountID == "" {
		return fmt.Errorf("storage.accountId: required for R2 when endpoint is unset")
	}
	return nil
}

func validateSelectedServices(config contracts.ProjectConfiguration, available map[string]any) error {
	required := map[string]bool{"db": config.Services.Database, "auth": config.Services.Auth, "rest": config.Services.REST, "studio": config.Services.Studio, "meta": config.Services.PostgresMeta, "realtime": config.Services.Realtime, "storage": config.Services.Storage, "imgproxy": config.Services.Imgproxy, "functions": config.Services.Functions, "supavisor": config.Services.Supavisor, "analytics": config.Services.Logs, "vector": config.Services.Vector}
	for name, enabled := range required {
		if enabled && available[name] == nil {
			return fmt.Errorf("services.%s: selected service is absent from pinned Compose", name)
		}
	}
	if config.Services.Gateway {
		if available["api-gw"] == nil && available["envoy"] == nil && available["kong"] == nil {
			return fmt.Errorf("services.gateway: selected gateway is absent from pinned Compose")
		}
	}
	if config.Network.HTTPSMode == contracts.HTTPSModeCaddy && available["caddy"] == nil {
		return fmt.Errorf("network.httpsMode: caddy service is absent from pinned Compose")
	}
	return nil
}

func LoadOfficialCompose(config contracts.ProjectConfiguration, files map[string][]byte) (map[string]any, error) {
	read := func(name string) (map[string]any, error) {
		data := files[name]
		if len(data) == 0 {
			return nil, fmt.Errorf("official Compose template is missing overlay %s", name)
		}
		var value map[string]any
		if err := yaml.Unmarshal(data, &value); err != nil {
			return nil, fmt.Errorf("decode pinned Compose overlay %s: %w", name, err)
		}
		return value, nil
	}
	base, err := read("docker-compose.yml")
	if err != nil {
		return nil, err
	}
	if config.Database.Version != "17" {
		return nil, fmt.Errorf("database.version: PostgreSQL 17 is the only supported official runtime")
	}
	// PostgreSQL 17 is the only pinned database runtime. Keep its explicit
	// official overlay so generated Compose stays canonical with the upstream
	// self-hosted template, but never retain a PostgreSQL 15 fallback.
	overlays := []string{"docker-compose.pg17.yml"}
	if config.Network.Gateway == contracts.GatewayKong {
		overlays = append(overlays, "docker-compose.kong.yml")
	} else if config.Network.Gateway != "" && config.Network.Gateway != contracts.GatewayEnvoy {
		return nil, fmt.Errorf("network.gateway: unsupported gateway %q", config.Network.Gateway)
	}
	if config.Services.Logs {
		overlays = append(overlays, "docker-compose.logs.yml")
	}
	if config.Network.HTTPSMode == contracts.HTTPSModeCaddy {
		overlays = append(overlays, "docker-compose.caddy.yml")
	} else if config.Network.HTTPSMode != "" && config.Network.HTTPSMode != contracts.HTTPSModeExternal {
		return nil, fmt.Errorf("network.httpsMode: unsupported HTTPS mode %q", config.Network.HTTPSMode)
	}
	for _, name := range overlays {
		overlay, err := read(name)
		if err != nil {
			return nil, err
		}
		mergeCompose(base, overlay)
	}
	if services, ok := base["services"].(map[string]any); ok {
		if caddy, ok := services["caddy"].(map[string]any); ok {
			if caddy["image"] == "caddy:2" {
				caddy["image"] = "caddy:2.9.1"
			}
		}
	}
	return base, nil
}

func mergeCompose(dst, src map[string]any) {
	for key, value := range src {
		srcMap, srcOK := value.(map[string]any)
		dstMap, dstOK := dst[key].(map[string]any)
		if srcOK && dstOK {
			mergeCompose(dstMap, srcMap)
			continue
		}
		dst[key] = value
	}
}

func validatePinnedImage(service, image string) error {
	if image == "" || strings.Contains(image, "${") {
		return fmt.Errorf("service %s uses unpinned image %s; latest is forbidden", service, image)
	}
	last := image[strings.LastIndex(image, "/")+1:]
	validTag := false
	if at := strings.LastIndex(last, "@sha256:"); at >= 0 {
		digest := last[at+len("@sha256:"):]
		validTag = len(digest) == 64
		for _, r := range digest {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				validTag = false
				break
			}
		}
	} else if colon := strings.LastIndex(last, ":"); colon > 0 {
		tag := last[colon+1:]
		validTag = tag != "" && !strings.HasPrefix(tag, "latest")
	}
	if !validTag {
		return fmt.Errorf("service %s uses unpinned image %s; latest is forbidden", service, image)
	}
	return nil
}

func pruneDependencies(service map[string]any, selected map[string]bool) {
	dependencies, ok := service["depends_on"]
	if !ok {
		return
	}
	switch typed := dependencies.(type) {
	case map[string]any:
		for name := range typed {
			if !selected[name] {
				delete(typed, name)
			}
		}
		if len(typed) == 0 {
			delete(service, "depends_on")
		}
	case []any:
		kept := typed[:0]
		for _, raw := range typed {
			if name, ok := raw.(string); ok && selected[name] {
				kept = append(kept, name)
			}
		}
		if len(kept) == 0 {
			delete(service, "depends_on")
		} else {
			service["depends_on"] = kept
		}
	}
}

func isGateway(name string) bool { return name == "api-gw" || name == "envoy" || name == "kong" }

func requiredServices(s contracts.Services) []string {
	var required []string
	if s.Database {
		required = append(required, "db")
	}
	if s.Auth {
		required = append(required, "auth")
	}
	if s.REST {
		required = append(required, "rest")
	}
	if s.Studio {
		required = append(required, "studio")
	}
	if s.PostgresMeta {
		required = append(required, "meta")
	}
	return required
}
