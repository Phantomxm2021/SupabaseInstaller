package functionlogs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"supabase-manager/internal/contracts"
)

func testStore(t *testing.T, options Options) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "logs.db"), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func record(id, project, function string, timestamp time.Time, level contracts.FunctionLogLevel, message string) contracts.FunctionLogRecord {
	return contracts.FunctionLogRecord{ID: id, ProjectID: project, FunctionName: function, Timestamp: timestamp, IngestedAt: timestamp.Add(time.Second), ExecutionID: "exec", EventType: "Log", Level: level, Message: message}
}

func TestInsertBatchIsAtomicAndIdempotent(t *testing.T) {
	ctx := context.Background()
	store := testStore(t, Options{})
	now := time.Unix(100, 0)
	good := record("one", "project", "hello", now, contracts.FunctionLogLevelInfo, "ok")
	bad := record("two", "project", "bad/name", now, contracts.FunctionLogLevelInfo, "bad")
	if err := store.InsertBatch(ctx, []contracts.FunctionLogRecord{good, bad}); err == nil {
		t.Fatal("expected invalid batch error")
	}
	page, err := store.Query(ctx, "project", "hello", contracts.FunctionLogQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 0 {
		t.Fatalf("atomic insert left %d rows", len(page.Logs))
	}
	if err := store.InsertBatch(ctx, []contracts.FunctionLogRecord{good, good}); err != nil {
		t.Fatal(err)
	}
	page, err = store.Query(ctx, "project", "hello", contracts.FunctionLogQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 {
		t.Fatalf("got %d rows, want 1", len(page.Logs))
	}
}

func TestInsertBatchSanitizesBeforePersistence(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	functionsEnv := filepath.Join(dir, ".env.functions")
	if err := os.WriteFile(functionsEnv, []byte("PRIVATE_VALUE=insert-sentinel\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	redactor, err := LoadRedactor("", functionsEnv)
	if err != nil {
		t.Fatal(err)
	}
	store := testStore(t, Options{Redactor: redactor})
	now := time.Unix(150, 0).UTC()
	input := "insert-sentinel\x00" + strings.Repeat("界", 5000)
	if err := store.InsertBatch(ctx, []contracts.FunctionLogRecord{record("secret", "p", "fn", now, contracts.FunctionLogLevelInfo, input)}); err != nil {
		t.Fatal(err)
	}
	page, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	got := page.Logs[0]
	if strings.Contains(got.Message, "insert-sentinel") || strings.ContainsRune(got.Message, '\x00') {
		t.Fatalf("unsafe message persisted: %q", got.Message)
	}
	if len(got.Message) > 10*1024 || !utf8.ValidString(got.Message) || !got.Truncated {
		t.Fatalf("invalid persisted bounds: bytes=%d valid=%t truncated=%t", len(got.Message), utf8.ValidString(got.Message), got.Truncated)
	}
}

func TestQueryOrderingIsolationFiltersAndCursors(t *testing.T) {
	ctx := context.Background()
	store := testStore(t, Options{})
	base := time.Unix(200, 0).UTC()
	rows := []contracts.FunctionLogRecord{
		record("a", "p", "fn", base, contracts.FunctionLogLevelInfo, "percent 100%"),
		record("b", "p", "fn", base, contracts.FunctionLogLevelError, "needle_value"),
		record("c", "p", "fn", base.Add(time.Second), contracts.FunctionLogLevelInfo, "NEEDLE_value"),
		record("d", "other", "fn", base.Add(2*time.Second), contracts.FunctionLogLevelInfo, "needle_value"),
		record("e", "p", "other", base.Add(3*time.Second), contracts.FunctionLogLevelInfo, "needle_value"),
	}
	if err := store.InsertBatch(ctx, rows); err != nil {
		t.Fatal(err)
	}
	page, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{page.Logs[0].ID, page.Logs[1].ID}; got[0] != "c" || got[1] != "b" {
		t.Fatalf("order = %v", got)
	}
	if page.OlderCursor == "" || page.NewerCursor == "" {
		t.Fatalf("missing cursors: %+v", page)
	}
	older, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 2, Before: page.OlderCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Logs) != 1 || older.Logs[0].ID != "a" {
		t.Fatalf("older = %+v", older.Logs)
	}
	newer, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 5, After: older.NewerCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(newer.Logs) != 2 || newer.Logs[0].ID != "c" || newer.Logs[1].ID != "b" {
		t.Fatalf("newer = %+v", newer.Logs)
	}
	filtered, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 10, Level: "error", Search: "needle_"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Logs) != 1 || filtered.Logs[0].ID != "b" {
		t.Fatalf("filtered = %+v", filtered.Logs)
	}
	literal, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 10, Search: "%"})
	if err != nil {
		t.Fatal(err)
	}
	if len(literal.Logs) != 1 || literal.Logs[0].ID != "a" {
		t.Fatalf("literal search = %+v", literal.Logs)
	}
}

func TestMaintainExpiresThenDeletesOldestUntilUnderCapacity(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(10_000_000, 0).UTC()
	probes := []int64{600, 600, 400}
	store := testStore(t, Options{Now: func() time.Time { return now }, Retention: 7 * 24 * time.Hour, MaxBytes: 512, SizeBytes: func(string) (int64, error) {
		value := probes[0]
		if len(probes) > 1 {
			probes = probes[1:]
		}
		return value, nil
	}})
	rows := []contracts.FunctionLogRecord{
		record("expired", "p", "fn", now.Add(-8*24*time.Hour), contracts.FunctionLogLevelInfo, "expired"),
		record("oldest", "p", "fn", now.Add(-2*time.Hour), contracts.FunctionLogLevelInfo, "oldest"),
		record("newest", "p", "fn", now.Add(-time.Hour), contracts.FunctionLogLevelInfo, "newest"),
	}
	if err := store.InsertBatch(ctx, rows); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 0 {
		t.Fatalf("capacity cleanup retained %+v", page.Logs)
	}
}

func TestMaintainExpiresOnlyRowsOlderThanSevenDaysWhenBelowCapacity(t *testing.T) {
	ctx := context.Background()
	now := time.Unix(20_000_000, 0).UTC()
	store := testStore(t, Options{
		Now:       func() time.Time { return now },
		SizeBytes: func(string) (int64, error) { return 100, nil },
		MaxBytes:  512,
	})
	if err := store.InsertBatch(ctx, []contracts.FunctionLogRecord{
		record("expired", "p", "fn", now.Add(-7*24*time.Hour-time.Nanosecond), contracts.FunctionLogLevelInfo, "expired"),
		record("boundary", "p", "fn", now.Add(-7*24*time.Hour), contracts.FunctionLogLevelInfo, "boundary"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	page, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 || page.Logs[0].ID != "boundary" {
		t.Fatalf("retained logs = %+v", page.Logs)
	}
}

func TestMaintainStopsWhenSizeProbeDoesNotShrinkAndNoRowsRemain(t *testing.T) {
	store := testStore(t, Options{MaxBytes: 1, SizeBytes: func(string) (int64, error) { return 2, nil }})
	if err := store.Maintain(context.Background()); err != nil {
		t.Fatal(err)
	}
}
