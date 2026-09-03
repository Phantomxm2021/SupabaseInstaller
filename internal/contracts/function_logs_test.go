package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestParseEdgeRuntimeEventFixtures(t *testing.T) {
	tests := []struct {
		file, functionName, executionID, eventType, message, eventID string
		level                                                        FunctionLogLevel
		timestamp                                                    time.Time
	}{
		{"boot-event.json", "contract-log", "e8a9d201-c618-4051-be3c-721a97fee216", "Boot", "", "277b9cfd8b07e451a7d27656ca57416ae743d1d0c0c17b63c639b921a4844288", FunctionLogLevelInfo, time.Date(2026, 9, 3, 20, 51, 33, 319000000, time.UTC)},
		{"log-event.json", "contract-log", "e8a9d201-c618-4051-be3c-721a97fee216", "Log", "FUNCTION_LOG_FIXTURE_MESSAGE\n", "ed79c198aa6af2159f0b449644d4333a441ce8a2f8f0237fc8157c70e224bae6", FunctionLogLevelInfo, time.Date(2026, 9, 3, 20, 51, 33, 326000000, time.UTC)},
		{"uncaught-exception.json", "contract-throw", "f7cb1a48-200e-4586-9ec2-5baa4250af84", "UncaughtException", "event loop error: Error: FUNCTION_THROW_FIXTURE_MESSAGE\n    at file:///var/tmp/sb-compile-edge-runtime/app/examples/contract-throw/index.ts:3:11\n    at callback (ext:deno_web/02_timers.js:58:7)\n    at eventLoopTick (ext:core/01_core.js:210:13)", "00647ff3976a9d1e0f7ec41a6b44eae4095aa21c71c1dc86d43a2c7c60b738c1", FunctionLogLevelError, time.Date(2026, 9, 3, 20, 51, 33, 426000000, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "templates", "manager", "function-logs", "event-worker", "fixtures", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseEdgeRuntimeEvent(raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.Version != 1 || got.FunctionName != tt.functionName || got.ExecutionID != tt.executionID || got.EventType != tt.eventType || got.Message != tt.message || got.Level != tt.level || !got.Timestamp.Equal(tt.timestamp) {
				t.Fatalf("parsed event = %#v", got)
			}
			if len(got.EventID) != 64 || strings.ToLower(got.EventID) != got.EventID {
				t.Fatalf("EventID = %q", got.EventID)
			}
			canonical, err := CanonicalEdgeRuntimeEventJSON(raw)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(canonical)
			if canonicalHash := hex.EncodeToString(hash[:]); canonicalHash != tt.eventID || got.EventID != tt.eventID {
				t.Fatalf("canonical hash = %q, EventID = %q, want %q", canonicalHash, got.EventID, tt.eventID)
			}
			var compact bytes.Buffer
			if err := json.Compact(&compact, raw); err != nil {
				t.Fatal(err)
			}
			compactEvent, err := ParseEdgeRuntimeEvent(compact.Bytes())
			if err != nil || compactEvent.EventID != got.EventID {
				t.Fatalf("compact EventID = %q, %v; pretty EventID = %q", compactEvent.EventID, err, got.EventID)
			}
			if got.EventID == got.ExecutionID {
				t.Fatal("EventID must not reuse ExecutionID")
			}
		})
	}
}

func TestParseEdgeRuntimeEventRejectsIncompatiblePayloads(t *testing.T) {
	base := `{"timestamp":"2026-09-03T20:51:33.326Z","event_type":"Log","event":{"msg":"hello","level":"Info"},"metadata":{"service_path":"./examples/good-name","execution_id":"exec"}}`
	tests := map[string]string{
		"missing service path":         strings.Replace(base, `"service_path":"./examples/good-name",`, "", 1),
		"invalid function name":        strings.Replace(base, "./examples/good-name", "./examples/../main", 1),
		"missing execution identifier": strings.Replace(base, `"execution_id":"exec"`, `"execution_id":""`, 1),
		"unknown event type":           strings.Replace(base, `"event_type":"Log"`, `"event_type":"Shutdown"`, 1),
		"invalid event level":          strings.Replace(base, `"level":"Info"`, `"level":"Notice"`, 1),
		"absolute service path":        strings.Replace(base, "./examples/good-name", "/tmp/api", 1),
		"alternate prefix":             strings.Replace(base, "./examples/good-name", "./other/api", 1),
		"dot segments":                 strings.Replace(base, "./examples/good-name", "./examples/victim/../api", 1),
		"nested service path":          strings.Replace(base, "./examples/good-name", "./examples/nested/api", 1),
		"backslash service path":       strings.Replace(base, "./examples/good-name", `.\\examples\\api`, 1),
		"unknown top-level field":      strings.TrimSuffix(base, "}") + `,"extra":true}`,
		"duplicate top-level field":    strings.Replace(base, `"timestamp":`, `"timestamp":"2026-09-03T20:51:33.000Z","timestamp":`, 1),
		"unknown event field":          strings.Replace(base, `"level":"Info"`, `"level":"Info","extra":true`, 1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseEdgeRuntimeEvent([]byte(raw))
			if !errors.Is(err, ErrIncompatibleEdgeRuntimeEvent) {
				t.Fatalf("error = %v", err)
			}
			var incompatibility *EdgeRuntimeIncompatibilityError
			if !errors.As(err, &incompatibility) {
				t.Fatalf("error type = %T", err)
			}
		})
	}
}

