package render

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	provisionersecrets "supabase-manager/apps/provisioner/internal/secrets"
	"supabase-manager/internal/templates"

	"gopkg.in/yaml.v3"
)

type Input struct {
	ProjectID       string
	Slug            string
	Domain          string
	SiteURL         string
	APIPort         int
	TemplateCompose []byte
	Secrets         provisionersecrets.ProjectSecrets
}

type OutputFiles struct {
	Env                string
	Compose            string
	ComposeProjectName string
}

func Lightweight(input Input) (OutputFiles, error) {
	if input.Slug == "" || input.Domain == "" || input.SiteURL == "" || input.APIPort < 1 || input.APIPort > 65535 {
		return OutputFiles{}, fmt.Errorf("slug, domain, site URL, and valid API port are required")
	}
	if len(input.TemplateCompose) == 0 {
		input.TemplateCompose = templates.DockerCompose()
	}
	var compose map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(input.TemplateCompose))
	if err := decoder.Decode(&compose); err != nil {
		return OutputFiles{}, fmt.Errorf("decode pinned Compose: %w", err)
	}
	services, ok := compose["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return OutputFiles{}, fmt.Errorf("pinned Compose has no services")
	}
	selected := map[string]bool{"db": true, "auth": true, "rest": true, "meta": true, "studio": true, "api-gw": true, "envoy": true, "kong": true}
	filtered := make(map[string]any)
	for name, raw := range services {
		if !selected[name] {
			continue
		}
		service, ok := raw.(map[string]any)
		if !ok {
			return OutputFiles{}, fmt.Errorf("service %s is not a mapping", name)
		}
		if image, ok := service["image"].(string); ok {
			if err := validatePinnedImage(name, image); err != nil {
				return OutputFiles{}, err
			}
		}
		pruneDependencies(service, selected)
		if name == "api-gw" || name == "envoy" || (name == "kong" && services["api-gw"] == nil && services["envoy"] == nil) {
			service["ports"] = []string{fmt.Sprintf("127.0.0.1:%d:8000", input.APIPort)}
		}
		filtered[name] = service
	}
	for _, required := range []string{"db", "auth", "rest", "meta", "studio"} {
		if _, ok := filtered[required]; !ok {
			return OutputFiles{}, fmt.Errorf("pinned Compose is missing required service %s", required)
		}
	}
	if _, apiGateway := filtered["api-gw"]; !apiGateway {
		if _, envoy := filtered["envoy"]; !envoy {
			if _, kong := filtered["kong"]; !kong {
				return OutputFiles{}, fmt.Errorf("pinned Compose is missing Envoy or Kong gateway")
			}
		}
	}
	compose["name"] = "supabase-manager-" + input.Slug
	compose["services"] = filtered
	encoded, err := yaml.Marshal(compose)
	if err != nil {
		return OutputFiles{}, fmt.Errorf("encode Lightweight Compose: %w", err)
	}
	return OutputFiles{
		Env: renderEnv(input), Compose: string(encoded), ComposeProjectName: "supabase-manager-" + input.Slug,
	}, nil
}

func validatePinnedImage(service, image string) error {
	if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") {
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

func renderEnv(input Input) string {
	values := map[string]string{
		"ANON_KEY":            input.Secrets.AnonKey,
		"API_EXTERNAL_URL":    "https://" + input.Domain + "/auth/v1",
		"DASHBOARD_PASSWORD":  input.Secrets.DashboardPassword,
		"JWT_SECRET":          input.Secrets.JWTSecret,
		"POSTGRES_PASSWORD":   input.Secrets.DatabasePassword,
		"SECRET_KEY_BASE":     input.Secrets.SecretKeyBase,
		"SERVICE_ROLE_KEY":    input.Secrets.ServiceRoleKey,
		"SITE_URL":            input.SiteURL,
		"SUPABASE_PUBLIC_URL": "https://" + input.Domain,
		"VAULT_ENC_KEY":       input.Secrets.VaultEncryptionKey,
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var output strings.Builder
	for _, key := range keys {
		output.WriteString(key)
		output.WriteByte('=')
		output.WriteString(values[key])
		output.WriteByte('\n')
	}
	return output.String()
}
