package projectfs

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageFunctionReleaseRejectsUnrelatedEnclosingDirectory(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	archive := zipFixture(t, map[string]string{"project/index.ts": "Deno.serve(() => new Response('ok'))"})
	if _, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(archive)); err == nil {
		t.Fatal("StageFunctionRelease() succeeded, want root index rejection")
	}
}

func TestStageFunctionReleaseReportsArchiveEntriesWhenIndexIsMissing(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(zipFixture(t, map[string]string{
		"src/main.ts": "export default {}",
		"README.md":   "not an edge function entrypoint",
	})))
	if err == nil || !strings.Contains(err.Error(), "src/main.ts") {
		t.Fatalf("StageFunctionRelease() error = %v, want archive entry diagnostic", err)
	}
}

func TestStageFunctionReleaseAcceptsNamedEnclosingDirectory(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stage, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(zipFixture(t, map[string]string{
		"demo/index.ts":    "Deno.serve(() => new Response('ok'))",
		"demo/lib/tool.ts": "export const x = 1",
	})))
	if err != nil {
		t.Fatalf("StageFunctionRelease() error = %v", err)
	}
	if body, err := os.ReadFile(filepath.Join(stage.path, "index.ts")); err != nil || string(body) == "" {
		t.Fatalf("normalized index = %q, %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(stage.path, "demo", "index.ts")); !os.IsNotExist(err) {
		t.Fatalf("nested index still exists: %v", err)
	}
}

func TestStageFunctionReleaseIgnoresMacOSArchiveMetadata(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stage, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(zipFixture(t, map[string]string{
		"demo/index.ts":   "Deno.serve(() => new Response('ok'))",
		"__MACOSX/._demo": "AppleDouble metadata",
		"demo/.DS_Store":  "Finder metadata",
	})))
	if err != nil {
		t.Fatalf("StageFunctionRelease() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage.path, "index.ts")); err != nil {
		t.Fatalf("normalized index = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage.path, "__MACOSX")); !os.IsNotExist(err) {
		t.Fatalf("macOS metadata extracted: %v", err)
	}
}

func TestStageFunctionReleaseAcceptsSupabaseFunctionsDirectory(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stage, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(zipFixture(t, map[string]string{
		"supabase/functions/demo/index.ts":   "Deno.serve(() => new Response('ok'))",
		"supabase/functions/demo/deno.json":  "{}",
		"supabase/functions/other/README.md": "not part of archive",
	})))
	if err == nil {
		t.Fatal("StageFunctionRelease() succeeded with unrelated function directory")
	}
	stage, err = root.StageFunctionRelease("bee", "demo", "operation-2", bytes.NewReader(zipFixture(t, map[string]string{
		"supabase/functions/demo/index.ts":  "Deno.serve(() => new Response('ok'))",
		"supabase/functions/demo/deno.json": "{}",
	})))
	if err != nil {
		t.Fatalf("StageFunctionRelease() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage.path, "index.ts")); err != nil {
		t.Fatalf("normalized index = %v", err)
	}
}

func TestStageFunctionReleaseAcceptsProjectWrappedSupabaseDirectory(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stage, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(zipFixture(t, map[string]string{
		"my-project/supabase/functions/demo/index.ts": "Deno.serve(() => new Response('ok'))",
	})))
	if err != nil {
		t.Fatalf("StageFunctionRelease() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stage.path, "index.ts")); err != nil {
		t.Fatalf("normalized index = %v", err)
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

func TestListFunctionsReturnsCurrentAndPreviousRelease(t *testing.T) {
	root, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, release := range []struct{ operation, body string }{{"operation-1", "one"}, {"operation-2", "two"}} {
		stage, err := root.StageFunctionRelease("bee", "demo", release.operation, bytes.NewReader(zipFixture(t, map[string]string{"index.ts": release.body})))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := root.ActivateFunctionRelease("bee", "demo", stage); err != nil {
			t.Fatal(err)
		}
	}
	functions, err := root.ListFunctions("bee")
	if err != nil || len(functions) != 1 || functions[0].Name != "demo" || functions[0].Current == nil || functions[0].Previous == nil {
		t.Fatalf("ListFunctions() = %#v, %v", functions, err)
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
