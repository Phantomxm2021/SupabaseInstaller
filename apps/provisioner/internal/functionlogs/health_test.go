package functionlogs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"supabase-manager/internal/contracts"
)

func TestHealthSnapshotRoundTripAndStaleness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.json")
	now := time.Now().UTC()
	if err := WriteHealthSnapshot(path, contracts.FunctionLogHealth{Status: "healthy", Dropped: 3, Detail: "/secret/path"}, now); err != nil {
		t.Fatal(err)
	}
	health, err := ReadHealthSnapshot(path, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "dropped" || health.Dropped != 3 || health.Detail != "" {
		t.Fatalf("health = %#v", health)
	}
	health, err = ReadHealthSnapshot(path, now.Add(HealthStaleAfter+time.Second))
	if err != nil || health.Status != "offline" {
		t.Fatalf("stale health/error = %#v/%v", health, err)
	}
}

func TestHealthSnapshotRejectsUnknownDuplicateAndOversize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.json")
	for _, raw := range []string{`{"version":1,"version":1,"updatedAt":"2026-01-01T00:00:00Z","health":{"status":"healthy"}}`, `{"version":1,"updatedAt":"2026-01-01T00:00:00Z","health":{"status":"healthy"},"extra":1}`, `{"version":1,"updatedAt":"2026-01-01T00:00:00Z","health":{"status":"evil"}}`, `{"version":1,"updatedAt":"2026-01-01T00:00:00Z","health":{}}`, `{"version":1,"updatedAt":"2026-01-01T00:00:00Z","health":{"status":"healthy","dropped":-1}}`, string(make([]byte, 4097))} {
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		health, err := ReadHealthSnapshot(path, time.Now())
		if err != nil || health.Status != "incompatible" {
			t.Fatalf("invalid snapshot status/error = %#v/%v for %q", health, err, raw[:min(len(raw), 40)])
		}
	}
}

func TestWriteHealthSnapshotCanonicalizesDroppedStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.json")
	now := time.Now().UTC()
	if err := WriteHealthSnapshot(path, contracts.FunctionLogHealth{Status: "healthy", Dropped: 2}, now); err != nil {
		t.Fatal(err)
	}
	health, err := ReadHealthSnapshot(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "dropped" {
		t.Fatalf("status = %q", health.Status)
	}
}

func TestHealthSnapshotFutureTimestampIsIncompatible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health.json")
	now := time.Now().UTC()
	if err := WriteHealthSnapshot(path, contracts.FunctionLogHealth{Status: "healthy"}, now.Add(HealthStaleAfter+time.Second)); err != nil {
		t.Fatal(err)
	}
	health, err := ReadHealthSnapshot(path, now)
	if err != nil || health.Status != "incompatible" {
		t.Fatalf("health/error = %#v/%v", health, err)
	}
}
