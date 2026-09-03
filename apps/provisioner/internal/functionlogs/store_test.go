package functionlogs

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

func TestSchemaHasExactLookupIndex(t *testing.T) {
	store := testStore(t, Options{})
	var sqlText string
	if err := store.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'function_logs_lookup'`).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	normalized := strings.Join(strings.Fields(sqlText), " ")
	want := "CREATE INDEX function_logs_lookup ON function_logs(project_id, function_name, timestamp_ns DESC, event_id DESC)"
	if normalized != want {
		t.Fatalf("index SQL = %q, want %q", normalized, want)
	}
}

func TestInsertBatchRejectsMalformedRecords(t *testing.T) {
	now := time.Unix(120, 0).UTC()
	valid := record("id", "project", "fn", now, contracts.FunctionLogLevelInfo, "")
	tests := map[string]func(*contracts.FunctionLogRecord){
		"missing event ID":      func(r *contracts.FunctionLogRecord) { r.ID = "" },
		"missing project ID":    func(r *contracts.FunctionLogRecord) { r.ProjectID = "" },
		"missing function name": func(r *contracts.FunctionLogRecord) { r.FunctionName = "" },
		"invalid function name": func(r *contracts.FunctionLogRecord) { r.FunctionName = "bad/name" },
		"missing timestamp":     func(r *contracts.FunctionLogRecord) { r.Timestamp = time.Time{} },
		"missing ingested at":   func(r *contracts.FunctionLogRecord) { r.IngestedAt = time.Time{} },
		"missing execution ID":  func(r *contracts.FunctionLogRecord) { r.ExecutionID = "" },
		"missing event type":    func(r *contracts.FunctionLogRecord) { r.EventType = "" },
		"unknown event type":    func(r *contracts.FunctionLogRecord) { r.EventType = "Shutdown" },
		"missing level":         func(r *contracts.FunctionLogRecord) { r.Level = "" },
		"unsupported log level": func(r *contracts.FunctionLogRecord) { r.Level = "fatal" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			store := testStore(t, Options{})
			candidate := valid
			mutate(&candidate)
			if err := store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{candidate}); err == nil {
				t.Fatal("expected validation error")
			}
		})
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
	probes := []int64{400, 600, 400}
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

func TestMaintainErrorsWhenStillOverCapacityWithNoRows(t *testing.T) {
	store := testStore(t, Options{})
	store.maxBytes = 1
	store.sizeBytes = func(string) (int64, error) { return 2, nil }
	if err := store.Maintain(context.Background()); err == nil || !strings.Contains(err.Error(), "over capacity") {
		t.Fatalf("error = %v, want over capacity", err)
	}
}

func TestStorePersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "logs.db")
	store, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.InsertBatch(ctx, []contracts.FunctionLogRecord{record("persisted", "p", "fn", now, contracts.FunctionLogLevelInfo, "hello")}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	page, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 || page.Logs[0].ID != "persisted" {
		t.Fatalf("reopened logs = %+v", page.Logs)
	}
}

func TestOpenReaderQueriesWithoutCreatingOrMutatingDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "logs.db")
	readPath := filepath.Join(filepath.Dir(path), "function-logs.read.db")
	if _, err := OpenReader(readPath, time.Now); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenReader(missing) error = %v, want not exist", err)
	}
	writer, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.InsertBatch(ctx, []contracts.FunctionLogRecord{record("one", "p", "fn", time.Now().UTC(), contracts.FunctionLogLevelInfo, "hello")}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReader(readPath, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	page, err := reader.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 || page.Logs[0].Message != "hello" {
		t.Fatalf("page = %#v", page)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("database mtime changed: %v -> %v", before.ModTime(), after.ModTime())
	}
}

func TestOpenReaderRejectsDatabaseSymlink(t *testing.T) {
	dir := t.TempDir()
	external := filepath.Join(t.TempDir(), "external.db")
	store, err := Open(external, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	link := filepath.Join(dir, "function-logs.read.db")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenReader(link, time.Now); err == nil {
		t.Fatal("OpenReader accepted database symlink")
	}
}

func TestOpenReaderReadsWALAndRemainsBoundAcrossPathSwap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs.db")
	now := time.Now().UTC()
	writer, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("original", "p", "fn", now, contracts.FunctionLogLevelInfo, "wal")}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	readPath := filepath.Join(dir, "function-logs.read.db")
	reader, err := OpenReader(readPath, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	fd := int(reader.readerFile.Fd())
	if err := os.Rename(readPath, readPath+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(readPath, []byte("not the opened snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := reader.Query(context.Background(), "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 || page.Logs[0].ID != "original" {
		t.Fatalf("page = %#v", page)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0); err == nil {
		t.Fatal("reader descriptor remains open")
	}
}

func TestSnapshotPublishFailurePreservesPreviousReadableSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "function-logs.db")
	now := time.Now().UTC()
	writer, err := Open(path, Options{SnapshotInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if err := writer.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("one", "p", "fn", now, contracts.FunctionLogLevelInfo, "one")}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		reader, e := OpenReader(filepath.Join(dir, "function-logs.read.db"), time.Now)
		if e == nil {
			page, _ := reader.Query(context.Background(), "p", "fn", contracts.FunctionLogQuery{Limit: 10})
			_ = reader.Close()
			if len(page.Logs) == 1 {
				break
			}
		}
		time.Sleep(time.Millisecond)
	}
	writer.publishSnapshot = func(context.Context) error { return errors.New("publish failed") }
	if err := writer.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("two", "p", "fn", now.Add(time.Second), contracts.FunctionLogLevelInfo, "two")}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for writer.SnapshotHealthy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if writer.SnapshotHealthy() {
		t.Fatal("snapshot failure not visible")
	}
	reader, err := OpenReader(filepath.Join(dir, "function-logs.read.db"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	page, err := reader.Query(context.Background(), "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Logs) != 1 || page.Logs[0].ID != "one" {
		t.Fatalf("page=%#v", page)
	}
}

func TestPublishedSnapshotsRemainConsistentDuringConcurrentInserts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "function-logs.db")
	writer, err := Open(path, Options{SnapshotInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	ctx := context.Background()
	base := time.Now().UTC()
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 40; i++ {
			id := fmt.Sprintf("event-%03d", i)
			if err := writer.InsertBatch(ctx, []contracts.FunctionLogRecord{record(id, "p", "fn", base.Add(time.Duration(i)*time.Second), contracts.FunctionLogLevelInfo, id)}); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	last := 0
	for {
		reader, openErr := OpenReader(filepath.Join(dir, "function-logs.read.db"), time.Now)
		if openErr != nil {
			t.Fatal(openErr)
		}
		page, queryErr := reader.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 200})
		_ = reader.Close()
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		if len(page.Logs) < last || len(page.Logs) > 40 {
			t.Fatalf("non-monotonic snapshot rows=%d last=%d", len(page.Logs), last)
		}
		last = len(page.Logs)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				reader, _ := OpenReader(filepath.Join(dir, "function-logs.read.db"), time.Now)
				final, _ := reader.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 200})
				_ = reader.Close()
				if len(final.Logs) == 40 {
					return
				}
				time.Sleep(time.Millisecond)
			}
			t.Fatal("final snapshot did not reach 40 rows")
		default:
		}
	}
}

func TestSnapshotPublisherCoalescesAndDoesNotBlockInsert(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "function-logs.db"), Options{SnapshotInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	store.publishSnapshot = func(context.Context) error {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return nil
	}
	start := time.Now()
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("rapid-%03d", i)
		if err := store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record(id, "p", "fn", time.Now().UTC(), contracts.FunctionLogLevelInfo, id)}); err != nil {
			t.Fatal(err)
		}
	}
	if time.Since(start) > time.Second {
		t.Fatal("inserts waited for snapshot publisher")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("publisher did not start")
	}
	close(release)
	time.Sleep(30 * time.Millisecond)
	if got := calls.Load(); got > 2 {
		t.Fatalf("publication calls=%d", got)
	}
}

func TestStoreCloseFlushesLatestDirtySnapshot(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "function-logs.db"), Options{SnapshotInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("latest", "p", "fn", now, contracts.FunctionLogLevelInfo, "latest")}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReader(filepath.Join(dir, "function-logs.read.db"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	page, err := reader.Query(context.Background(), "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	if err != nil || len(page.Logs) != 1 {
		t.Fatalf("page/error=%#v/%v", page, err)
	}
}

func TestPostRenameSyncFailureCommitsNewSnapshotButMarksUnhealthy(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "function-logs.db"), Options{SnapshotInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.snapshotHooks.syncDirectory = func(*os.File) error { return errors.New("sync failed") }
	now := time.Now().UTC()
	_ = store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("committed", "p", "fn", now, contracts.FunctionLogLevelInfo, "new")})
	deadline := time.Now().Add(time.Second)
	for store.SnapshotHealthy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	reader, err := OpenReader(filepath.Join(dir, "function-logs.read.db"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	page, err := reader.Query(context.Background(), "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	_ = reader.Close()
	if err != nil || len(page.Logs) != 1 || store.SnapshotHealthy() {
		t.Fatalf("page/healthy/error=%#v/%v/%v", page, store.SnapshotHealthy(), err)
	}
}

func TestRenameFailurePreservesPriorSnapshot(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "function-logs.db"), Options{SnapshotInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Now().UTC()
	_ = store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("old", "p", "fn", now, contracts.FunctionLogLevelInfo, "old")})
	waitForSnapshotRows(t, dir, 1)
	store.snapshotHooks.rename = func(string, string) error { return errors.New("rename failed") }
	_ = store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("new", "p", "fn", now.Add(time.Second), contracts.FunctionLogLevelInfo, "new")})
	deadline := time.Now().Add(time.Second)
	for store.SnapshotHealthy() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	reader, _ := OpenReader(filepath.Join(dir, "function-logs.read.db"), time.Now)
	page, _ := reader.Query(context.Background(), "p", "fn", contracts.FunctionLogQuery{Limit: 10})
	_ = reader.Close()
	if len(page.Logs) != 1 || page.Logs[0].ID != "old" {
		t.Fatalf("page=%#v", page)
	}
}

func waitForSnapshotRows(t *testing.T, dir string, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		reader, err := OpenReader(filepath.Join(dir, "function-logs.read.db"), time.Now)
		if err == nil {
			page, _ := reader.Query(context.Background(), "p", "fn", contracts.FunctionLogQuery{Limit: 200})
			_ = reader.Close()
			if len(page.Logs) == want {
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("snapshot did not reach %d rows", want)
}

func TestStoreCloseContextIsBoundedByBlockedPublisher(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "function-logs.db"), Options{SnapshotInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	store.publishSnapshot = func(context.Context) error { close(started); <-release; return nil }
	_ = store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("one", "p", "fn", time.Now().UTC(), contracts.FunctionLogLevelInfo, "one")})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("publisher did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := store.CloseContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("CloseContext error=%v", err)
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := store.CloseContext(ctx2); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentInsertAndQuerySmoke(t *testing.T) {
	store := testStore(t, Options{})
	ctx := context.Background()
	now := time.Unix(400, 0).UTC()
	var wait sync.WaitGroup
	errors := make(chan error, 2)
	wait.Add(2)
	go func() {
		defer wait.Done()
		errors <- store.InsertBatch(ctx, []contracts.FunctionLogRecord{record("concurrent", "p", "fn", now, contracts.FunctionLogLevelInfo, "hello")})
	}()
	go func() {
		defer wait.Done()
		_, err := store.Query(ctx, "p", "fn", contracts.FunctionLogQuery{Limit: 10})
		errors <- err
	}()
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}
