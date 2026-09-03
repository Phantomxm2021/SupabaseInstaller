package functionlogs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"supabase-manager/internal/contracts"
)

type collectorStore struct {
	mu          sync.Mutex
	records     []contracts.FunctionLogRecord
	insertErr   error
	maintainErr error
	maintained  chan struct{}
}

func (s *collectorStore) InsertBatch(_ context.Context, records []contracts.FunctionLogRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, records...)
	return s.insertErr
}

func (s *collectorStore) Maintain(context.Context) error {
	if s.maintained != nil {
		select {
		case s.maintained <- struct{}{}:
		default:
		}
	}
	return s.maintainErr
}

func newCollectorTest(t *testing.T, store *collectorStore, logOutput *bytes.Buffer, interval time.Duration) (*Collector, http.Handler) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "hello"), 0o700); err != nil {
		t.Fatal(err)
	}
	collector, err := NewCollector(CollectorOptions{
		ProjectID: "project", Store: store, Redactor: &Redactor{}, FunctionsRoot: root,
		Logger: slog.New(slog.NewTextHandler(logOutput, nil)), Now: func() time.Time { return time.Unix(200, 0).UTC() },
		MaintenanceInterval: interval,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = collector.Close() })
	return collector, NewCollectorHandler(collector)
}

func validBatch() contracts.FunctionLogBatch {
	return contracts.FunctionLogBatch{Version: 1, ProjectID: "project", Events: []contracts.EdgeRuntimeEvent{{
		Version: 1, EventID: "event-1", FunctionName: "hello", ExecutionID: "exec-1", EventType: "Log",
		Message: "hello", Timestamp: time.Unix(100, 0).UTC(), Level: contracts.FunctionLogLevelInfo,
	}}}
}

func requestBatch(t *testing.T, handler http.Handler, batch any, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/events", bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestCollectorAcceptsValidAndDuplicateBatches(t *testing.T) {
	store, logs := &collectorStore{}, &bytes.Buffer{}
	collector, handler := newCollectorTest(t, store, logs, time.Hour)
	for range 2 {
		if got := requestBatch(t, handler, validBatch(), "application/json").Code; got != http.StatusNoContent {
			t.Fatalf("status = %d", got)
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != 2 || store.records[0].IngestedAt != time.Unix(200, 0).UTC() {
		t.Fatalf("records = %+v", store.records)
	}
	if health := collector.Health(); health.Status != "healthy" || health.Dropped != 0 || health.Rejected != 0 {
		t.Fatalf("health = %+v", health)
	}
}

func TestCollectorRejectsBadRequestsAndCountsEvents(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*contracts.FunctionLogBatch)
		contentType string
		want        int
		rejected    uint64
		status      string
	}{
		{"content type", func(*contracts.FunctionLogBatch) {}, "text/plain", 400, 1, "healthy"},
		{"batch version", func(b *contracts.FunctionLogBatch) { b.Version = 2 }, "application/json", 422, 1, "incompatible"},
		{"project", func(b *contracts.FunctionLogBatch) { b.ProjectID = "other" }, "application/json", 400, 1, "healthy"},
		{"no events", func(b *contracts.FunctionLogBatch) { b.Events = nil }, "application/json", 400, 1, "healthy"},
		{"event version", func(b *contracts.FunctionLogBatch) { b.Events[0].Version = 2 }, "application/json", 422, 1, "incompatible"},
		{"event id", func(b *contracts.FunctionLogBatch) { b.Events[0].EventID = "" }, "application/json", 400, 1, "healthy"},
		{"execution id", func(b *contracts.FunctionLogBatch) { b.Events[0].ExecutionID = "" }, "application/json", 400, 1, "healthy"},
		{"timestamp", func(b *contracts.FunctionLogBatch) { b.Events[0].Timestamp = time.Time{} }, "application/json", 400, 1, "healthy"},
		{"level", func(b *contracts.FunctionLogBatch) { b.Events[0].Level = "fatal" }, "application/json", 400, 1, "healthy"},
		{"event type", func(b *contracts.FunctionLogBatch) { b.Events[0].EventType = "Exit" }, "application/json", 400, 1, "healthy"},
		{"invalid function", func(b *contracts.FunctionLogBatch) { b.Events[0].FunctionName = "../hello" }, "application/json", 400, 1, "healthy"},
		{"unknown function", func(b *contracts.FunctionLogBatch) { b.Events[0].FunctionName = "missing" }, "application/json", 400, 1, "healthy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collector, handler := newCollectorTest(t, &collectorStore{}, &bytes.Buffer{}, time.Hour)
			batch := validBatch()
			test.mutate(&batch)
			if got := requestBatch(t, handler, batch, test.contentType).Code; got != test.want {
				t.Fatalf("status = %d, want %d", got, test.want)
			}
			health := collector.Health()
			if health.Rejected != test.rejected || health.Status != test.status {
				t.Fatalf("health = %+v", health)
			}
		})
	}
}

