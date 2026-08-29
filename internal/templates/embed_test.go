package templates

import (
	"strings"
	"testing"
)

func TestEmbeddedStudioDefaultUsesServerTerminology(t *testing.T) {
	if !strings.Contains(string(EnvExample()), "STUDIO_DEFAULT_PROJECT=Default Server") {
		t.Fatal("embedded Studio default must use Server terminology")
	}
	configuration, err := Files().ReadFile("self-hosted-v0.8.0/CONFIG.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configuration), "Name shown for the single default server on the dashboard.") || !strings.Contains(string(configuration), "Default: `Default Server`.") {
		t.Fatal("Studio configuration reference must use Server terminology")
	}
}

func TestEmbeddedDocumentationAndScriptsUseServerTerminology(t *testing.T) {
	cases := map[string][]string{
		"self-hosted-v0.8.0/README.md": {
			"A dashboard for managing your self-hosted Supabase server",
		},
		"self-hosted-v0.8.0/CONFIG.md": {
			"Server API Settings page",
			"Renames the default server",
			"hosted (multi-server) mode",
			"self-hosted single-server mode",
			"External/public URL of the Supabase server.",
		},
		"self-hosted-v0.8.0/run.sh": {
			"is not a service in this server",
		},
		"self-hosted-v0.8.0/setup.sh": {
			"Already in a Supabase server directory; skipping bootstrap.",
			"Creating server at $target",
			"Setup complete. Server ready at: $(pwd)",
		},
	}
	for path, expected := range cases {
		data, err := Files().ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, phrase := range expected {
			if !strings.Contains(string(data), phrase) {
				t.Errorf("%s is missing server terminology %q", path, phrase)
			}
		}
	}
}
