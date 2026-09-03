package templates

import "testing"

func TestLatestOfficialManifestUsesPinnedComponentImages(t *testing.T) {
	manifest := LatestOfficial()
	if manifest.ID != "official-2026-08-03" {
		t.Fatalf("manifest ID = %q", manifest.ID)
	}
	if manifest.Label != "Latest official" || manifest.Channel != ChannelOfficial {
		t.Fatalf("manifest = %+v", manifest)
	}
	if manifest.Images["studio"] != "supabase/studio:2026.08.03-sha-022b374" {
		t.Fatalf("studio image = %q", manifest.Images["studio"])
	}
	if manifest.Images["auth"] != "supabase/gotrue:v2.189.0" {
		t.Fatalf("auth image = %q", manifest.Images["auth"])
	}
	if manifest.Images["db"] != "supabase/postgres:17.6.1.136" {
		t.Fatalf("db image = %q", manifest.Images["db"])
	}
}

func TestResolveLegacyTemplateVersionMapsToBundledManifest(t *testing.T) {
	manifest, ok := ResolveLegacy("self-hosted/v0.8.0")
	if !ok {
		t.Fatal("expected legacy template version to resolve")
	}
	if manifest.ID != LatestOfficial().ID {
		t.Fatalf("resolved manifest = %q", manifest.ID)
	}
}

func TestManifestLookupRejectsMutableAndUnknownIdentifiers(t *testing.T) {
	for _, id := range []string{"", "latest", "master", "supabase/studio:latest", "official-2099-01-01"} {
		if _, ok := Lookup(id); ok {
			t.Fatalf("unexpected lookup success for %q", id)
		}
	}
}
