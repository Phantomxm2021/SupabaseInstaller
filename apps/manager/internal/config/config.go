package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const minimumTokenLength = 32

type Config struct {
	ListenAddr          string
	DatabasePath        string
	MasterEncryptionKey []byte
	ProvisionerURL      string
	ProvisionerToken    string
	WebDistPath         string
	PublicOrigin        string
	SecureCookies       bool
	PortRangeStart      int
	PortRangeEnd        int
}

func Load() (Config, error) {
	keyText := os.Getenv("MASTER_ENCRYPTION_KEY")
	if keyText == "" || isExampleSecret(keyText) {
		return Config{}, errors.New("MASTER_ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(key) != 32 {
		return Config{}, fmt.Errorf("MASTER_ENCRYPTION_KEY must be base64-encoded 32 bytes")
	}
	token := os.Getenv("PROVISIONER_TOKEN")
	if len(token) < minimumTokenLength || isExampleSecret(token) {
		return Config{}, fmt.Errorf("PROVISIONER_TOKEN must be at least %d bytes", minimumTokenLength)
	}

	portStart, err := envInt("PORT_RANGE_START", 8000)
	if err != nil {
		return Config{}, err
	}
	portEnd, err := envInt("PORT_RANGE_END", 8999)
	if err != nil || portEnd < portStart {
		return Config{}, fmt.Errorf("PORT_RANGE_END must be an integer greater than or equal to PORT_RANGE_START")
	}
	secureCookies, err := strconv.ParseBool(envOr("SECURE_COOKIES", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("SECURE_COOKIES must be true or false")
	}

	return Config{
		ListenAddr:          envOr("MANAGER_LISTEN_ADDR", "0.0.0.0:8080"),
		DatabasePath:        envOr("MANAGER_DATABASE_PATH", "/var/lib/supabase-manager/manager.db"),
		MasterEncryptionKey: key,
		ProvisionerURL:      envOr("PROVISIONER_URL", "http://provisioner:9090"),
		ProvisionerToken:    token,
		WebDistPath:         envOr("WEB_DIST_PATH", ""),
		PublicOrigin:        envOr("PUBLIC_ORIGIN", "http://localhost:8080"),
		SecureCookies:       secureCookies,
		PortRangeStart:      portStart,
		PortRangeEnd:        portEnd,
	}, nil
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

func envInt(name string, fallback int) (int, error) {
	text := os.Getenv(name)
	if text == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < 1 || value > 65535 {
		return 0, fmt.Errorf("%s must be a valid TCP port", name)
	}
	return value, nil
}
