package projectfs

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStageFunctionReleaseRequiresRootIndex(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	archive := zipFixture(t, map[string]string{"demo/index.ts": "Deno.serve(() => new Response('ok'))"})
	if _, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(archive)); err == nil {
		t.Fatal("StageFunctionRelease() succeeded, want root index rejection")
	}
}

func TestActivateFunctionReleaseCreatesCurrentPointer(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stage, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(zipFixture(t, map[string]string{"index.ts": "one"})))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.ActivateFunctionRelease("bee", "demo", stage); err != nil {
		t.Fatalf("ActivateFunctionRelease() error = %v", err)
	}
	current, err := root.FunctionCurrentPath("bee", "demo")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(current)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("current pointer = %v, %v; want symlink", info, err)
	}
	if body, err := os.ReadFile(filepath.Join(current, "index.ts")); err != nil || string(body) != "one" {
		t.Fatalf("current index = %q, %v", body, err)
	}
}

func TestRollbackFunctionReleaseRestoresPreviousRelease(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(zipFixture(t, map[string]string{"index.ts": "one"})))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.ActivateFunctionRelease("bee", "demo", first); err != nil {
		t.Fatal(err)
	}
	second, err := root.StageFunctionRelease("bee", "demo", "operation-2", bytes.NewReader(zipFixture(t, map[string]string{"index.ts": "two"})))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.ActivateFunctionRelease("bee", "demo", second); err != nil {
		t.Fatal(err)
	}
	if _, err := root.RollbackFunctionRelease("bee", "demo", "operation-3"); err != nil {
		t.Fatalf("RollbackFunctionRelease() error = %v", err)
	}
	current, err := root.FunctionCurrentPath("bee", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(current, "index.ts")); err != nil || string(body) != "one" {
		t.Fatalf("current index = %q, %v", body, err)
	}
}

func TestStageFunctionReleaseRejectsTraversal(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	archive := zipFixture(t, map[string]string{"index.ts": "ok", "../outside.ts": "bad"})
	if _, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(archive)); err == nil {
		t.Fatal("StageFunctionRelease() succeeded, want traversal rejection")
	}
}

func TestStageFunctionReleaseStagesValidArchiveWithoutLivePointer(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	archive := zipFixture(t, map[string]string{"index.ts": "Deno.serve(() => new Response('ok'))", "lib/tool.ts": "export const x = 1"})
	stage, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("StageFunctionRelease() error = %v", err)
	}
	if stage.SHA256 == "" {
		t.Fatal("stage SHA256 is empty")
	}
	if _, err := root.FunctionCurrentPath("bee", "demo"); err == nil {
		t.Fatal("live pointer exists before activation")
	}
}

func zipFixture(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
