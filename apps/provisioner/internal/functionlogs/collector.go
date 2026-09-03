package functionlogs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"supabase-manager/internal/contracts"
)

const (
	maxCollectorBody       = 1 << 20
	defaultMaintenanceTime = time.Hour
	maintenanceTimeout     = 5 * time.Minute
)

var projectIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type CollectorStore interface {
	InsertBatch(context.Context, []contracts.FunctionLogRecord) error
	// Maintain must return promptly when its context is cancelled.
	Maintain(context.Context) error
}

type CollectorOptions struct {
	ProjectID           string
	Store               CollectorStore
	Redactor            *Redactor
	ProjectEnvPath      string
	FunctionsEnvPath    string
	FunctionsRoot       string
	Logger              *slog.Logger
	Now                 func() time.Time
	MaintenanceInterval time.Duration
	HealthPath          string
}

type Collector struct {
	projectID      string
	store          CollectorStore
	redactor       *Redactor
	configuredRoot string
	canonicalRoot  string
	rootIdentity   os.FileInfo
	logger         *slog.Logger
	now            func() time.Time
	healthPath     string

	healthMu sync.RWMutex
	health   contracts.FunctionLogHealth
	stop     chan struct{}
	done     chan struct{}
	close    sync.Once
}

func NewCollector(options CollectorOptions) (*Collector, error) {
	if !projectIDPattern.MatchString(options.ProjectID) {
		return nil, errors.New("invalid function log project ID")
	}
	if options.Store == nil {
		return nil, errors.New("function log store is required")
	}
	configuredRoot, canonicalRoot, rootIdentity, err := validateFunctionsRoot(options.FunctionsRoot)
	if err != nil {
		return nil, err
	}
	redactor := options.Redactor
	if redactor == nil {
		redactor, err = LoadRedactor(options.ProjectEnvPath, options.FunctionsEnvPath)
		if err != nil {
			return nil, errors.New("load function log redactor")
		}
	}
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.MaintenanceInterval <= 0 {
		options.MaintenanceInterval = defaultMaintenanceTime
	}
	c := &Collector{
		projectID: options.ProjectID, store: options.Store, redactor: redactor, configuredRoot: configuredRoot, canonicalRoot: canonicalRoot, rootIdentity: rootIdentity,
		logger: options.Logger, now: options.Now, healthPath: options.HealthPath, health: contracts.FunctionLogHealth{Status: "healthy"},
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	if options.HealthPath != "" {
		previous, readErr := ReadHealthSnapshot(options.HealthPath, options.Now())
		if readErr != nil {
			return nil, errors.New("read function log health")
		}
		if previous.Status != "offline" && !(previous.Status == "incompatible" && previous.Dropped == 0 && previous.Rejected == 0) {
			c.health.Dropped, c.health.Rejected = previous.Dropped, previous.Rejected
			if previous.Status == "incompatible" {
				c.health.Status = "incompatible"
			} else if previous.Dropped > 0 {
				c.health.Status = "dropped"
			}
		}
		if err := WriteHealthSnapshot(c.healthPath, c.health, c.now()); err != nil {
			return nil, errors.New("write function log health")
		}
	}
	go c.maintain(options.MaintenanceInterval)
	return c, nil
}

func (c *Collector) persistHealth() error {
	err := WriteHealthSnapshot(c.healthPath, c.Health(), c.now())
	if err != nil {
		c.healthMu.Lock()
		c.health.Status = "storage_error"
		c.health.Detail = "function log health persistence unavailable"
		c.healthMu.Unlock()
		c.logger.Error("function log health persistence failed")
	}
	return err
}

func validateFunctionsRoot(path string) (string, string, os.FileInfo, error) {
	if path == "" {
		return "", "", nil, errors.New("functions root is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", nil, errors.New("invalid functions root")
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "", nil, errors.New("functions root must be a real directory")
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", nil, errors.New("resolve functions root")
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", "", nil, errors.New("resolve functions root")
	}
	return filepath.Clean(abs), filepath.Clean(canonical), info, nil
}

func (c *Collector) Health() contracts.FunctionLogHealth {
	c.healthMu.RLock()
	defer c.healthMu.RUnlock()
	return c.health
}

func (c *Collector) reject(count uint64, incompatible bool) {
	if count == 0 {
		count = 1
	}
	c.healthMu.Lock()
	c.health.Rejected = saturatingAdd(c.health.Rejected, count)
	if incompatible {
		c.health.Status = "incompatible"
		c.health.Detail = "unsupported event contract version"
	}
	c.healthMu.Unlock()
	_ = c.persistHealth()
}

func (c *Collector) storageFailure(count uint64, maintenance bool) {
	c.healthMu.Lock()
	if !maintenance {
		c.health.Dropped = saturatingAdd(c.health.Dropped, count)
		if c.health.Status != "incompatible" && c.health.Status != "storage_error" {
			c.health.Status = "dropped"
		}
	} else {
		c.health.Status = "storage_error"
	}
	c.health.Detail = "function log storage unavailable"
	c.healthMu.Unlock()
	_ = c.persistHealth()
}

func (c *Collector) functionExists(name string) bool {
	if contracts.ValidateFunctionName(name) != nil {
		return false
	}
	rootInfo, err := os.Lstat(c.configuredRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(c.rootIdentity, rootInfo) {
		return false
	}
	resolvedRoot, err := filepath.EvalSymlinks(c.configuredRoot)
	if err != nil {
		return false
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil || filepath.Clean(resolvedRoot) != c.canonicalRoot {
		return false
	}
	candidate := filepath.Join(c.configuredRoot, name)
	info, err := os.Lstat(candidate)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	// This establishes membership at validation time only. The collector passes no
	// filesystem path onward and never opens or reads the candidate after this check.
	resolvedCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return false
	}
	resolvedCandidate, err = filepath.Abs(resolvedCandidate)
	return err == nil && filepath.Dir(filepath.Clean(resolvedCandidate)) == c.canonicalRoot
}

func (c *Collector) maintain(interval time.Duration) {
	defer close(c.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), maintenanceTimeout)
			err := c.store.Maintain(ctx)
			cancel()
			if err != nil {
				c.storageFailure(1, true)
				c.logger.Error("function log maintenance failed")
			}
		case <-heartbeat.C:
			_ = c.persistHealth()
		case <-c.stop:
			return
		}
	}
}

