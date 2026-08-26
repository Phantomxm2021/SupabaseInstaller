package render

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	provisionersecrets "supabase-manager/apps/provisioner/internal/secrets"
	"supabase-manager/internal/contracts"
	"supabase-manager/internal/templates"

	"gopkg.in/yaml.v3"
)

// Input is the complete, redacted desired state plus private values needed to
// render it. Domain and SiteURL remain for old prepare callers.
type Input struct {
	ProjectID       string
	Slug            string
	Domain          string
	SiteURL         string
	APIPort         int
	Configuration   contracts.ProjectConfiguration
	Secrets         provisionersecrets.ProjectSecrets
	RuntimeSecrets  map[string]string
	TemplateCompose []byte
}

type OutputFiles struct {
	Env                    string
	FunctionsEnv           string
	Compose                string
	ComposeProjectName     string
	EnabledComposeServices []string
}

// Lightweight is retained as a compatibility shim. Project is the sole
// authoritative renderer used by new reconciliation code.
func Lightweight(input Input) (OutputFiles, error) {
	if input.Configuration.General.Domain == "" {
		input.Configuration.General.Domain = input.Domain
	}
	if input.Configuration.General.SiteURL == "" {
		input.Configuration.General.SiteURL = input.SiteURL
	}
	if input.Configuration.Services == (contracts.Services{}) {
		input.Configuration.Services = contracts.Services{Database: true, Gateway: true, Auth: true, REST: true, Studio: true, PostgresMeta: true}
	}
	return Project(input)
}

func Project(input Input) (OutputFiles, error) {
	if input.Slug == "" || input.APIPort < 1 || input.APIPort > 65535 {
		return OutputFiles{}, fmt.Errorf("slug and valid API port are required")
	}
	if input.Configuration.General.Domain == "" {
		input.Configuration.General.Domain = input.Domain
	}
	if input.Configuration.General.SiteURL == "" {
		input.Configuration.General.SiteURL = input.SiteURL
	}
	if input.Configuration.General.Domain == "" || input.Configuration.General.SiteURL == "" {
		return OutputFiles{}, fmt.Errorf("domain and site URL are required")
	}
	if len(input.TemplateCompose) == 0 {
		input.TemplateCompose = templates.DockerCompose()
	}
	var compose map[string]any
	if err := yaml.NewDecoder(bytes.NewReader(input.TemplateCompose)).Decode(&compose); err != nil {
		return OutputFiles{}, fmt.Errorf("decode pinned Compose: %w", err)
	}
	services, ok := compose["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return OutputFiles{}, fmt.Errorf("pinned Compose has no services")
	}
	selected := selectServices(input.Configuration.Services, services)
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
		if isGateway(name) {
			service["ports"] = []string{fmt.Sprintf("127.0.0.1:%d:8000", input.APIPort)}
		}
		if name == "db" && input.Configuration.Database.DirectPort {
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
	return OutputFiles{Env: env, FunctionsEnv: functionsEnv, Compose: string(encoded), ComposeProjectName: "supabase-manager-" + input.Slug, EnabledComposeServices: enabled}, nil
}

func validatePinnedImage(service, image string) error {
	if strings.HasSuffix(image, ":latest") || !strings.Contains(image, ":") || strings.Contains(image, "${") {
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
