package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAuthDirectoryRepairsNginxReadableMode(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "nginx-auth")

	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create restrictive auth directory: %v", err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatalf("set restrictive parent mode: %v", err)
	}
	if err := ensureAuthDirectory(dir); err != nil {
		t.Fatalf("ensure auth directory: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat auth directory: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
		t.Fatalf("auth directory mode = %o, want %o", got, want)
	}

	parentInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("stat auth parent directory: %v", err)
	}
	if got, want := parentInfo.Mode().Perm(), os.FileMode(0o711); got != want {
		t.Fatalf("auth parent directory mode = %o, want %o", got, want)
	}
}