func (c *Collector) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.CloseContext(ctx)
}

func (c *Collector) CloseContext(ctx context.Context) error {
	c.close.Do(func() { close(c.stop) })
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func NewCollectorHandler(collector *Collector) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/v1/events", collector.ingest)
	mux.HandleFunc("GET /health/live", func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]string{"status": "live"})
	})
	return mux
}

func (c *Collector) ingest(response http.ResponseWriter, request *http.Request) {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.reject(1, false)
		http.Error(response, "invalid request", http.StatusBadRequest)
		return
	}
	request.Body = http.MaxBytesReader(response, request.Body, maxCollectorBody)
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		c.reject(1, false)
		if errors.As(err, &maxErr) {
			http.Error(response, "request too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(response, "invalid request", http.StatusBadRequest)
		}
		return
	}
	if err = rejectDuplicateJSONKeys(raw); err != nil {
		c.reject(1, false)
		http.Error(response, "invalid request", http.StatusBadRequest)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var batch contracts.FunctionLogBatch
	if err = decoder.Decode(&batch); err != nil {
		var maxErr *http.MaxBytesError
		c.reject(1, false)
		if errors.As(err, &maxErr) {
			http.Error(response, "request too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(response, "invalid request", http.StatusBadRequest)
		}
		return
	}
	if err = ensureJSONEOF(decoder); err != nil {
		c.reject(eventCount(batch.Events), false)
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(response, "request too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(response, "invalid request", http.StatusBadRequest)
		}
		return
	}
	count := eventCount(batch.Events)
	if batch.Version != 1 {
		c.reject(count, true)
		http.Error(response, "unsupported contract version", http.StatusUnprocessableEntity)
		return
	}
	if len(batch.Events) > 100 {
		c.reject(count, false)
		http.Error(response, "batch too large", http.StatusRequestEntityTooLarge)
		return
	}
	if batch.ProjectID != c.projectID || len(batch.Events) < 1 {
		c.reject(count, false)
		http.Error(response, "invalid batch", http.StatusBadRequest)
		return
	}
	records := make([]contracts.FunctionLogRecord, len(batch.Events))
	for index, event := range batch.Events {
		if event.Version != 1 {
			c.reject(count, true)
			http.Error(response, "unsupported contract version", http.StatusUnprocessableEntity)
			return
		}
		record := contracts.FunctionLogRecord{ID: event.EventID, ProjectID: c.projectID, FunctionName: event.FunctionName, ExecutionID: event.ExecutionID, EventType: event.EventType, Message: event.Message, Timestamp: event.Timestamp, IngestedAt: c.now().UTC(), Level: event.Level}
		if validateRecord(record) != nil || !c.functionExists(event.FunctionName) {
			c.reject(count, false)
			http.Error(response, "invalid event", http.StatusBadRequest)
			return
		}
		var sanitizedTruncated bool
		record.Message, sanitizedTruncated = c.redactor.SanitizeMessage(record.Message)
		record.Truncated = record.Truncated || sanitizedTruncated
		records[index] = record
	}
	if err = c.store.InsertBatch(request.Context(), records); err != nil {
		c.storageFailure(uint64(len(records)), false)
		c.logger.Error("function log batch storage failed")
		http.Error(response, "storage unavailable", http.StatusServiceUnavailable)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid JSON object key")
			}
			if _, exists := seen[key]; exists {
				return errors.New("duplicate JSON object key")
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	closingDelim, ok := closing.(json.Delim)
	if !ok || (delim == '{' && closingDelim != '}') || (delim == '[' && closingDelim != ']') {
		return errors.New("invalid JSON closing delimiter")
	}
	return nil
}

func eventCount(events []contracts.EdgeRuntimeEvent) uint64 {
	if len(events) == 0 {
		return 1
	}
	return uint64(len(events))
}

func saturatingAdd(value, increment uint64) uint64 {
	if increment > math.MaxUint64-value {
		return math.MaxUint64
	}
	return value + increment
}
