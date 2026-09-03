package config

import (
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("PROVISIONER_IMAGE_REF", "supabase-provisioner:test")
	os.Exit(m.Run())
}

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
	if cfg.ProvisionerImageRef != "supabase-provisioner:test" {
		t.Fatalf("ProvisionerImageRef = %q", cfg.ProvisionerImageRef)
	}
}

func TestLoadRequiresConcreteProvisionerImageReference(t *testing.T) {
	t.Setenv("MANAGER_TOKEN", strings.Repeat("a", 32))
	t.Setenv("PROVISIONER_IMAGE_REF", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PROVISIONER_IMAGE_REF") {
		t.Fatalf("Load() error = %v, want image reference validation", err)
	}
	t.Setenv("PROVISIONER_IMAGE_REF", "supabase-provisioner:${MANAGER_IMAGE_TAG}")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PROVISIONER_IMAGE_REF") {
		t.Fatalf("Load() error = %v, want interpolation rejection", err)
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

func TestLoadEnablesInspectorFailpointOnlyWhenExplicitlyRequested(t *testing.T) {
	t.Setenv("MANAGER_TOKEN", strings.Repeat("a", 32))
	t.Setenv("ACCEPTANCE_INSPECTOR_FAIL_ONCE", "1")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AcceptanceInspectorFailOnce {
		t.Fatal("explicit acceptance inspector failpoint was not loaded")
	}
}

func TestLoadManagedNginxProxyRequiresSocketAndToken(t *testing.T) {
	t.Setenv("MANAGER_TOKEN", strings.Repeat("a", 32))
	t.Setenv("NGINX_PROXY_MODE", "managed")
	t.Setenv("NGINX_PROXY_SOCKET", "")
	t.Setenv("NGINX_PROXY_TOKEN", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "NGINX_PROXY") {
		t.Fatalf("Load() error = %v, want managed proxy validation", err)
	}

	t.Setenv("NGINX_PROXY_SOCKET", "/run/supabase-manager/nginx-proxy-agent.sock")
	t.Setenv("NGINX_PROXY_TOKEN", "separate-agent-token")
	config, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := config.NginxProxyMode, "managed"; got != want {
		t.Fatalf("NginxProxyMode = %q, want %q", got, want)
	}
}

func TestLoadRejectsUnknownNginxProxyMode(t *testing.T) {
	t.Setenv("MANAGER_TOKEN", strings.Repeat("a", 32))
	t.Setenv("NGINX_PROXY_MODE", "sometimes")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "NGINX_PROXY_MODE") {
		t.Fatalf("Load() error = %v, want mode validation", err)
	}
}
