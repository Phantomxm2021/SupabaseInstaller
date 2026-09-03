package functionlogs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	_ "modernc.org/sqlite"

	"supabase-manager/internal/contracts"
)

const (
	defaultRetention = 7 * 24 * time.Hour
	defaultMaxBytes  = int64(512 * 1024 * 1024)
	maintenanceBatch = 10_000
)

type Options struct {
	Now       func() time.Time
	SizeBytes func(path string) (int64, error)
	Redactor  *Redactor
	Retention time.Duration
	MaxBytes  int64
}

type Store struct {
	db              *sql.DB
	path            string
	now             func() time.Time
	sizeBytes       func(string) (int64, error)
	retention       time.Duration
	maxBytes        int64
	redactor        *Redactor
	readerFile      *os.File
	readerTemp      string
	snapshotPath    string
	publishSnapshot func(context.Context) error
}

func Open(path string, options Options) (*Store, error) {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.SizeBytes == nil {
		options.SizeBytes = func(path string) (int64, error) {
			info, err := os.Stat(path)
			if err != nil {
				return 0, err
			}
			size := info.Size()
			if wal, walErr := os.Stat(path + "-wal"); walErr == nil {
				size += wal.Size()
			} else if !errors.Is(walErr, os.ErrNotExist) {
				return 0, walErr
			}
			return size, nil
		}
	}
	if options.Retention <= 0 {
		options.Retention = defaultRetention
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = defaultMaxBytes
	}
	if options.Redactor == nil {
		options.Redactor = &Redactor{}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, path: path, now: options.Now, sizeBytes: options.SizeBytes, retention: options.Retention, maxBytes: options.MaxBytes, redactor: options.Redactor}
	store.snapshotPath = filepath.Join(filepath.Dir(path), "function-logs.read.db")
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; CREATE TABLE IF NOT EXISTS function_logs (
		event_id TEXT NOT NULL UNIQUE, project_id TEXT NOT NULL, function_name TEXT NOT NULL,
		timestamp_ns INTEGER NOT NULL, ingested_at_ns INTEGER NOT NULL, execution_id TEXT NOT NULL,
		level TEXT NOT NULL, event_type TEXT NOT NULL, message TEXT NOT NULL, truncated INTEGER NOT NULL
	); DROP INDEX IF EXISTS function_logs_project_function_time;
	CREATE INDEX IF NOT EXISTS function_logs_lookup ON function_logs(project_id, function_name, timestamp_ns DESC, event_id DESC);`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize function log store: %w", err)
	}
	if err = store.Maintain(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("maintain function log store: %w", err)
	}
	return store, nil
}

func (s *Store) publishReadSnapshot(ctx context.Context) error {
	if s.publishSnapshot != nil {
		return s.publishSnapshot(ctx)
	}
	temp, err := os.CreateTemp(filepath.Dir(s.snapshotPath), ".function-logs-read-*.db")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return err
	}
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, tempPath); err != nil {
		return err
	}
	file, err := os.Open(tempPath)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return err
	}
	if err := os.Rename(tempPath, s.snapshotPath); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(s.snapshotPath))
	if err != nil {
		return err
	}
	syncErr = directory.Sync()
	closeErr = directory.Close()
	return errors.Join(syncErr, closeErr)
}

// OpenReader opens an existing SQLite log database without running schema or
// retention work. mode=ro also prevents accidental writes at the driver layer.
func OpenReader(path string, now func() time.Time) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("invalid function log database path")
	}
	parent := filepath.Dir(abs)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, errors.New("function log database parent is unsafe")
	}
	parent = resolvedParent
	parentFD, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Openat(parentFD, filepath.Base(abs), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	_ = unix.Close(parentFD)
	if err != nil {
		return nil, err
	}
	readerFile := os.NewFile(uintptr(fd), filepath.Base(abs))
	return OpenReaderFile(readerFile, now)
}

// OpenReaderFile consumes a no-follow, regular snapshot descriptor and copies
// only that immutable standalone SQLite file into a private query location.
func OpenReaderFile(readerFile *os.File, now func() time.Time) (*Store, error) {
	if readerFile == nil {
		return nil, errors.New("open function log database")
	}
	info, err := readerFile.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = readerFile.Close()
		return nil, errors.New("function log database is not a regular file")
	}
	if now == nil {
		now = time.Now
	}
	tempDir, err := os.MkdirTemp("", "function-log-reader-*")
	if err != nil {
		_ = readerFile.Close()
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tempDir); _ = readerFile.Close() }
	snapshotPath := filepath.Join(tempDir, "function-logs.read.db")
	if err := copyOpenFile(readerFile, snapshotPath); err != nil {
		cleanup()
		return nil, err
	}
	dsn := (&url.URL{Scheme: "file", Path: snapshotPath, RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		cleanup()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		cleanup()
		return nil, err
	}
	return &Store{db: db, path: readerFile.Name(), now: now, readerFile: readerFile, readerTemp: tempDir}, nil
}

func copyOpenFile(source *os.File, destination string) error {
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return err
	}
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	return errors.Join(copyErr, closeErr)
}

func (s *Store) Close() error {
	dbErr := s.db.Close()
	if s.readerFile != nil {
		return errors.Join(dbErr, s.readerFile.Close(), os.RemoveAll(s.readerTemp))
	}
	return dbErr
}

func validateRecord(record contracts.FunctionLogRecord) error {
	if record.ID == "" || record.ProjectID == "" || record.ExecutionID == "" || record.EventType == "" {
		return errors.New("event, project, execution, and event type IDs are required")
	}
	if err := contracts.ValidateFunctionName(record.FunctionName); err != nil {
		return fmt.Errorf("invalid function name: %w", err)
	}
	switch record.Level {
	case contracts.FunctionLogLevelDebug, contracts.FunctionLogLevelInfo, contracts.FunctionLogLevelWarn, contracts.FunctionLogLevelError:
	default:
		return errors.New("invalid function log level")
	}
	switch record.EventType {
	case "Boot", "Log", "UncaughtException":
	default:
		return errors.New("invalid function log event type")
	}
	if record.Timestamp.IsZero() || record.IngestedAt.IsZero() {
		return errors.New("timestamp and ingested timestamp are required")
	}
	return nil
}

func (s *Store) InsertBatch(ctx context.Context, records []contracts.FunctionLogRecord) (resultErr error) {
	for _, record := range records {
		if err := validateRecord(record); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback()
		}
	}()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO function_logs(event_id,project_id,function_name,timestamp_ns,ingested_at_ns,execution_id,level,event_type,message,truncated) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(event_id) DO NOTHING`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, record := range records {
		var sanitizedTruncated bool
		record.Message, sanitizedTruncated = s.redactor.SanitizeMessage(record.Message)
		record.Truncated = record.Truncated || sanitizedTruncated
		if _, err = statement.ExecContext(ctx, record.ID, record.ProjectID, record.FunctionName, record.Timestamp.UnixNano(), record.IngestedAt.UnixNano(), record.ExecutionID, record.Level, record.EventType, record.Message, record.Truncated); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.publishReadSnapshot(ctx)
}

func (s *Store) Query(ctx context.Context, projectID, functionName string, query contracts.FunctionLogQuery) (contracts.FunctionLogPage, error) {
	if projectID == "" {
		return contracts.FunctionLogPage{}, errors.New("project ID is required")
	}
	if err := contracts.ValidateFunctionName(functionName); err != nil {
		return contracts.FunctionLogPage{}, err
	}
	if err := contracts.ValidateFunctionLogQuery(query); err != nil {
		return contracts.FunctionLogPage{}, err
	}
	where := []string{"project_id = ?", "function_name = ?"}
	args := []any{projectID, functionName}
	if query.Level != "" {
		where = append(where, "level = ?")
		args = append(args, query.Level)
	}
	if query.Search != "" {
		where = append(where, "instr(lower(message), lower(?)) > 0")
		args = append(args, query.Search)
	}
	encodedCursor := query.Before
	comparison := "<"
	if query.After != "" {
		encodedCursor, comparison = query.After, ">"
	}
	if encodedCursor != "" {
		cursor, err := contracts.DecodeFunctionLogCursor(encodedCursor)
		if err != nil {
			return contracts.FunctionLogPage{}, fmt.Errorf("decode function log cursor: %w", err)
		}
		where = append(where, fmt.Sprintf("(timestamp_ns %s ? OR (timestamp_ns = ? AND event_id %s ?))", comparison, comparison))
		args = append(args, cursor.Timestamp.UnixNano(), cursor.Timestamp.UnixNano(), cursor.ID)
	}
	args = append(args, query.Limit)
	rows, err := s.db.QueryContext(ctx, `SELECT event_id,project_id,function_name,timestamp_ns,ingested_at_ns,execution_id,level,event_type,message,truncated FROM function_logs WHERE `+strings.Join(where, " AND ")+` ORDER BY timestamp_ns DESC,event_id DESC LIMIT ?`, args...)
	if err != nil {
		return contracts.FunctionLogPage{}, err
	}
	defer rows.Close()
	page := contracts.FunctionLogPage{Logs: make([]contracts.FunctionLogRecord, 0), ServerTime: s.now()}
	for rows.Next() {
		var record contracts.FunctionLogRecord
		var timestampNS, ingestedNS int64
		if err := rows.Scan(&record.ID, &record.ProjectID, &record.FunctionName, &timestampNS, &ingestedNS, &record.ExecutionID, &record.Level, &record.EventType, &record.Message, &record.Truncated); err != nil {
			return contracts.FunctionLogPage{}, err
		}
		record.Timestamp, record.IngestedAt = time.Unix(0, timestampNS).UTC(), time.Unix(0, ingestedNS).UTC()
		page.Logs = append(page.Logs, record)
	}
	if err := rows.Err(); err != nil {
		return contracts.FunctionLogPage{}, err
	}
	if len(page.Logs) > 0 {
		page.NewerCursor, err = contracts.EncodeFunctionLogCursor(contracts.FunctionLogCursor{Timestamp: page.Logs[0].Timestamp, ID: page.Logs[0].ID})
		if err != nil {
			return contracts.FunctionLogPage{}, err
		}
		last := page.Logs[len(page.Logs)-1]
		page.OlderCursor, err = contracts.EncodeFunctionLogCursor(contracts.FunctionLogCursor{Timestamp: last.Timestamp, ID: last.ID})
	}
	return page, err
}

func (s *Store) Maintain(ctx context.Context) error {
	cutoff := s.now().Add(-s.retention).UnixNano()
	for {
		deleted, err := s.deleteBatch(ctx, `timestamp_ns < ?`, cutoff)
		if err != nil {
			return err
		}
		if deleted < maintenanceBatch {
			break
		}
	}
	for {
		size, err := s.sizeBytes(s.path)
		if err != nil {
			return err
		}
		if size <= s.maxBytes {
			return s.publishReadSnapshot(ctx)
		}
		deleted, err := s.deleteBatch(ctx, "1=1")
		if err != nil {
			return err
		}
		if deleted == 0 {
			return fmt.Errorf("function log store remains over capacity: %d bytes exceeds %d bytes", size, s.maxBytes)
		}
		if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
			return err
		}
	}
}

func (s *Store) deleteBatch(ctx context.Context, predicate string, args ...any) (count int64, resultErr error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback()
		}
	}()
	statement := fmt.Sprintf(`DELETE FROM function_logs WHERE rowid IN (SELECT rowid FROM function_logs WHERE %s ORDER BY timestamp_ns ASC,event_id ASC LIMIT %d)`, predicate, maintenanceBatch)
	result, err := tx.ExecContext(ctx, statement, args...)
	if err != nil {
		return 0, err
	}
	count, err = result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return count, tx.Commit()
}
