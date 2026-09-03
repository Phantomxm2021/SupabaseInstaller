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
	"sync"
	"time"

	"golang.org/x/sys/unix"

	_ "modernc.org/sqlite"

	"supabase-manager/internal/contracts"
)

const (
	defaultRetention        = 7 * 24 * time.Hour
	defaultMaxBytes         = int64(512 * 1024 * 1024)
	maintenanceBatch        = 10_000
	capacityDeleteBatch     = 256
	incrementalVacuumPages  = 256
	defaultSnapshotInterval = 5 * time.Second
)

type Options struct {
	Now              func() time.Time
	SizeBytes        func(path string) (int64, error)
	Redactor         *Redactor
	Retention        time.Duration
	MaxBytes         int64
	SnapshotInterval time.Duration
}

type Store struct {
	db                  *sql.DB
	snapshotDB          *sql.DB
	path                string
	now                 func() time.Time
	sizeBytes           func(string) (int64, error)
	retention           time.Duration
	maxBytes            int64
	redactor            *Redactor
	readerFile          *os.File
	readerTemp          string
	snapshotPath        string
	publishSnapshot     func(context.Context) error
	snapshotInterval    time.Duration
	dirty               chan struct{}
	closeRequest        chan snapshotCloseRequest
	publisherDone       chan struct{}
	dbCloseOnce         sync.Once
	closeErr            error
	closeCallMu         sync.Mutex
	lifecycleMu         sync.RWMutex
	maintenanceMu       sync.Mutex
	closing             bool
	snapshotMu          sync.RWMutex
	snapshotHealthy     bool
	committedGeneration uint64
	publishedGeneration uint64
	snapshotDurable     bool
	hooksMu             sync.RWMutex
	snapshotHooks       snapshotPublishHooks
	beforeInsertCommit  func()
	maintenanceEntered  func()
}

type snapshotCloseRequest struct {
	ctx    context.Context
	result chan error
}

type snapshotPublishHooks struct {
	beforeRename  func() error
	rename        func(string, string) error
	syncDirectory func(*os.File) error
}

func Open(path string, options Options) (*Store, error) {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Retention <= 0 {
		options.Retention = defaultRetention
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = defaultMaxBytes
	}
	if options.SnapshotInterval <= 0 {
		options.SnapshotInterval = defaultSnapshotInterval
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
	if store.sizeBytes == nil {
		store.sizeBytes = store.databaseUsageBytes
	}
	store.snapshotPath = filepath.Join(filepath.Dir(path), "function-logs.read.db")
	store.snapshotInterval = options.SnapshotInterval
	if err = configureIncrementalAutoVacuum(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize function log store: %w", err)
	}
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
	snapshotDSN := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String()
	snapshotDB, openErr := sql.Open("sqlite", snapshotDSN)
	if openErr != nil {
		_ = db.Close()
		return nil, openErr
	}
	snapshotDB.SetMaxOpenConns(1)
	if openErr = snapshotDB.Ping(); openErr != nil {
		_ = snapshotDB.Close()
		_ = db.Close()
		return nil, openErr
	}
	store.snapshotDB = snapshotDB
	if err = store.publishAndRecord(context.Background()); err != nil {
		_ = snapshotDB.Close()
		_ = db.Close()
		return nil, fmt.Errorf("publish initial function log snapshot: %w", err)
	}
	store.dirty = make(chan struct{}, 1)
	store.closeRequest = make(chan snapshotCloseRequest)
	store.publisherDone = make(chan struct{})
	go store.runSnapshotPublisher()
	return store, nil
}

func configureIncrementalAutoVacuum(db *sql.DB) error {
	var mode, tables int
	if err := db.QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		return err
	}
	if mode == 2 {
		return nil
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
		return err
	}
	if _, err := db.Exec(`PRAGMA auto_vacuum=INCREMENTAL`); err != nil {
		return err
	}
	if tables > 0 {
		// Existing databases need this one-time format migration. Subsequent
		// opens observe mode 2 and never perform a full rewrite.
		if _, err := db.Exec(`VACUUM`); err != nil {
			return fmt.Errorf("migrate function log store for incremental maintenance: %w", err)
		}
	}
	if err := db.QueryRow(`PRAGMA auto_vacuum`).Scan(&mode); err != nil {
		return err
	}
	if mode != 2 {
		return errors.New("function log store requires incremental auto-vacuum migration")
	}
	return nil
}

