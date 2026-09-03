package functionlogs

import (
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
}

type Collector struct {
	projectID     string
	store         CollectorStore
	redactor      *Redactor
	functionsRoot string
	logger        *slog.Logger
	now           func() time.Time

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
	root, err := validateFunctionsRoot(options.FunctionsRoot)
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
		projectID: options.ProjectID, store: options.Store, redactor: redactor, functionsRoot: root,
		logger: options.Logger, now: options.Now, health: contracts.FunctionLogHealth{Status: "healthy"},
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	go c.maintain(options.MaintenanceInterval)
	return c, nil
}

func validateFunctionsRoot(path string) (string, error) {
	if path == "" {
		return "", errors.New("functions root is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("invalid functions root")
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("functions root must be a real directory")
	}
	return filepath.Clean(abs), nil
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
	defer c.healthMu.Unlock()
	c.health.Rejected = saturatingAdd(c.health.Rejected, count)
	if incompatible {
		c.health.Status = "incompatible"
		c.health.Detail = "unsupported event contract version"
	}
}

func (c *Collector) storageFailure(count uint64, maintenance bool) {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()
	if maintenance {
		count = 1
	}
	c.health.Dropped = saturatingAdd(c.health.Dropped, count)
	c.health.Status = "storage_error"
	c.health.Detail = "function log storage unavailable"
}

func (c *Collector) functionExists(name string) bool {
	if contracts.ValidateFunctionName(name) != nil {
		return false
	}
	info, err := os.Lstat(filepath.Join(c.functionsRoot, name))
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func (c *Collector) maintain(interval time.Duration) {
	defer close(c.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
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
		case <-c.stop:
			return
		}
	}
}

func (c *Collector) Close() error {
	c.close.Do(func() { close(c.stop) })
	<-c.done
	return nil
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
	decoder := json.NewDecoder(request.Body)
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
	if batch.ProjectID != c.projectID || len(batch.Events) < 1 || len(batch.Events) > 100 {
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
