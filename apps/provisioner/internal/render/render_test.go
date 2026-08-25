package render

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const testCompose = `
name: supabase
services:
  db:
    image: supabase/postgres:17.6.1.136
  auth:
    image: supabase/gotrue:v2.177.0
    depends_on:
      db:
        condition: service_healthy
  rest:
    image: postgrest/postgrest:v13.0.4
  meta:
    image: supabase/postgres-meta:v0.91.0
  studio:
    image: supabase/studio:2026.04.27-sha-5f60601
    depends_on:
      meta:
        condition: service_healthy
      analytics:
        condition: service_healthy
  envoy:
    image: envoyproxy/envoy:v1.35.3
  realtime:
    image: supabase/realtime:v2.44.0
  storage:
    image: supabase/storage-api:v1.25.7
  analytics:
    image: supabase/logflare:1.22.4
`

func TestLightweightRenderUsesPinnedImagesAndUniqueComposeName(t *testing.T) {
	output, err := Lightweight(Input{
		ProjectID: "project-1", Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com",
		APIPort: 18001, TemplateCompose: []byte(testCompose),
	})
	if err != nil {
		t.Fatalf("Lightweight() error = %v", err)
	}
	if !strings.Contains(output.Env, "SUPABASE_PUBLIC_URL=https://bee.example.com") {
		t.Fatalf(".env missing public URL: %s", output.Env)
	}
	if strings.Contains(output.Compose, ":latest") {
		t.Fatal("Compose contains an unpinned latest image")
	}
	for _, disabled := range []string{"realtime:", "storage:", "analytics:"} {
		if strings.Contains(output.Compose, disabled) {
			t.Fatalf("Lightweight Compose contains disabled service %s", disabled)
		}
	}
	if strings.Contains(output.Compose, "analytics") {
		t.Fatal("Lightweight Compose retained dependency on disabled analytics")
	}
	if output.ComposeProjectName != "supabase-manager-bee" {
		t.Fatalf("ComposeProjectName = %q", output.ComposeProjectName)
	}
}

func TestLightweightRejectsLatestImage(t *testing.T) {
	compose := strings.Replace(testCompose, "supabase/gotrue:v2.177.0", "supabase/gotrue:latest", 1)
	_, err := Lightweight(Input{Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", APIPort: 18001, TemplateCompose: []byte(compose)})
	if err == nil || !strings.Contains(err.Error(), "latest") {
		t.Fatalf("Lightweight() error = %v, want latest rejection", err)
	}
}

func TestEmbeddedOfficialTemplateRendersOnlyLightweightServices(t *testing.T) {
	output, err := Lightweight(Input{Slug: "bee", Domain: "bee.example.com", SiteURL: "https://example.com", APIPort: 18001})
	if err != nil {
		t.Fatalf("Lightweight() with embedded template error = %v", err)
	}
	var document struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(output.Compose), &document); err != nil {
		t.Fatalf("decode rendered Compose: %v", err)
	}
	for _, required := range []string{"api-gw", "auth", "db", "meta", "rest", "studio"} {
		if _, ok := document.Services[required]; !ok {
			t.Fatalf("embedded Lightweight Compose missing %s", required)
		}
	}
	for _, disabled := range []string{"realtime", "storage", "imgproxy", "functions", "supavisor"} {
		if _, ok := document.Services[disabled]; ok {
			t.Fatalf("embedded Lightweight Compose contains %s", disabled)
		}
	}
}
