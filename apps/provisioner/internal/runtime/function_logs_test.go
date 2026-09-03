package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"supabase-manager/apps/provisioner/internal/functionlogs"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/internal/contracts"
)

func functionLogsBackendFixture(t *testing.T) (*Backend, *projectfs.Root, string) {
	t.Helper()
	root, err := projectfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.UpdateMetadata("bee", func(m *projectfs.Metadata) error { m.ProjectID = "project-1"; m.Slug = "bee"; return nil }); err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	zw := zip.NewWriter(&raw)
	file, _ := zw.Create("index.ts")
	_, _ = file.Write([]byte("serve()"))
	_ = zw.Close()
	stage, err := root.StageFunctionRelease("bee", "demo", "operation-1", bytes.NewReader(raw.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.ActivateFunctionRelease("bee", "demo", stage); err != nil {
		t.Fatal(err)
	}
	path, err := root.FunctionLogDatabasePath("bee")
	if err != nil {
		t.Fatal(err)
	}
	return NewBackend(root, nil, nil), root, path
}

func TestFunctionLogsRequiresManagedFunctionAndReturnsNotInstalled(t *testing.T) {
	backend, root, _ := functionLogsBackendFixture(t)
	project, _ := root.ProjectPath("bee")
	if err := os.MkdirAll(filepath.Join(project, "volumes", "functions", "unmanaged"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.FunctionLogs(context.Background(), "bee", "unmanaged", contracts.FunctionLogQuery{Limit: 10}); !errors.Is(err, contracts.ErrFunctionNotFound) {
		t.Fatalf("unmanaged error = %v", err)
	}
	page, err := backend.FunctionLogs(context.Background(), "bee", "demo", contracts.FunctionLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 0 || page.Health.Status != "not_installed" {
		t.Fatalf("page = %#v", page)
	}
}

func TestFunctionLogsQueriesOnlyProjectAndFunctionAndReportsHealth(t *testing.T) {
	backend, _, path := functionLogsBackendFixture(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store, err := functionlogs.Open(path, functionlogs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rows := []contracts.FunctionLogRecord{{ID: "wanted", ProjectID: "project-1", FunctionName: "demo", ExecutionID: "e", EventType: "Log", Message: "needle", Timestamp: now, IngestedAt: now, Level: "error"}, {ID: "other", ProjectID: "other", FunctionName: "demo", ExecutionID: "e", EventType: "Log", Message: "needle", Timestamp: now, IngestedAt: now, Level: "error"}, {ID: "other-function", ProjectID: "project-1", FunctionName: "elsewhere", ExecutionID: "e", EventType: "Log", Message: "needle", Timestamp: now, IngestedAt: now, Level: "error"}}
	if err := store.InsertBatch(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	if err := functionlogs.WriteHealthSnapshot(filepath.Join(filepath.Dir(path), "health.json"), contracts.FunctionLogHealth{Status: "healthy"}, now); err != nil {
		t.Fatal(err)
	}
	page, err := backend.FunctionLogs(context.Background(), "bee", "demo", contracts.FunctionLogQuery{Limit: 10, Level: "error", Search: "need"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 || page.Logs[0].ID != "wanted" || page.Health.Status != "healthy" {
		t.Fatalf("page = %#v", page)
	}
}

func TestFunctionLogsCorruptionReturnsSafeTypedFailure(t *testing.T) {
	backend, _, path := functionLogsBackendFixture(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("/private/secret corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := backend.FunctionLogs(context.Background(), "bee", "demo", contracts.FunctionLogQuery{Limit: 10})
	if !errors.Is(err, contracts.ErrFunctionLogsUnavailable) || strings.Contains(err.Error(), path) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v", err)
	}
}

func TestFunctionLogsCanonicalizesInvalidAndStaleHealth(t *testing.T) {
	backend, _, path := functionLogsBackendFixture(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := functionlogs.Open(path, functionlogs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	healthPath := filepath.Join(filepath.Dir(path), "health.json")
	if err := os.WriteFile(healthPath, []byte(`{"version":1,"updatedAt":"2026-01-01T00:00:00Z","health":{"status":"evil","detail":"/private/path"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := backend.FunctionLogs(context.Background(), "bee", "demo", contracts.FunctionLogQuery{Limit: 10})
	if err != nil || page.Health.Status != "incompatible" || page.Health.Detail != "" {
		t.Fatalf("invalid health page/error = %#v/%v", page, err)
	}
	if err := functionlogs.WriteHealthSnapshot(healthPath, contracts.FunctionLogHealth{Status: "healthy"}, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	page, err = backend.FunctionLogs(context.Background(), "bee", "demo", contracts.FunctionLogQuery{Limit: 10})
	if err != nil || page.Health.Status != "offline" {
		t.Fatalf("stale health page/error = %#v/%v", page, err)
	}
}
