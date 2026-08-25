package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

const minimumTokenLength = 32

type Config struct {
	ListenAddr          string
	DatabasePath        string
	MasterEncryptionKey []byte
	ProvisionerURL      string
	ProvisionerToken    string
	WebDistPath         string
}

func Load() (Config, error) {
	keyText := os.Getenv("MASTER_ENCRYPTION_KEY")
	if keyText == "" {
		return Config{}, errors.New("MASTER_ENCRYPTION_KEY is required")
	}
	key, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(key) != 32 {
		return Config{}, fmt.Errorf("MASTER_ENCRYPTION_KEY must be base64-encoded 32 bytes")
	}
	token := os.Getenv("PROVISIONER_TOKEN")
	if len(token) < minimumTokenLength {
		return Config{}, fmt.Errorf("PROVISIONER_TOKEN must be at least %d bytes", minimumTokenLength)
	}

	return Config{
		ListenAddr:          envOr("MANAGER_LISTEN_ADDR", "0.0.0.0:8080"),
		DatabasePath:        envOr("MANAGER_DATABASE_PATH", "/var/lib/supabase-manager/manager.db"),
		MasterEncryptionKey: key,
		ProvisionerURL:      envOr("PROVISIONER_URL", "http://provisioner:9090"),
		ProvisionerToken:    token,
		WebDistPath:         envOr("WEB_DIST_PATH", ""),
	}, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
