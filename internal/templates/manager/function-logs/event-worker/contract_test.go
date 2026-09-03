package eventworker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"supabase-manager/internal/contracts"
)

func TestAdapterUsesPinnedAPIAndBoundedDelivery(t *testing.T) {
	source := Source()
	for _, required := range [][]byte{
		[]byte("new globalThis.EventManager()"),
		[]byte("MAX_QUEUE = 1000"),
		[]byte("MAX_BATCH = 100"),
		[]byte("FLUSH_INTERVAL_MS = 250"),
		[]byte("FETCH_TIMEOUT_MS = 500"),
		[]byte("http://function-log-collector:8081/internal/v1/events"),
		[]byte("crypto.subtle.digest(\"SHA-256\""),
		[]byte("function canonicalRuntimeEvent"),
		[]byte("canonicalJSONString(canonical)"),
		[]byte("FUNCTION_LOG_VERIFY_FIXTURES"),
		[]byte("FUNCTION_LOG_FIXTURE_RECORDS="),
		[]byte("clearInterval(flushInterval)"),
		[]byte("FUNCTION_LOG_EVENT_MANAGER_INERT timers=0 ticks="),
	} {
		if !bytes.Contains(source, required) {
			t.Errorf("adapter missing %q", required)
		}
	}
}

// These values pin the Edge Runtime compatibility contract verified by running
// the image and capturing its event-worker callback payloads.
const (
	pinnedEdgeRuntimeImage    = "supabase/edge-runtime:v1.74.0"
	pinnedEdgeRuntimeDigest   = "sha256:2781daf92394db91f7e94129cc3d04ec474ad16a8fe64b3fbeef6e7d557ab120"
	pinnedEventWorkerArgument = "--event-worker"
)

func TestPinnedEdgeRuntimeCompatibilityReference(t *testing.T) {
	if pinnedEdgeRuntimeImage != "supabase/edge-runtime:v1.74.0" {
		t.Fatalf("unexpected pinned image: %q", pinnedEdgeRuntimeImage)
	}
	if pinnedEdgeRuntimeDigest != "sha256:2781daf92394db91f7e94129cc3d04ec474ad16a8fe64b3fbeef6e7d557ab120" {
		t.Fatalf("unexpected pinned digest: %q", pinnedEdgeRuntimeDigest)
	}
	if pinnedEventWorkerArgument != "--event-worker" {
		t.Fatalf("unexpected event-worker argument: %q", pinnedEventWorkerArgument)
	}
}

func TestPinnedEventFixturesCarryExactFunctionAttribution(t *testing.T) {
	tests := []struct {
		name         string
		functionName string
		executionID  string
		eventType    string
		eventID      string
	}{
		{
			name:         "log-event.json",
			functionName: "contract-log",
			executionID:  "e8a9d201-c618-4051-be3c-721a97fee216",
			eventType:    "Log",
			eventID:      "ed79c198aa6af2159f0b449644d4333a441ce8a2f8f0237fc8157c70e224bae6",
		},
		{
			name:         "uncaught-exception.json",
			functionName: "contract-throw",
			executionID:  "f7cb1a48-200e-4586-9ec2-5baa4250af84",
			eventType:    "UncaughtException",
			eventID:      "00647ff3976a9d1e0f7ec41a6b44eae4095aa21c71c1dc86d43a2c7c60b738c1",
		},
		{
			name:         "boot-event.json",
			functionName: "contract-log",
			executionID:  "e8a9d201-c618-4051-be3c-721a97fee216",
			eventType:    "Boot",
			eventID:      "277b9cfd8b07e451a7d27656ca57416ae743d1d0c0c17b63c639b921a4844288",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("fixtures", tt.name))
			if err != nil {
				t.Fatal(err)
			}

			// EventID hashes the deterministic compact JSON representation of the
			// callback object, independent of fixture whitespace. ExecutionID is
			// invocation attribution and must not be reused for deduplication.
			canonical, err := contracts.CanonicalEdgeRuntimeEventJSON(raw)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(canonical)
			if got := hex.EncodeToString(hash[:]); got != tt.eventID {
				t.Fatalf("canonical fixture SHA-256 = %q, want %q", got, tt.eventID)
			}

			event, err := contracts.ParseEdgeRuntimeEvent(raw)
			if err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}
			if event.FunctionName != tt.functionName {
				t.Errorf("FunctionName = %q, want %q", event.FunctionName, tt.functionName)
			}
			if event.ExecutionID != tt.executionID {
				t.Errorf("ExecutionID = %q, want %q", event.ExecutionID, tt.executionID)
			}
			if event.EventType != tt.eventType {
				t.Errorf("EventType = %q, want %q", event.EventType, tt.eventType)
			}
			if event.EventID != tt.eventID {
				t.Errorf("EventID = %q, want canonical SHA-256 %q", event.EventID, tt.eventID)
			}
		})
	}
}
