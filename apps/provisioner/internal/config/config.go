package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	ListenAddr                  string
	ProjectRoot                 string
	DockerHost                  string
	ManagerToken                string
	NginxProxyMode              string
	NginxProxySocket            string
	NginxProxyToken             string
	AcceptanceInspectorFailOnce bool
}

func Load() (Config, error) {
	token := os.Getenv("MANAGER_TOKEN")
	if len(token) < 32 || isExampleSecret(token) {
		return Config{}, fmt.Errorf("MANAGER_TOKEN must be at least 32 bytes")
	}
	nginxProxyMode := envOr("NGINX_PROXY_MODE", "disabled")
	if nginxProxyMode != "disabled" && nginxProxyMode != "managed" {
		return Config{}, fmt.Errorf("NGINX_PROXY_MODE must be disabled or managed")
	}
	config := Config{
		ListenAddr:                  envOr("PROVISIONER_LISTEN_ADDR", "0.0.0.0:9090"),
		ProjectRoot:                 envOr("PROJECT_ROOT", "/opt/supabase-manager/projects"),
		DockerHost:                  envOr("DOCKER_HOST", "unix:///var/run/docker.sock"),
		ManagerToken:                token,
		NginxProxyMode:              nginxProxyMode,
		NginxProxySocket:            os.Getenv("NGINX_PROXY_SOCKET"),
		NginxProxyToken:             os.Getenv("NGINX_PROXY_TOKEN"),
		AcceptanceInspectorFailOnce: os.Getenv("ACCEPTANCE_INSPECTOR_FAIL_ONCE") == "1",
	}
	if config.NginxProxyMode == "managed" && (config.NginxProxySocket == "" || config.NginxProxyToken == "") {
		return Config{}, fmt.Errorf("NGINX_PROXY_SOCKET and NGINX_PROXY_TOKEN are required when NGINX_PROXY_MODE=managed")
	}
	return config, nil
}

func isExampleSecret(value string) bool {
	for _, marker := range []string{"replace-with", "change-me", "example", "your-"} {
		if strings.Contains(strings.ToLower(value), marker) {
			return true
		}
	}
	return value == strings.Repeat("0", len(value)) || value == strings.Repeat("A", len(value))
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
