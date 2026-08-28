// Package config loads the native Nginx proxy agent's host-owned settings.
package config

import (
	"fmt"
	"strings"
)

type Config struct {
	SocketPath         string
	Token              string
	SitesAvailable     string
	SitesEnabled       string
	AuthDirectory      string
	CertificateFile    string
	CertificateKeyFile string
	NginxBinary        string
	SystemctlBinary    string
}

func Load(lookup func(string) string) (Config, error) {
	config := Config{
		SocketPath:         valueOr(lookup, "NGINX_PROXY_SOCKET", "/run/supabase-manager/nginx-proxy-agent.sock"),
		Token:              strings.TrimSpace(lookup("NGINX_PROXY_TOKEN")),
		SitesAvailable:     valueOr(lookup, "NGINX_SITES_AVAILABLE", "/etc/nginx/sites-available"),
		SitesEnabled:       valueOr(lookup, "NGINX_SITES_ENABLED", "/etc/nginx/sites-enabled"),
		AuthDirectory:      valueOr(lookup, "NGINX_AUTH_DIRECTORY", "/etc/supabase-manager/nginx-auth"),
		CertificateFile:    strings.TrimSpace(lookup("NGINX_CERTIFICATE_FILE")),
		CertificateKeyFile: strings.TrimSpace(lookup("NGINX_CERTIFICATE_KEY_FILE")),
		NginxBinary:        valueOr(lookup, "NGINX_BINARY", "/usr/sbin/nginx"),
		SystemctlBinary:    valueOr(lookup, "SYSTEMCTL_BINARY", "/bin/systemctl"),
	}
	for label, path := range map[string]string{
		"NGINX_PROXY_SOCKET":         config.SocketPath,
		"NGINX_SITES_AVAILABLE":      config.SitesAvailable,
		"NGINX_SITES_ENABLED":        config.SitesEnabled,
		"NGINX_AUTH_DIRECTORY":       config.AuthDirectory,
		"NGINX_CERTIFICATE_FILE":     config.CertificateFile,
		"NGINX_CERTIFICATE_KEY_FILE": config.CertificateKeyFile,
		"NGINX_BINARY":               config.NginxBinary,
		"SYSTEMCTL_BINARY":           config.SystemctlBinary,
	} {
		if !safeAbsolutePath(path) {
			return Config{}, fmt.Errorf("%s must be a safe absolute path", label)
		}
	}
	if config.Token == "" {
		return Config{}, fmt.Errorf("NGINX_PROXY_TOKEN is required")
	}
	return config, nil
}

func valueOr(lookup func(string) string, key, fallback string) string {
	if value := strings.TrimSpace(lookup(key)); value != "" {
		return value
	}
	return fallback
}

func safeAbsolutePath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.ContainsAny(path, "\r\n;")
}