func (s *Store) markSnapshotDirty() {
	if s.dirty == nil {
		return
	}
	select {
	case s.dirty <- struct{}{}:
	default:
	}
}
func (s *Store) markCommitted() {
	s.snapshotMu.Lock()
	s.committedGeneration++
	s.snapshotMu.Unlock()
	s.markSnapshotDirty()
}

func (s *Store) needsSnapshot() bool {
	s.snapshotMu.RLock()
	defer s.snapshotMu.RUnlock()
	return s.publishedGeneration < s.committedGeneration || !s.snapshotDurable
}
func (s *Store) SnapshotHealthy() bool {
	s.snapshotMu.RLock()
	defer s.snapshotMu.RUnlock()
	return s.snapshotHealthy
}
func (s *Store) publishAndRecord(ctx context.Context) error {
	s.snapshotMu.RLock()
	target := s.committedGeneration
	s.snapshotMu.RUnlock()
	committed, err := s.publishReadSnapshot(ctx)
	s.snapshotMu.Lock()
	if committed && target > s.publishedGeneration {
		s.publishedGeneration = target
	}
	s.snapshotDurable = err == nil
	s.snapshotHealthy = err == nil
	s.snapshotMu.Unlock()
	return err
}

func (s *Store) runSnapshotPublisher() {
	defer close(s.publisherDone)
	var timer *time.Timer
	var timerC <-chan time.Time
	backoff := s.snapshotInterval
	for {
		select {
		case <-s.dirty:
			if timer == nil {
				timer = time.NewTimer(backoff)
				timerC = timer.C
			}
		case <-timerC:
			timer = nil
			timerC = nil
			err := s.publishAndRecord(context.Background())
			if err != nil {
				backoff = nextBackoff(backoff)
			} else {
				backoff = s.snapshotInterval
			}
			if s.needsSnapshot() {
				timer = time.NewTimer(backoff)
				timerC = timer.C
			}
		case request := <-s.closeRequest:
			if timer != nil {
				timer.Stop()
				timer, timerC = nil, nil
			}
			err := s.flushSnapshots(request.ctx)
			request.result <- err
			if err == nil {
				return
			}
		}
	}
}

func nextBackoff(backoff time.Duration) time.Duration {
	backoff *= 2
	if backoff > 5*time.Second {
		return 5 * time.Second
	}
	return backoff
}

func (s *Store) flushSnapshots(ctx context.Context) error {
	backoff := s.snapshotInterval
	for s.needsSnapshot() {
		if err := s.publishAndRecord(ctx); err != nil {
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			backoff = nextBackoff(backoff)
		}
	}
	return nil
}

func (s *Store) publishReadSnapshot(ctx context.Context) (bool, error) {
	if s.publishSnapshot != nil {
		err := s.publishSnapshot(ctx)
		return err == nil, err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.snapshotPath), ".function-logs-read-*.db")
	if err != nil {
		return false, err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		return false, err
	}
	_ = os.Remove(tempPath)
	defer os.Remove(tempPath)
	if _, err := s.snapshotDB.ExecContext(ctx, `VACUUM INTO ?`, tempPath); err != nil {
		return false, err
	}
	file, err := os.Open(tempPath)
	if err != nil {
		return false, err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return false, err
	}
	s.hooksMu.RLock()
	hooks := s.snapshotHooks
	s.hooksMu.RUnlock()
	if hooks.beforeRename != nil {
		if err := hooks.beforeRename(); err != nil {
			return false, err
		}
	}
	rename := os.Rename
	if hooks.rename != nil {
		rename = hooks.rename
	}
	if err := rename(tempPath, s.snapshotPath); err != nil {
		return false, err
	}
	directory, err := os.Open(filepath.Dir(s.snapshotPath))
	if err != nil {
		// Rename is the publication commit point. Failure to fsync its parent is
		// a durability warning, but the new snapshot is already visible.
		return true, err
	}
	if hooks.syncDirectory != nil {
		syncErr = hooks.syncDirectory(directory)
	} else {
		syncErr = directory.Sync()
	}
	closeErr = directory.Close()
	return true, errors.Join(syncErr, closeErr)
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
	if s.readerFile != nil {
		s.dbCloseOnce.Do(func() {
			s.closeErr = errors.Join(s.db.Close(), s.readerFile.Close(), os.RemoveAll(s.readerTemp))
		})
		return s.closeErr
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.CloseContext(ctx)
}

