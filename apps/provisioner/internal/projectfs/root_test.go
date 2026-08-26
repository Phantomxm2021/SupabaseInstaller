package projectfs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	restore, commit, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("new-compose"), Env: []byte("new-env"), FunctionsEnv: []byte("FUNCTION_SECRET=secret")})
	if err != nil {
		t.Fatal(err)
	}
	runtimePath, err := root.RuntimeComposePath("bee")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(runtimePath); !os.IsNotExist(err) {
		t.Fatal("stage published before commit")
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	ref, err := root.CurrentRuntimeFiles("bee")
	if err != nil || ref.ProjectDir != project || ref.ComposeFile != filepath.Join(project, ".manager-runtime", "current", "docker-compose.yml") {
		t.Fatalf("runtime reference = %#v, error = %v", ref, err)
	}
	if _, err := os.Lstat(filepath.Join(project, "docker-compose.yml")); !os.IsNotExist(err) {
		t.Fatal("compatibility root compose mirror still exists")
	}
	for name, want := range map[string]string{"docker-compose.yml": "new-compose", ".env": "new-env", ".env.functions": "FUNCTION_SECRET=secret"} {
		path := filepath.Join(filepath.Dir(runtimePath), name)
		if got := string(mustRead(t, path)); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
		if info, err := os.Stat(filepath.Join(filepath.Dir(runtimePath), name)); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, want 0600", name, info.Mode().Perm())
		}
	}
	if err := restore(); err != nil {
		t.Fatal(err)
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
	runtimePath, _ := root.RuntimeComposePath("bee")
	if got := string(mustRead(t, runtimePath)); got != "compose-before" {
		t.Fatalf("staged input was not copied: %q", got)
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Dir(filepath.Dir(runtimePath))
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
	_, _, err = root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("orphan"), Env: []byte("orphan"), FunctionsEnv: []byte("orphan")})
	if err != nil {
		t.Fatal(err)
	}
	if err := root.CleanupAbandonedRuntimeCandidates("bee"); err != nil {
		t.Fatal(err)
	}
	matches, _ = filepath.Glob(filepath.Join(runtimeRoot, ".candidate-*"))
	if len(matches) != 0 {
		t.Fatalf("startup cleanup retained candidates: %v", matches)
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
	runtimePath, _ := root.RuntimeComposePath("bee")
	if got := string(mustRead(t, runtimePath)); got != "two" {
		t.Fatal("second generation was not selected")
	}
	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, runtimePath)); got != "one" {
		t.Fatalf("restore selected %q, want prior generation", got)
	}
}

func TestStageRuntimeFilesRestoreUsesGenerationCAS(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	restoreOne, commitOne, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("one"), Env: []byte("one"), FunctionsEnv: []byte("one")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitOne(); err != nil {
		t.Fatal(err)
	}
	restoreTwo, commitTwo, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("two"), Env: []byte("two"), FunctionsEnv: []byte("two")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitTwo(); err != nil {
		t.Fatal(err)
	}
	if err := restoreOne(); err == nil || !strings.Contains(err.Error(), "stale runtime generation") {
		t.Fatalf("restore of superseded generation error = %v", err)
	}
	composePath, _ := root.RuntimeComposePath("bee")
	if got := string(mustRead(t, composePath)); got != "two" {
		t.Fatalf("stale restore changed current generation to %q", got)
	}
	if err := restoreTwo(); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, composePath)); got != "one" {
		t.Fatalf("restore did not select prior committed generation: %q", got)
	}
}

