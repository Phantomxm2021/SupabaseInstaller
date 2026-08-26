package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	ListenAddr   string
	ProjectRoot  string
	DockerHost   string
	ManagerToken string
}

func Load() (Config, error) {
	token := os.Getenv("MANAGER_TOKEN")
	if len(token) < 32 || isExampleSecret(token) {
		return Config{}, fmt.Errorf("MANAGER_TOKEN must be at least 32 bytes")
	}
	return Config{
		ListenAddr:   envOr("PROVISIONER_LISTEN_ADDR", "0.0.0.0:9090"),
		ProjectRoot:  envOr("PROJECT_ROOT", "/opt/supabase-manager/projects"),
		DockerHost:   envOr("DOCKER_HOST", "unix:///var/run/docker.sock"),
		ManagerToken: token,
	}, nil
}

func isExampleSecret(value string) bool {
	for _, marker := range []string{"replace-with", "change-me", "example", "your-"} {
		if strings.Contains(strings.ToLower(value), marker) {
			return true
		}
	}
	return value == strings.Repeat("0", len(value))
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
