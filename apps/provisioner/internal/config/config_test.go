package config

import (
	"strings"
	"testing"
)

func TestLoadDefaultsToPrivateAddressAndProjectRoot(t *testing.T) {
	t.Setenv("MANAGER_TOKEN", strings.Repeat("a", 32))
	t.Setenv("PROVISIONER_LISTEN_ADDR", "")
	t.Setenv("PROJECT_ROOT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ListenAddr != "0.0.0.0:9090" {
		t.Fatalf("ListenAddr = %q, want 0.0.0.0:9090", cfg.ListenAddr)
	}
	if cfg.ProjectRoot != "/opt/supabase-manager/projects" {
		t.Fatalf("ProjectRoot = %q, want default project root", cfg.ProjectRoot)
	}
}

func TestLoadRejectsShortManagerToken(t *testing.T) {
	t.Setenv("MANAGER_TOKEN", "short")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "MANAGER_TOKEN") {
		t.Fatalf("Load() error = %v, want MANAGER_TOKEN validation", err)
	}
}

func TestLoadRejectsPublishedExampleToken(t *testing.T) {
	t.Setenv("MANAGER_TOKEN", "replace-with-output-of-openssl-rand-hex-32")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted example token")
	}
}

func TestLoadRejectsPublishedZeroToken(t *testing.T) {
	t.Setenv("MANAGER_TOKEN", strings.Repeat("0", 32))
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted published zero token")
	}
}
