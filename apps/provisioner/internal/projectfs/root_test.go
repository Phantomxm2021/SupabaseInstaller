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
	runtimePath, err := root.RuntimePath("bee")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(runtimePath); !os.IsNotExist(err) {
		t.Fatal("stage published before commit")
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"docker-compose.yml": "new-compose", ".env": "new-env", ".env.functions": "FUNCTION_SECRET=secret"} {
		if got := string(mustRead(t, filepath.Join(runtimePath, name))); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
		if info, err := os.Stat(filepath.Join(runtimePath, name)); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, want 0600", name, info.Mode().Perm())
		}
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if string(mustRead(t, filepath.Join(project, "docker-compose.yml"))) != "old-compose" {
		t.Fatal("restore did not reinstall prior set")
	}
	if _, err := os.Lstat(runtimePath); !os.IsNotExist(err) {
		t.Fatal("restore retained the candidate pointer")
	}
}

func TestStageRuntimeFilesCopiesInputAndCleansAbortCandidates(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := RuntimeFiles{Compose: []byte("compose-before"), Env: []byte("env-before"), FunctionsEnv: []byte("fn-before")}
	restore, commit, err := root.StageRuntimeFiles("bee", files)
	if err != nil {
		t.Fatal(err)
	}
	files.Compose[0] = 'X'
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	runtimePath, _ := root.RuntimePath("bee")
	if got := string(mustRead(t, filepath.Join(runtimePath, "docker-compose.yml"))); got != "compose-before" {
		t.Fatalf("staged input was not copied: %q", got)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Dir(runtimePath)
	matches, _ := filepath.Glob(filepath.Join(runtimeRoot, ".candidate-*"))
	if len(matches) != 0 {
		t.Fatalf("abandoned candidates remain: %v", matches)
	}

	abortRestore, _, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("abort"), Env: []byte("abort"), FunctionsEnv: []byte("abort")})
	if err != nil {
		t.Fatal(err)
	}
	if err := abortRestore(); err != nil {
		t.Fatal(err)
	}
	matches, _ = filepath.Glob(filepath.Join(runtimeRoot, ".candidate-*"))
	if len(matches) != 0 {
		t.Fatalf("aborted candidates remain: %v", matches)
	}
}

func TestStageRuntimeFilesRestoreSwitchesToPriorGeneration(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, commit, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("one"), Env: []byte("one"), FunctionsEnv: []byte("one")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	restore, commit, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("two"), Env: []byte("two"), FunctionsEnv: []byte("two")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	runtimePath, _ := root.RuntimePath("bee")
	if got := string(mustRead(t, filepath.Join(runtimePath, "docker-compose.yml"))); got != "two" {
		t.Fatal("second generation was not selected")
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, filepath.Join(runtimePath, "docker-compose.yml"))); got != "one" {
		t.Fatalf("restore selected %q, want prior generation", got)
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
