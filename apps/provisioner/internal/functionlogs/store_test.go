package functionlogs

import (
	"context"
	"database/sql"
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

func TestConcurrentMaintainSerializesCapacityPolicy(t *testing.T) {
	store := testStore(t, Options{SnapshotInterval: time.Hour})
	now := time.Now().UTC()
	records := make([]contracts.FunctionLogRecord, 512)
	for i := range records {
		id := fmt.Sprintf("capacity-%03d", i)
		records[i] = record(id, "p", "fn", now.Add(time.Duration(i)*time.Nanosecond), contracts.FunctionLogLevelInfo, id)
	}
	if err := store.InsertBatch(context.Background(), records); err != nil {
		t.Fatal(err)
	}
	store.maxBytes = 256
	store.sizeBytes = func(string) (int64, error) {
		var count int64
		err := store.db.QueryRow(`SELECT count(*) FROM function_logs`).Scan(&count)
		return count, err
	}
	var active, maximum atomic.Int32
	store.maintenanceEntered = func() {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() { <-start; errs <- store.Maintain(context.Background()) }()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var remaining int
	if err := store.db.QueryRow(`SELECT count(*) FROM function_logs`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 1 || remaining != 256 {
		t.Fatalf("maximum concurrent maintenance=%d remaining=%d", maximum.Load(), remaining)
	}
}

func TestInsertProgressesWhileMaintenanceIsBetweenBoundedSQLiteSteps(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	store := testStore(t, Options{SizeBytes: func(string) (int64, error) { return 0, nil }})
	store.sizeBytes = func(string) (int64, error) {
		close(entered)
		<-release
		return 0, nil
	}
	maintainDone := make(chan error, 1)
	go func() { maintainDone <- store.Maintain(context.Background()) }()
	<-entered
	insertDone := make(chan error, 1)
	go func() {
		insertDone <- store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("during-maintenance", "p", "fn", time.Now().UTC(), contracts.FunctionLogLevelInfo, "ok")})
	}()
	select {
	case err := <-insertDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("insert did not progress between bounded maintenance steps")
	}
	close(release)
	if err := <-maintainDone; err != nil {
		t.Fatal(err)
	}
}

func TestOpenConfiguresIncrementalAutoVacuumAndReopenPreservesIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "function-logs.db")
	store, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		store, err = Open(path, Options{})
		if err != nil {
			t.Fatal(err)
		}
		var mode int
		if err := store.db.QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
			t.Fatal(err)
		}
		if mode != 2 {
			t.Fatalf("auto_vacuum=%d", mode)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenMigratesLegacyAutoVacuumModeOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "function-logs.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE legacy_marker(value TEXT); INSERT INTO legacy_marker VALUES ('preserved')`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var mode int
	if err := store.db.QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	var marker string
	if err := store.db.QueryRow(`SELECT value FROM legacy_marker`).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	if mode != 2 || marker != "preserved" {
		t.Fatalf("mode=%d marker=%q", mode, marker)
	}
}

func TestReaderCloseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writer, err := Open(filepath.Join(dir, "function-logs.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReader(filepath.Join(dir, "function-logs.read.db"), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 8)
	for range 8 {
		go func() { errs <- reader.Close() }()
	}
	for range 8 {
		if err := <-errs; err != nil {
			t.Fatalf("repeated concurrent close: %v", err)
		}
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
	var failSync atomic.Bool
	failSync.Store(true)
	store.hooksMu.Lock()
	store.snapshotHooks.syncDirectory = func(directory *os.File) error {
		if failSync.Load() {
			return errors.New("sync failed")
		}
		return directory.Sync()
	}
	store.hooksMu.Unlock()
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
	failSync.Store(false)
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
	var failRename atomic.Bool
	failRename.Store(true)
	store.hooksMu.Lock()
	store.snapshotHooks.rename = func(oldPath, newPath string) error {
		if failRename.Load() {
			return errors.New("rename failed")
		}
		return os.Rename(oldPath, newPath)
	}
	store.hooksMu.Unlock()
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
	failRename.Store(false)
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
	if err := store.InsertBatch(context.Background(), nil); err == nil {
		t.Fatal("insert while closing succeeded")
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := store.CloseContext(ctx2); err != nil {
		t.Fatal(err)
	}
}

func TestCloseRetriesFailedPublicationAndFlushesLatestGeneration(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "function-logs.db"), Options{SnapshotInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	store.hooksMu.Lock()
	store.snapshotHooks.beforeRename = func() error {
		if calls.Add(1) == 1 {
			return errors.New("transient publish failure")
		}
		return nil
	}
	store.hooksMu.Unlock()
	if err := store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("close-retry", "p", "fn", time.Now().UTC(), contracts.FunctionLogLevelInfo, "ok")}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	waitForSnapshotRows(t, dir, 1)
}

func TestSnapshotConnectionDoesNotMonopolizeWriterConnection(t *testing.T) {
	store := testStore(t, Options{SnapshotInterval: time.Hour})
	if store.db == store.snapshotDB {
		t.Fatal("snapshot and writer connections are shared")
	}
	tx, err := store.snapshotDB.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM function_logs`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("separate", "p", "fn", time.Now().UTC(), contracts.FunctionLogLevelInfo, "ok")})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("writer waited for snapshot connection")
	}
}

func TestSnapshotPublisherRetriesFailedGenerationWhileIdle(t *testing.T) {
	store := testStore(t, Options{SnapshotInterval: time.Millisecond})
	var calls atomic.Int32
	store.publishSnapshot = func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("fail once")
		}
		return nil
	}
	if err := store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("retry", "p", "fn", time.Now().UTC(), contracts.FunctionLogLevelInfo, "ok")}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	store.snapshotMu.RLock()
	committed, published := store.committedGeneration, store.publishedGeneration
	healthy := store.snapshotHealthy
	store.snapshotMu.RUnlock()
	if calls.Load() < 2 || !healthy || committed != published {
		t.Fatalf("calls=%d healthy=%v committed=%d published=%d", calls.Load(), healthy, committed, published)
	}
}

func TestCloseWaitsForInflightInsertAndFlushesItsGeneration(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "function-logs.db"), Options{SnapshotInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	paused := make(chan struct{})
	release := make(chan struct{})
	store.beforeInsertCommit = func() { close(paused); <-release }
	insertDone := make(chan error, 1)
	go func() {
		insertDone <- store.InsertBatch(context.Background(), []contracts.FunctionLogRecord{record("raced", "p", "fn", time.Now().UTC(), contracts.FunctionLogLevelInfo, "ok")})
	}()
	<-paused
	closeDone := make(chan error, 1)
	go func() { closeDone <- store.CloseContext(context.Background()) }()
	select {
	case err := <-closeDone:
		t.Fatalf("close returned before in-flight insert: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-insertDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if err := store.InsertBatch(context.Background(), nil); err == nil {
		t.Fatal("insert after close succeeded")
	}
	waitForSnapshotRows(t, dir, 1)
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