func TestEdgeRuntimeCanonicalDomainMatchesJavaScript(t *testing.T) {
	base := `{"timestamp":"2026-09-03T20:51:33.326Z","event_type":"Log","event":{"msg":"line paragraph ","level":"%s"},"metadata":{"service_path":"./examples/good-name","execution_id":"exec","otel_attributes":{"edge_runtime.worker.kind":"user"}}}`
	for level, eventID := range map[string]string{
		"Warn":    "9f053d87d5ee7e902d32a426ef041c8d4d2a58279914038543256b58821f4dea",
		"Warning": "d12a29e976e73eca17fcbb45adc2aa6db63c6967bdfc88ffe77a04b152fcf458",
	} {
		event, err := ParseEdgeRuntimeEvent([]byte(fmt.Sprintf(base, level)))
		if err != nil || event.Level != FunctionLogLevelWarn || event.EventID != eventID {
			t.Errorf("level %s: event=%#v err=%v", level, event, err)
		}
	}

	rejected := []string{
		`{"timestamp":"2026-09-03T20:51:33.326Z","event_type":"Boot","event":{"boot_time":1.5},"metadata":{"service_path":"./examples/good-name","execution_id":"exec","otel_attributes":{}}}`,
		`{"timestamp":"2026-09-03T20:51:33.326Z","event_type":"Boot","event":{"boot_time":9007199254740992},"metadata":{"service_path":"./examples/good-name","execution_id":"exec","otel_attributes":{}}}`,
		`{"timestamp":"2026-09-03T20:51:33.326Z","event_type":"UncaughtException","event":{"exception":"x","cpu_time_used":9007199254740992},"metadata":{"service_path":"./examples/good-name","execution_id":"exec","otel_attributes":{}}}`,
		`{"timestamp":"2026-09-03T20:51:33.326Z","event_type":"Log","event":{"msg":"x","level":"Info"},"metadata":{"service_path":"./examples/good-name","execution_id":"exec","otel_attributes":{"é":"x"}}}`,
		`{"timestamp":"2026-09-03T20:51:33.326Z","event_type":"Log","event":{"msg":"x","level":"Info"},"metadata":{"service_path":"./examples/good-name","execution_id":"exec","otel_attributes":{"valid.key":1}}}`,
	}
	for _, raw := range rejected {
		if _, err := ParseEdgeRuntimeEvent([]byte(raw)); !errors.Is(err, ErrIncompatibleEdgeRuntimeEvent) {
			t.Errorf("ParseEdgeRuntimeEvent(%s) error = %v", raw, err)
		}
	}
}