func TestCollectorRejectsMalformedUnknownTrailingOversizedAndTooMany(t *testing.T) {
	collector, handler := newCollectorTest(t, &collectorStore{}, &bytes.Buffer{}, time.Hour)
	for _, body := range []string{"{", `{"version":1,"projectId":"project","events":[],"extra":true}`, `{"version":1,"projectId":"project","events":[]} {}`} {
		req := httptest.NewRequest(http.MethodPost, "/internal/v1/events", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != 400 {
			t.Fatalf("body %q status = %d", body, response.Code)
		}
	}
	tooMany := validBatch()
	tooMany.Events = make([]contracts.EdgeRuntimeEvent, 101)
	if got := requestBatch(t, handler, tooMany, "application/json").Code; got != http.StatusRequestEntityTooLarge {
		t.Fatalf("too many status = %d", got)
	}
	big := `{"version":1,"projectId":"project","events":[],"padding":"` + strings.Repeat("x", 1<<20) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/events", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status = %d", response.Code)
	}
	if collector.Health().Rejected != 105 {
		t.Fatalf("health = %+v", collector.Health())
	}
}

func TestCollectorReturnsPayloadTooLargeForOversizedTrailingWhitespace(t *testing.T) {
	_, handler := newCollectorTest(t, &collectorStore{}, &bytes.Buffer{}, time.Hour)
	body, err := json.Marshal(validBatch())
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, bytes.Repeat([]byte(" "), maxCollectorBody+1-len(body))...)
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestCollectorRejectsDuplicateSemanticJSONKeys(t *testing.T) {
	event := `{"version":1,"eventId":"event-1","functionName":"hello","executionId":"exec-1","eventType":"Log","message":"hello","timestamp":"1970-01-01T00:01:40Z","level":"info"}`
	tests := map[string]string{
		"batch version":       `{"version":1,"version":1,"projectId":"project","events":[` + event + `]}`,
		"batch project":       `{"version":1,"projectId":"project","projectId":"project","events":[` + event + `]}`,
		"event function name": `{"version":1,"projectId":"project","events":[{"version":1,"eventId":"event-1","functionName":"hello","functionName":"hello","executionId":"exec-1","eventType":"Log","message":"hello","timestamp":"1970-01-01T00:01:40Z","level":"info"}]}`,
		"event ID":            `{"version":1,"projectId":"project","events":[{"version":1,"eventId":"event-1","eventId":"event-1","functionName":"hello","executionId":"exec-1","eventType":"Log","message":"hello","timestamp":"1970-01-01T00:01:40Z","level":"info"}]}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			store := &collectorStore{}
			_, handler := newCollectorTest(t, store, &bytes.Buffer{}, time.Hour)
			req := httptest.NewRequest(http.MethodPost, "/internal/v1/events", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			store.mu.Lock()
			defer store.mu.Unlock()
			if len(store.records) != 0 {
				t.Fatalf("stored records = %+v", store.records)
			}
		})
	}
}

func TestCollectorRedactsBeforeCallingStore(t *testing.T) {
	for _, insertErr := range []error{nil, errors.New("store unavailable")} {
		store := &collectorStore{insertErr: insertErr}
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "hello"), 0o700); err != nil {
			t.Fatal(err)
		}
		collector, err := NewCollector(CollectorOptions{
			ProjectID: "project", Store: store, Redactor: &Redactor{known: []string{"private-sentinel"}},
			FunctionsRoot: root, Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		})
		if err != nil {
			t.Fatal(err)
		}
		batch := validBatch()
		batch.Events[0].Message = "token private-sentinel"
		requestBatch(t, NewCollectorHandler(collector), batch, "application/json")
		_ = collector.Close()
		store.mu.Lock()
		if len(store.records) != 1 || strings.Contains(store.records[0].Message, "private-sentinel") || !strings.Contains(store.records[0].Message, "[REDACTED]") {
			t.Fatalf("records = %+v", store.records)
		}
		store.mu.Unlock()
	}
}

func TestCollectorHealthCountersSaturate(t *testing.T) {
	collector, _ := newCollectorTest(t, &collectorStore{}, &bytes.Buffer{}, time.Hour)
	collector.healthMu.Lock()
	collector.health.Rejected = math.MaxUint64 - 1
	collector.health.Dropped = math.MaxUint64 - 1
	collector.healthMu.Unlock()
	collector.reject(10, false)
	collector.storageFailure(10, false)
	collector.storageFailure(1, true)
	health := collector.Health()
	if health.Rejected != math.MaxUint64 || health.Dropped != math.MaxUint64 {
		t.Fatalf("health = %+v", health)
	}
}

func TestCollectorStorageFailureIsSafeAndLive(t *testing.T) {
	secret := "collector-secret"
	store, output := &collectorStore{insertErr: errors.New("/private/secret/path " + secret)}, &bytes.Buffer{}
	collector, handler := newCollectorTest(t, store, output, time.Hour)
	batch := validBatch()
	batch.Events[0].Message = "payload " + secret
	if got := requestBatch(t, handler, batch, "application/json").Code; got != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", got)
	}
	health := collector.Health()
	if health.Status != "dropped" || health.Dropped != 1 || strings.Contains(health.Detail, secret) || strings.Contains(health.Detail, "/private") {
		t.Fatalf("health = %+v", health)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if response.Code != 200 {
		t.Fatalf("live status = %d", response.Code)
	}
	logged := output.String()
	if strings.Contains(logged, secret) || strings.Contains(logged, batch.Events[0].Message) || strings.Contains(logged, "event-1") {
		t.Fatalf("unsafe logs: %s", logged)
	}
}

func TestCollectorConstructionRejectsUnsafeFunctionsRoots(t *testing.T) {
	if _, err := NewCollector(CollectorOptions{ProjectID: "", Store: &collectorStore{}, Redactor: &Redactor{}, FunctionsRoot: t.TempDir()}); err == nil {
		t.Fatal("expected project validation")
	}
	root := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	collector, err := NewCollector(CollectorOptions{ProjectID: "project", Store: &collectorStore{}, Redactor: &Redactor{}, FunctionsRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	batch := validBatch()
	batch.Events[0].FunctionName = "linked"
	if got := requestBatch(t, NewCollectorHandler(collector), batch, "application/json").Code; got != 400 {
		t.Fatalf("symlink status = %d", got)
	}
}

func TestCollectorRejectsReplacedFunctionsRoot(t *testing.T) {
	configuredRoot := filepath.Join(t.TempDir(), "functions")
	if err := os.Mkdir(configuredRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(configuredRoot, "hello"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := &collectorStore{}
	collector, err := NewCollector(CollectorOptions{ProjectID: "project", Store: store, Redactor: &Redactor{}, FunctionsRoot: configuredRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	originalRoot := configuredRoot + "-original"
	if err := os.Rename(configuredRoot, originalRoot); err != nil {
		t.Fatal(err)
	}
	externalRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(externalRoot, "hello"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalRoot, configuredRoot); err != nil {
		t.Fatal(err)
	}
	if got := requestBatch(t, NewCollectorHandler(collector), validBatch(), "application/json").Code; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != 0 {
		t.Fatalf("stored records = %+v", store.records)
	}
}

func TestCollectorRejectsOrdinaryDirectoryReplacingFunctionsRoot(t *testing.T) {
	configuredRoot := filepath.Join(t.TempDir(), "functions")
	if err := os.Mkdir(configuredRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(configuredRoot, "hello"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := &collectorStore{}
	collector, err := NewCollector(CollectorOptions{ProjectID: "project", Store: store, Redactor: &Redactor{}, FunctionsRoot: configuredRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	if err := os.Rename(configuredRoot, configuredRoot+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(configuredRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(configuredRoot, "hello"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := requestBatch(t, NewCollectorHandler(collector), validBatch(), "application/json").Code; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", got, http.StatusBadRequest)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.records) != 0 {
		t.Fatalf("stored records = %+v", store.records)
	}
}

func TestCollectorRunsAndStopsPeriodicMaintenance(t *testing.T) {
	maintained := make(chan struct{}, 2)
	store := &collectorStore{maintained: maintained}
	collector, _ := newCollectorTest(t, store, &bytes.Buffer{}, 5*time.Millisecond)
	select {
	case <-maintained:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not run")
	}
	if err := collector.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-maintained:
		t.Fatal("maintenance ran after close")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestCollectorMaintenanceFailureDoesNotCountDroppedEvents(t *testing.T) {
	maintained := make(chan struct{}, 1)
	store := &collectorStore{maintainErr: errors.New("maintenance failed"), maintained: maintained}
	collector, _ := newCollectorTest(t, store, &bytes.Buffer{}, 5*time.Millisecond)
	collector.healthMu.Lock()
	collector.health.Dropped = 7
	collector.healthMu.Unlock()
	select {
	case <-maintained:
	case <-time.After(time.Second):
		t.Fatal("maintenance did not run")
	}
	deadline := time.Now().Add(time.Second)
	for collector.Health().Status != "storage_error" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	health := collector.Health()
	if health.Status != "storage_error" || health.Detail != "function log storage unavailable" || health.Dropped != 7 {
		t.Fatalf("health = %+v", health)
	}
}
