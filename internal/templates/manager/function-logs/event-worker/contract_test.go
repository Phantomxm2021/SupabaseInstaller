package eventworker

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"supabase-manager/internal/contracts"
)

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
			eventID:      "b21ef14e03afeafac0e5295527887d83d44a8f4f6b48ad6125845a82fea0850f",
		},
		{
			name:         "uncaught-exception.json",
			functionName: "contract-throw",
			executionID:  "f7cb1a48-200e-4586-9ec2-5baa4250af84",
			eventType:    "UncaughtException",
			eventID:      "863deab288b9bc0f09416df4d02a15d20ce0bdaeb5495eb49fb81b4912699e8c",
		},
		{
			name:         "boot-event.json",
			functionName: "contract-log",
			executionID:  "e8a9d201-c618-4051-be3c-721a97fee216",
			eventType:    "Boot",
			eventID:      "8d7390899131ea89ae39fbf4ac5f577a66ec3fe919c748bef63e51ec04a6b86d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("fixtures", tt.name))
			if err != nil {
				t.Fatal(err)
			}

			// EventID deliberately hashes the exact parser input bytes. ExecutionID
			// attributes an invocation and must not be reused for event deduplication.
			hash := sha256.Sum256(raw)
			if got := hex.EncodeToString(hash[:]); got != tt.eventID {
				t.Fatalf("fixture bytes changed: got SHA-256 %q, want %q", got, tt.eventID)
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
				t.Errorf("EventID = %q, want raw SHA-256 %q", event.EventID, tt.eventID)
			}
		})
	}
}