func TestStageRuntimeFilesPreservesStableVolumeDataAcrossGenerations(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, _ := root.ProjectPath("legacy")
	if err := os.MkdirAll(filepath.Join(project, "volumes", "db"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "volumes", "storage"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "volumes", "db", "sentinel"), []byte("db-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "volumes", "storage", "sentinel"), []byte("storage-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"one", "two"} {
		_, commit, err := root.StageRuntimeFiles("legacy", RuntimeFiles{Compose: []byte(content), Env: []byte(content), FunctionsEnv: []byte(content)})
		if err != nil {
			t.Fatal(err)
		}
		if err := commit(); err != nil {
			t.Fatal(err)
		}
	}
	if got := string(mustRead(t, filepath.Join(project, "volumes", "db", "sentinel"))); got != "db-data" {
		t.Fatal("database volume sentinel changed")
	}
	if got := string(mustRead(t, filepath.Join(project, "volumes", "storage", "sentinel"))); got != "storage-data" {
		t.Fatal("storage volume sentinel changed")
	}
}

func TestStageRuntimeFilesMigratesLegacyRootRuntimeFiles(t *testing.T) {
	base := t.TempDir()
	root, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	project, err := root.ProjectPath("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "volumes", "db"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "volumes", "storage"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "functions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "project.json"), []byte(`{"projectId":"legacy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"docker-compose.yml": "stale-compose",
		".env":               "stale-env",
		".env.functions":     "stale-functions",
	} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(project, "volumes", "db", "sentinel"):      "db-data",
		filepath.Join(project, "volumes", "storage", "sentinel"): "storage-data",
		filepath.Join(project, "functions", "main.ts"):           "function-source",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	_, commit, err := root.StageRuntimeFiles("legacy", RuntimeFiles{Compose: []byte("current-compose"), Env: []byte("current-env"), FunctionsEnv: []byte("current-functions")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"docker-compose.yml", ".env", ".env.functions"} {
		if _, err := os.Lstat(filepath.Join(project, name)); !os.IsNotExist(err) {
			t.Fatalf("legacy root %s still exists: %v", name, err)
		}
	}
	current := filepath.Join(project, ".manager-runtime", "current")
	for name, want := range map[string]string{
		"docker-compose.yml": "current-compose",
		".env":               "current-env",
		".env.functions":     "current-functions",
	} {
		if got := string(mustRead(t, filepath.Join(current, name))); got != want {
			t.Fatalf("current %s = %q, want %q", name, got, want)
		}
	}
	for path, want := range map[string]string{
		filepath.Join(project, "project.json"):                   `{"projectId":"legacy"}`,
		filepath.Join(project, "volumes", "db", "sentinel"):      "db-data",
		filepath.Join(project, "volumes", "storage", "sentinel"): "storage-data",
		filepath.Join(project, "functions", "main.ts"):           "function-source",
	} {
		if got := string(mustRead(t, path)); got != want {
			t.Fatalf("user data %s = %q, want %q", path, got, want)
		}
	}
}

func TestStageRuntimeFilesSkipsNonRegularLegacyRuntimeEntries(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project, _ := root.ProjectPath("legacy")
	if err := os.MkdirAll(filepath.Join(project, ".env.functions"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, commit, err := root.StageRuntimeFiles("legacy", RuntimeFiles{Compose: []byte("compose"), Env: []byte("env"), FunctionsEnv: []byte("functions")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(project, ".env.functions")); err != nil || !info.IsDir() {
		t.Fatalf("non-regular legacy entry was removed or changed: info=%v err=%v", info, err)
	}
}

func TestStageRuntimeFilesRestoresChainedGenerations(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	restoreA, commitA, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("A"), Env: []byte("A"), FunctionsEnv: []byte("A")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitA(); err != nil {
		t.Fatal(err)
	}
	restoreB, commitB, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("B"), Env: []byte("B"), FunctionsEnv: []byte("B")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitB(); err != nil {
		t.Fatal(err)
	}
	restoreC, commitC, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("C"), Env: []byte("C"), FunctionsEnv: []byte("C")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitC(); err != nil {
		t.Fatal(err)
	}
	compose, _ := root.RuntimeComposePath("bee")
	targetC := assertCurrentRuntime(t, root, "bee", compose, "C")
	if err := restoreC(); err != nil {
		t.Fatal(err)
	}
	targetB := assertCurrentRuntime(t, root, "bee", compose, "B")
	if err := restoreB(); err != nil {
		t.Fatal(err)
	}
	targetA := assertCurrentRuntime(t, root, "bee", compose, "A")
	if targetC == targetB || targetB == targetA || targetC == targetA {
		t.Fatalf("generation links were not distinct: A=%q B=%q C=%q", targetA, targetB, targetC)
	}
	if err := restoreA(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(compose); !os.IsNotExist(err) {
		t.Fatalf("restoring first generation retained current runtime: %v", err)
	}
}

func TestStageRuntimeFilesRejectsStaleRestoreAfterNewerUnrelatedCommit(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, commitA, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("A"), Env: []byte("A"), FunctionsEnv: []byte("A")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitA(); err != nil {
		t.Fatal(err)
	}
	restoreB, commitB, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("B"), Env: []byte("B"), FunctionsEnv: []byte("B")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitB(); err != nil {
		t.Fatal(err)
	}
	_, commitC, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte("C"), Env: []byte("C"), FunctionsEnv: []byte("C")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitC(); err != nil {
		t.Fatal(err)
	}
	if err := restoreB(); err == nil || !strings.Contains(err.Error(), "stale runtime generation") {
		t.Fatalf("stale restore error = %v", err)
	}
	compose, _ := root.RuntimeComposePath("bee")
	assertCurrentRuntime(t, root, "bee", compose, "C")
}

func TestNewCleansAbandonedRuntimeCandidates(t *testing.T) {
	base := t.TempDir()
	project := filepath.Join(base, "bee")
	runtimeRoot := filepath.Join(project, ".manager-runtime")
	generation := filepath.Join(runtimeRoot, "generations", "generation-keep")
	candidate := filepath.Join(runtimeRoot, ".candidate-orphan")
	if err := os.MkdirAll(generation, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(candidate, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generation, "docker-compose.yml"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(candidate, "docker-compose.yml"), []byte("orphan"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("generations", "generation-keep"), filepath.Join(runtimeRoot, "current")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
		t.Fatalf("startup retained abandoned candidate: %v", err)
	}
	if got := string(mustRead(t, filepath.Join(runtimeRoot, "current", "docker-compose.yml"))); got != "keep" {
		t.Fatalf("startup changed current generation to %q", got)
	}
	if _, err := os.Stat(generation); err != nil {
		t.Fatalf("startup removed committed generation: %v", err)
	}
}

func TestConcurrentRuntimeStagesSerializePublication(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	for _, value := range []string{"one", "two"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			restore, commit, err := root.StageRuntimeFiles("bee", RuntimeFiles{Compose: []byte(value), Env: []byte(value), FunctionsEnv: []byte(value)})
			if err != nil {
				errCh <- err
				return
			}
			if err := commit(); err != nil {
				errCh <- err
				return
			}
			if err := restore(); err != nil && !strings.Contains(err.Error(), "stale runtime generation") {
				errCh <- err
			}
		}(value)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestStageRuntimeFilesSyncsGenerationsBeforePublishingCurrent(t *testing.T) {
	base := t.TempDir()
	root, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	project, err := root.ProjectPath("ordered")
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Join(project, ".manager-runtime")
	generations := filepath.Join(runtimeRoot, "generations")
	var operations []string
	root.hooks.operation = func(operation string) {
		operations = append(operations, operation)
	}
	_, commit, err := root.StageRuntimeFiles("ordered", RuntimeFiles{Compose: []byte("compose"), Env: []byte("env"), FunctionsEnv: []byte("functions")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	indexOf := func(want string, after int) int {
		for index := after + 1; index < len(operations); index++ {
			if operations[index] == want {
				return index
			}
		}
		return -1
	}
	generationRename := indexOf("rename-generation", -1)
	if generationRename < 0 {
		t.Fatalf("operation log missing candidate-to-generation rename: %v", operations)
	}
	generationSync := indexOf("sync:"+generations, generationRename)
	if generationSync < 0 {
		t.Fatalf("operation log missing generations fsync after rename: %v", operations)
	}
	currentRename := indexOf("rename-current", generationSync)
	if currentRename < 0 {
		t.Fatalf("operation log missing current replacement after generations fsync: %v", operations)
	}
	currentSync := indexOf("sync:"+runtimeRoot, currentRename)
	if currentSync < 0 {
		t.Fatalf("operation log missing runtime parent fsync after current replacement: %v", operations)
	}
}

func TestStageRuntimeFilesGenerationSyncFailureLeavesPreviousCurrent(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, commitA, err := root.StageRuntimeFiles("sync-failure", RuntimeFiles{Compose: []byte("A"), Env: []byte("A"), FunctionsEnv: []byte("A")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitA(); err != nil {
		t.Fatal(err)
	}
	failed := false
	root.hooks.syncDirectory = func(directory string) error {
		if filepath.Base(directory) == "generations" && !failed {
			failed = true
			return errors.New("injected generations fsync failure")
		}
		return syncDirectory(directory)
	}
	restoreB, commitB, err := root.StageRuntimeFiles("sync-failure", RuntimeFiles{Compose: []byte("B"), Env: []byte("B"), FunctionsEnv: []byte("B")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitB(); err == nil || !strings.Contains(err.Error(), "injected generations fsync failure") {
		t.Fatalf("commit error = %v, want generations fsync failure", err)
	}
	composePath, err := root.RuntimeComposePath("sync-failure")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, composePath)); got != "A" {
		t.Fatalf("current changed after pre-publication fsync failure: %q", got)
	}
	if err := restoreB(); err != nil {
		t.Fatalf("restore after generations fsync failure: %v", err)
	}
	if got := string(mustRead(t, composePath)); got != "A" {
		t.Fatalf("current changed after failed generation cleanup: %q", got)
	}
}

func TestStageRuntimeFilesCurrentSyncFailureRestoresPreviousCurrent(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, commitA, err := root.StageRuntimeFiles("pointer-sync-failure", RuntimeFiles{Compose: []byte("A"), Env: []byte("A"), FunctionsEnv: []byte("A")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitA(); err != nil {
		t.Fatal(err)
	}
	failed := false
	root.hooks.syncDirectory = func(directory string) error {
		if filepath.Base(directory) == ".manager-runtime" && !failed {
			failed = true
			return errors.New("injected current fsync failure")
		}
		return syncDirectory(directory)
	}
	restoreB, commitB, err := root.StageRuntimeFiles("pointer-sync-failure", RuntimeFiles{Compose: []byte("B"), Env: []byte("B"), FunctionsEnv: []byte("B")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitB(); err == nil || !strings.Contains(err.Error(), "injected current fsync failure") {
		t.Fatalf("commit error = %v, want current fsync failure", err)
	}
	composePath, err := root.RuntimeComposePath("pointer-sync-failure")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, composePath)); got != "A" {
		t.Fatalf("current changed after pointer fsync failure: %q", got)
	}
	if err := restoreB(); err != nil {
		t.Fatalf("restore after pointer fsync failure: %v", err)
	}
	if got := string(mustRead(t, composePath)); got != "A" {
		t.Fatalf("current changed after failed pointer cleanup: %q", got)
	}
}

func TestWriteRuntimeFilesRestoresWhenCommitFails(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, commit, err := root.StageRuntimeFiles("write-failure", RuntimeFiles{Compose: []byte("old"), Env: []byte("old"), FunctionsEnv: []byte("old")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commit(); err != nil {
		t.Fatal(err)
	}
	failed := false
	root.hooks.syncDirectory = func(directory string) error {
		if filepath.Base(directory) == "generations" && !failed {
			failed = true
			return errors.New("injected write commit failure")
		}
		return syncDirectory(directory)
	}
	if err := root.WriteRuntimeFiles("write-failure", []byte("new"), []byte("new")); err == nil || !strings.Contains(err.Error(), "injected write commit failure") {
		t.Fatalf("WriteRuntimeFiles error = %v, want commit failure", err)
	}
	path, _ := root.RuntimeComposePath("write-failure")
	if got := string(mustRead(t, path)); got != "old" {
		t.Fatalf("current after failed write = %q, want old", got)
	}
	entries, _ := os.ReadDir(filepath.Join(filepath.Dir(filepath.Dir(path)), "generations"))
	if len(entries) != 1 {
		t.Fatalf("generations after failed write = %d, want 1", len(entries))
	}
}

func TestNewRecoversAbandonedLegacyQuarantineWithoutCurrent(t *testing.T) {
	base := t.TempDir()
	project := filepath.Join(base, "legacy")
	runtimeRoot := filepath.Join(project, ".manager-runtime")
	quarantine := filepath.Join(runtimeRoot, ".legacy-quarantine-orphan")
	if err := os.MkdirAll(quarantine, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range legacyRuntimeNames {
		if err := os.WriteFile(filepath.Join(quarantine, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range legacyRuntimeNames {
		if got := string(mustRead(t, filepath.Join(project, name))); got != name {
			t.Fatalf("recovered %s = %q", name, got)
		}
	}
	if _, err := os.Lstat(quarantine); !os.IsNotExist(err) {
		t.Fatalf("quarantine remains after recovery: %v", err)
	}
	if err := root.CleanupAbandonedRuntimeCandidates("legacy"); err != nil {
		t.Fatal(err)
	}
}

func TestNewCleansLegacyQuarantineAfterCurrentWasPublished(t *testing.T) {
	base := t.TempDir()
	project := filepath.Join(base, "legacy")
	runtimeRoot := filepath.Join(project, ".manager-runtime")
	generation := filepath.Join(runtimeRoot, "generations", "generation-good")
	quarantine := filepath.Join(runtimeRoot, ".legacy-quarantine-after-publish")
	if err := os.MkdirAll(generation, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(quarantine, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generation, "docker-compose.yml"), []byte("good"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(quarantine, "docker-compose.yml"), []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("generations", "generation-good"), filepath.Join(runtimeRoot, "current")); err != nil {
		t.Fatal(err)
	}
	if _, err := New(base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(quarantine); !os.IsNotExist(err) {
		t.Fatalf("published quarantine remains: %v", err)
	}
	if got := string(mustRead(t, filepath.Join(runtimeRoot, "current", "docker-compose.yml"))); got != "good" {
		t.Fatalf("current changed to %q", got)
	}
}

func TestStageRuntimeFilesLegacyMoveFailureRollsBackBeforeCurrentRestore(t *testing.T) {
	base := t.TempDir()
	root, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	_, commitA, err := root.StageRuntimeFiles("legacy", RuntimeFiles{Compose: []byte("A"), Env: []byte("A"), FunctionsEnv: []byte("A")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitA(); err != nil {
		t.Fatal(err)
	}
	project, err := root.ProjectPath("legacy")
	if err != nil {
		t.Fatal(err)
	}
	legacyContents := map[string]string{
		"docker-compose.yml": "legacy-compose",
		".env":               "legacy-env",
		".env.functions":     "legacy-functions",
	}
	for name, contents := range legacyContents {
		if err := os.WriteFile(filepath.Join(project, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	moveCount := 0
	root.hooks.moveLegacy = func(source, destination string) error {
		if filepath.Dir(source) == project {
			moveCount++
			if moveCount == 2 {
				return errors.New("injected second legacy move failure")
			}
		}
		return os.Rename(source, destination)
	}
	restore, commitB, err := root.StageRuntimeFiles("legacy", RuntimeFiles{Compose: []byte("B"), Env: []byte("B"), FunctionsEnv: []byte("B")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commitB(); err == nil || !strings.Contains(err.Error(), "injected second legacy move failure") {
		t.Fatalf("commit error = %v, want injected cleanup failure", err)
	}
	if err := restore(); err != nil {
		t.Fatalf("restore after cleanup failure: %v", err)
	}
	composePath, err := root.RuntimeComposePath("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(t, composePath)); got != "A" {
		t.Fatalf("restored current = %q, want A", got)
	}
	for name, want := range legacyContents {
		if got := string(mustRead(t, filepath.Join(project, name))); got != want {
			t.Errorf("restored legacy %s = %q, want %q", name, got, want)
		}
	}
	entries, err := os.ReadDir(filepath.Join(project, ".manager-runtime"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".candidate-") || strings.HasPrefix(entry.Name(), ".legacy-quarantine-") {
			t.Errorf("runtime leaked transient entry %q", entry.Name())
		}
	}
	generations, err := os.ReadDir(filepath.Join(project, ".manager-runtime", "generations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 1 {
		t.Fatalf("generation count after rollback = %d, want 1", len(generations))
	}
}

func TestStageRuntimeFilesFirstMigrationFailureRestoresLegacyAndRemovesCurrent(t *testing.T) {
	base := t.TempDir()
	root, err := New(base)
	if err != nil {
		t.Fatal(err)
	}
	project, err := root.ProjectPath("first")
	if err != nil {
		t.Fatal(err)
	}
	legacyContents := map[string]string{
		"docker-compose.yml": "legacy-compose",
		".env":               "legacy-env",
		".env.functions":     "legacy-functions",
	}
	for name, contents := range legacyContents {
		if err := os.MkdirAll(project, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	moveCount := 0
	root.hooks.moveLegacy = func(source, destination string) error {
		if filepath.Dir(source) == project {
			moveCount++
			if moveCount == 2 {
				return errors.New("injected second legacy move failure")
			}
		}
		return os.Rename(source, destination)
	}
	restore, commit, err := root.StageRuntimeFiles("first", RuntimeFiles{Compose: []byte("current"), Env: []byte("current"), FunctionsEnv: []byte("current")})
	if err != nil {
		t.Fatal(err)
	}
	if err := commit(); err == nil {
		t.Fatal("commit succeeded despite injected cleanup failure")
	}
	if err := restore(); err != nil {
		t.Fatalf("restore first migration: %v", err)
	}
	current, err := root.RuntimeComposePath("first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(current); !os.IsNotExist(err) {
		t.Fatalf("first migration restore retained current: %v", err)
	}
	for name, want := range legacyContents {
		if got := string(mustRead(t, filepath.Join(project, name))); got != want {
			t.Errorf("restored legacy %s = %q, want %q", name, got, want)
		}
	}
	entries, err := os.ReadDir(filepath.Join(project, ".manager-runtime"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".candidate-") || strings.HasPrefix(entry.Name(), ".legacy-quarantine-") {
			t.Errorf("runtime leaked transient entry %q", entry.Name())
		}
	}
	generations, err := os.ReadDir(filepath.Join(project, ".manager-runtime", "generations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 0 {
		t.Fatalf("generation count after first migration rollback = %d, want 0", len(generations))
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

func assertCurrentRuntime(t *testing.T, root *Root, slug, composePath, want string) string {
	t.Helper()
	project, err := root.ProjectPath(slug)
	if err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(project, ".manager-runtime", "current")
	target, err := os.Readlink(current)
	if err != nil {
		t.Fatalf("read current runtime link: %v", err)
	}
	if !strings.HasPrefix(filepath.ToSlash(target), "generations/") {
		t.Fatalf("current runtime target = %q, want generation target", target)
	}
	if got := string(mustRead(t, composePath)); got != want {
		t.Fatalf("current runtime contents = %q, want %q", got, want)
	}
	return target
}
