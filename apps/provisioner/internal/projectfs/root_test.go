package projectfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectPathRejectsTraversalAndAbsoluteInput(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, slug := range []string{"../escape", "/tmp/escape", "bee/../../escape", "Bee", "bee_api"} {
		if _, err := root.ProjectPath(slug); err == nil {
			t.Errorf("ProjectPath(%q) succeeded, want rejection", slug)
		}
	}
}

func TestProjectPathReturnsContainedDirectory(t *testing.T) {
	base := t.TempDir()
	root, _ := New(base)
	path, err := root.ProjectPath("bee-2")
	if err != nil {
		t.Fatalf("ProjectPath() error = %v", err)
	}
	if path != base+"/bee-2" {
		t.Fatalf("ProjectPath() = %q, want %q", path, base+"/bee-2")
	}
}

func TestStageRuntimeFilesCommitsAndRestoresAsASet(t *testing.T) {
	base := t.TempDir()
	root, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	project, _ := root.ProjectPath("bee")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "docker-compose.yml"), []byte("old-compose"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("old-env"), 0o600); err != nil {
		t.Fatal(err)
	}
	restore, commit, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("new-compose"), Env: []byte("new-env"), FunctionsEnv: []byte("FUNCTION_SECRET=secret")})
	if err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, filepath.Join(project, "docker-compose.yml"))) != "old-compose" {
		t.Fatal("stage published before commit")
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"docker-compose.yml": "new-compose", ".env": "new-env", ".env.functions": "FUNCTION_SECRET=secret"} {
		if got := string(mustRead(t, filepath.Join(project, name))); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
		if info, err := os.Stat(filepath.Join(project, name)); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, want 0600", name, info.Mode().Perm())
		}
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, filepath.Join(project, "docker-compose.yml"))) != "old-compose" || string(mustRead(t, filepath.Join(project, ".env"))) != "old-env" {
		t.Fatal("restore did not reinstall prior set")
	}
	if _, err := os.Stat(filepath.Join(project, ".env.functions")); !os.IsNotExist(err) {
		t.Fatal("restore retained a file absent from prior set")
	}
	if got := string(mustRead(t, filepath.Join(project, ".manager-last-good", "docker-compose.yml"))); got != "old-compose" {
		t.Fatalf("last-good = %q", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
