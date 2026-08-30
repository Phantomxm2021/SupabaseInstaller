package config

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadRejectsMissingManagerSecrets(t *testing.T) {
	t.Setenv("MASTER_ENCRYPTION_KEY", "")
	t.Setenv("PROVISIONER_TOKEN", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "MASTER_ENCRYPTION_KEY") {
		t.Fatalf("Load() error = %v, want MASTER_ENCRYPTION_KEY validation", err)
	}
}

func TestLoadReturnsValidatedManagerConfiguration(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	t.Setenv("MASTER_ENCRYPTION_KEY", key)
	t.Setenv("PROVISIONER_TOKEN", strings.Repeat("a", 32))
	t.Setenv("MANAGER_LISTEN_ADDR", "127.0.0.1:8181")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:8181" {
		t.Fatalf("ListenAddr = %q, want 127.0.0.1:8181", cfg.ListenAddr)
	}
	if len(cfg.MasterEncryptionKey) != 32 {
		t.Fatalf("MasterEncryptionKey length = %d, want 32", len(cfg.MasterEncryptionKey))
	}
	if cfg.FunctionUploadSpoolDir != "/var/lib/supabase-manager/function-uploads" {
		t.Fatalf("FunctionUploadSpoolDir = %q", cfg.FunctionUploadSpoolDir)
	}
}

func TestLoadRejectsPublishedExampleSecrets(t *testing.T) {
	t.Setenv("MASTER_ENCRYPTION_KEY", "replace-with-output-of-openssl-rand-base64-32")
	t.Setenv("PROVISIONER_TOKEN", "replace-with-output-of-openssl-rand-hex-32")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted example secrets")
	}
}

func TestLoadRejectsPublishedZeroMasterKey(t *testing.T) {
	t.Setenv("MASTER_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("PROVISIONER_TOKEN", strings.Repeat("b", 32))
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted the published all-zero master key")
	}
}