func (s *Store) CloseContext(ctx context.Context) error {
	if s.readerFile != nil {
		return errors.New("reader does not support CloseContext")
	}
	s.closeCallMu.Lock()
	defer s.closeCallMu.Unlock()

	s.lifecycleMu.Lock()
	s.closing = true
	s.lifecycleMu.Unlock()

	select {
	case <-s.publisherDone:
		return s.closeDatabases()
	default:
	}
	request := snapshotCloseRequest{ctx: ctx, result: make(chan error, 1)}
	select {
	case s.closeRequest <- request:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.result:
		if err != nil {
			return err
		}
		<-s.publisherDone
		return s.closeDatabases()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Store) closeDatabases() error {
	s.dbCloseOnce.Do(func() {
		s.closeErr = errors.Join(s.snapshotDB.Close(), s.db.Close())
	})
	return s.closeErr
}

func (s *Store) beginOperation() error {
	s.lifecycleMu.RLock()
	if s.closing {
		s.lifecycleMu.RUnlock()
		return errors.New("function log store is closing")
	}
	return nil
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
	if err := s.beginOperation(); err != nil {
		return err
	}
	defer s.lifecycleMu.RUnlock()
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
	var inserted int64
	for _, record := range records {
		var sanitizedTruncated bool
		record.Message, sanitizedTruncated = s.redactor.SanitizeMessage(record.Message)
		record.Truncated = record.Truncated || sanitizedTruncated
		result, execErr := statement.ExecContext(ctx, record.ID, record.ProjectID, record.FunctionName, record.Timestamp.UnixNano(), record.IngestedAt.UnixNano(), record.ExecutionID, record.Level, record.EventType, record.Message, record.Truncated)
		if execErr != nil {
			err = execErr
			return err
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		inserted += rows
	}
	if s.beforeInsertCommit != nil {
		s.beforeInsertCommit()
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if inserted > 0 {
		s.markCommitted()
	}
	return nil
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
	s.maintenanceMu.Lock()
	defer s.maintenanceMu.Unlock()
	if s.maintenanceEntered != nil {
		s.maintenanceEntered()
	}
	if err := s.beginOperation(); err != nil {
		return err
	}
	defer s.lifecycleMu.RUnlock()
	mutated := false
	defer func() {
		if mutated {
			s.markCommitted()
		}
	}()
	cutoff := s.now().Add(-s.retention).UnixNano()
	for {
		deleted, err := s.deleteBatch(ctx, `timestamp_ns < ?`, cutoff)
		if err != nil {
			return err
		}
		mutated = mutated || deleted > 0
		if deleted < maintenanceBatch {
			break
		}
	}
	for {
		if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA incremental_vacuum(%d)`, incrementalVacuumPages)); err != nil {
			return err
		}
		size, err := s.sizeBytes(s.path)
		if err != nil {
			return err
		}
		if size <= s.maxBytes {
			return nil
		}
		deleted, err := s.deleteBatchLimit(ctx, "1=1", capacityDeleteBatch)
		if err != nil {
			return err
		}
		if deleted == 0 {
			return fmt.Errorf("function log store remains over capacity: %d bytes exceeds %d bytes", size, s.maxBytes)
		}
		mutated = true
	}
}

// databaseUsageBytes measures live database pages rather than physical file
// length so reusable freelist pages do not cause unnecessary log deletion.
func (s *Store) databaseUsageBytes(string) (int64, error) {
	var pageSize, pageCount, freeList int64
	if err := s.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		return 0, err
	}
	if err := s.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount); err != nil {
		return 0, err
	}
	if err := s.db.QueryRow(`PRAGMA freelist_count`).Scan(&freeList); err != nil {
		return 0, err
	}
	return (pageCount - freeList) * pageSize, nil
}

func (s *Store) deleteBatch(ctx context.Context, predicate string, args ...any) (count int64, resultErr error) {
	return s.deleteBatchLimit(ctx, predicate, maintenanceBatch, args...)
}

func (s *Store) deleteBatchLimit(ctx context.Context, predicate string, limit int, args ...any) (count int64, resultErr error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if resultErr != nil {
			_ = tx.Rollback()
		}
	}()
	statement := fmt.Sprintf(`DELETE FROM function_logs WHERE rowid IN (SELECT rowid FROM function_logs WHERE %s ORDER BY timestamp_ns ASC,event_id ASC LIMIT %d)`, predicate, limit)
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
