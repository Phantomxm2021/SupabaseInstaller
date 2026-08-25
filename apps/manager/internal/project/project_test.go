package project

import (
	"strings"
	"testing"
)

func TestNormalizeSlugProducesDNSCompatibleIdentifier(t *testing.T) {
	if got := NormalizeSlug("  My Bee 2!  "); got != "my-bee-2" {
		t.Fatalf("NormalizeSlug() = %q, want my-bee-2", got)
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