func TestValidateFunctionLogQuery(t *testing.T) {
	for _, query := range []FunctionLogQuery{{Limit: 1}, {Limit: 200}, {Limit: 10, Level: "debug"}, {Limit: 10, Level: "info"}, {Limit: 10, Level: "warn"}, {Limit: 10, Level: "error"}, {Limit: 10, Search: strings.Repeat("界", 85)}} {
		if err := ValidateFunctionLogQuery(query); err != nil {
			t.Errorf("valid query %#v: %v", query, err)
		}
	}
	for name, query := range map[string]FunctionLogQuery{
		"limit zero": {}, "limit too large": {Limit: 201}, "both cursors": {Limit: 10, Before: "a", After: "b"},
		"search too long": {Limit: 10, Search: strings.Repeat("界", 86)}, "invalid level": {Limit: 10, Level: "notice"},
		"invalid utf8":            {Limit: 10, Search: string([]byte{0xff})},
		"malformed before cursor": {Limit: 10, Before: "bad"},
		"malformed after cursor":  {Limit: 10, After: "bad"},
		"invalid cursor payload":  {Limit: 10, Before: base64.RawURLEncoding.EncodeToString([]byte(`{}`))},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateFunctionLogQuery(query); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if utf8.RuneCountInString(strings.Repeat("界", 86)) <= 256 { /* prove the test is byte-based */
	} else {
		t.Fatal("test input should have fewer than 256 runes")
	}
}

func TestFunctionLogCursorRoundTrip(t *testing.T) {
	want := FunctionLogCursor{Timestamp: time.Date(2026, 9, 3, 20, 51, 33, 123456789, time.UTC), ID: "record-7"}
	encoded, err := EncodeFunctionLogCursor(want)
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(encoded, "+/=") {
		t.Fatalf("cursor is not raw base64url: %q", encoded)
	}
	got, err := DecodeFunctionLogCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.Timestamp.Equal(want.Timestamp) {
		t.Fatalf("decoded = %#v, want %#v", got, want)
	}
}

func TestFunctionLogCursorRejectsMalformedValues(t *testing.T) {
	validJSON := `{"timestamp":"2026-09-03T20:51:33Z","id":"x"}`
	for _, cursor := range []string{
		"",
		"not base64!",
		"e30",
		"eyJ0aW1lc3RhbXAiOiIyMDI2LTA5LTAzVDIwOjUxOjMzWiIsImlkIjoieCIsImV4dHJhIjp0cnVlfQ",
		base64.RawURLEncoding.EncodeToString([]byte(validJSON + `{`)),
		base64.RawURLEncoding.EncodeToString([]byte(validJSON + `{}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"timestamp":"2026-09-03T20:51:33Z","timestamp":"2026-09-04T20:51:33Z","id":"x"}`)),
		base64.RawURLEncoding.EncodeToString([]byte(`{"timestamp":"2026-09-03T20:51:33Z","id":"x","id":"y"}`)),
	} {
		if _, err := DecodeFunctionLogCursor(cursor); err == nil {
			t.Errorf("DecodeFunctionLogCursor(%q) succeeded", cursor)
		}
	}
}

func TestFunctionLogJSONFieldNames(t *testing.T) {
	record := FunctionLogRecord{ID: "id", ProjectID: "project", FunctionName: "fn", ExecutionID: "exec", EventType: "Log", Message: "m", Timestamp: time.Unix(1, 0).UTC(), IngestedAt: time.Unix(2, 0).UTC(), Level: FunctionLogLevelInfo, Truncated: true}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"id"`, `"projectId"`, `"functionName"`, `"executionId"`, `"eventType"`, `"message"`, `"timestamp"`, `"ingestedAt"`, `"level"`, `"truncated"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("JSON %s missing %s", raw, key)
		}
	}
}
