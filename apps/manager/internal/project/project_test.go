package project

import (
	"strings"
	"testing"

	"supabase-manager/internal/contracts"
)

func TestNormalizeSlugProducesDNSCompatibleIdentifier(t *testing.T) {
	if got := NormalizeSlug("  My Bee 2!  "); got != "my-bee-2" {
		t.Fatalf("NormalizeSlug() = %q, want my-bee-2", got)
	}
}

func TestConfigurationPresetsApplyDependencyClosure(t *testing.T) {
	cases := []struct {
		preset contracts.Preset
		check  func(contracts.ProjectConfiguration) bool
	}{
		{contracts.PresetLightweight, func(c contracts.ProjectConfiguration) bool {
			return !c.Services.Storage && !c.Services.Logs && !c.Services.Vector
		}},
		{contracts.PresetStandard, func(c contracts.ProjectConfiguration) bool {
			return c.Services.Realtime && c.Services.Storage && c.Services.Functions && c.Services.Supavisor && c.Pooler.PoolSize > 0 && c.Pooler.MaxClientConnections > 0
		}},
		{contracts.PresetFull, func(c contracts.ProjectConfiguration) bool {
			return c.Services.Imgproxy && c.Services.Storage && c.Services.Logs && c.Services.Vector && c.Pooler.PoolSize > 0 && c.Pooler.MaxClientConnections > 0
		}},
	}
	for _, tc := range cases {
		got := ApplyConfigurationPreset(tc.preset)
		if !tc.check(got) {
			t.Fatalf("ApplyConfigurationPreset(%s) = %#v", tc.preset, got)
		}
	}
}

func TestNormalizeDraftConfigurationTakesPrecedenceOverLegacyProjection(t *testing.T) {
	draft := validDraft()
	draft.Domain = "legacy.example.com"
	draft.SiteURL = "https://legacy.example.com"
	draft.SupabaseVersion = "self-hosted/v0.8.0"
	draft.Services = contracts.Services{Database: true}
	draft.Configuration = DefaultConfiguration(contracts.PresetLightweight)
	draft.Configuration.General = contracts.GeneralConfig{Domain: "typed.example.com", SiteURL: "https://typed.example.com", SupabaseVersion: "self-hosted/v0.8.0"}
	draft.Configuration.Services = ApplyPreset(PresetLightweight)
	draft.Configuration.Services.Storage = true

	normalized := NormalizeDraft(draft)
	if normalized.Domain != "typed.example.com" || normalized.SiteURL != "https://typed.example.com" || normalized.Services.Storage != true {
		t.Fatalf("NormalizeDraft() did not project typed configuration: %#v", normalized)
	}
}

func TestNormalizeSlugCollapsesSeparators(t *testing.T) {
	if got := NormalizeSlug("Bee___API---Prod"); got != "bee-api-prod" {
		t.Fatalf("NormalizeSlug() = %q, want bee-api-prod", got)
	}
}

func TestLightweightPresetMatchesPRD(t *testing.T) {
	got := ApplyPreset(PresetLightweight)
	if !got.Database || !got.Gateway || !got.Auth || !got.REST || !got.Studio || !got.PostgresMeta {
		t.Fatalf("Lightweight core services = %#v, want all enabled", got)
	}
	if got.Realtime || got.Storage || got.Imgproxy || got.Functions || got.Supavisor || got.Logs || got.Vector || got.DirectDB {
		t.Fatalf("Lightweight optional services = %#v, want all disabled", got)
	}
}

func TestValidateDraftRejectsStudioWithoutPostgresMeta(t *testing.T) {
	draft := validDraft()
	draft.Services.Studio = true
	draft.Services.PostgresMeta = false

	err := ValidateDraft(draft)
	if err == nil || !strings.Contains(err.Error(), "postgres-meta") {
		t.Fatalf("ValidateDraft() error = %v, want postgres-meta dependency", err)
	}
}

func TestValidateDraftRejectsRelativeSiteURL(t *testing.T) {
	draft := validDraft()
	draft.SiteURL = "localhost:3000"

	err := ValidateDraft(draft)
	if err == nil || !strings.Contains(err.Error(), "siteUrl") {
		t.Fatalf("ValidateDraft() error = %v, want siteUrl validation", err)
	}
}

func TestValidateDraftRejectsLatestRuntimeVersion(t *testing.T) {
	draft := validDraft()
	draft.SupabaseVersion = "latest"

	err := ValidateDraft(draft)
	if err == nil || !strings.Contains(err.Error(), "supabaseVersion") {
		t.Fatalf("ValidateDraft() error = %v, want pinned version validation", err)
	}
}

func validDraft() Draft {
	return Draft{
		Name:            "Bee",
		Slug:            "bee",
		Domain:          "bee.example.com",
		SiteURL:         "https://example.com",
		SupabaseVersion: "self-hosted/v0.8.0",
		Preset:          PresetLightweight,
		Services:        ApplyPreset(PresetLightweight),
	}
}
