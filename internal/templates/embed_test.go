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
