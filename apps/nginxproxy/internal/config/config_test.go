package config

import "testing"

func TestLoadRequiresSecretAndTLSPaths(t *testing.T) {
	_, err := Load(func(string) string { return "" })
	if err == nil {
		t.Fatal("Load() error = nil, want required-variable error")
	}
}

func TestLoadUsesConfiguredValuesAndSafeDefaults(t *testing.T) {
	values := map[string]string{
		"NGINX_PROXY_TOKEN":          "secret",
		"NGINX_CERTIFICATE_FILE":     "/etc/nginx/ssl/origin.pem",
		"NGINX_CERTIFICATE_KEY_FILE": "/etc/nginx/ssl/origin.key",
		"NGINX_PROXY_SOCKET":         "/run/supabase-manager/agent.sock",
		"NGINX_SITES_AVAILABLE":      "/etc/nginx/sites-available",
		"NGINX_SITES_ENABLED":        "/etc/nginx/sites-enabled",
		"NGINX_BINARY":               "/usr/sbin/nginx",
		"SYSTEMCTL_BINARY":           "/bin/systemctl",
	}
	config, err := Load(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := config.SocketPath, "/run/supabase-manager/agent.sock"; got != want {
		t.Fatalf("SocketPath = %q, want %q", got, want)
	}
	if got, want := config.NginxBinary, "/usr/sbin/nginx"; got != want {
		t.Fatalf("NginxBinary = %q, want %q", got, want)
	}
}
