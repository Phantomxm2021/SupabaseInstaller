package functionlogs

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"supabase-manager/internal/contracts"
)

const HealthStaleAfter = 30 * time.Second
const maxHealthSnapshotBytes = 512 << 10
const maxPersistedReportIDs = 2048

type healthSnapshot struct {
	Version   int                         `json:"version"`
	UpdatedAt time.Time                   `json:"updatedAt"`
	Health    contracts.FunctionLogHealth `json:"health"`
	ReportIDs []string                    `json:"reportIds,omitempty"`
}

func WriteHealthSnapshot(path string, health contracts.FunctionLogHealth, now time.Time) error {
	return writeHealthSnapshotState(path, health, nil, now)
}

func writeHealthSnapshotState(path string, health contracts.FunctionLogHealth, reportIDs []string, now time.Time) error {
	if path == "" {
		return nil
	}
	health.Detail = ""
	if health.Dropped > 0 && health.Status == "healthy" {
		health.Status = "dropped"
	}
	if !validHealthStatus(health.Status) {
		return errors.New("invalid health status")
	}
	if !validReportIDs(reportIDs) {
		return errors.New("invalid adapter report IDs")
	}
	raw, err := json.Marshal(healthSnapshot{Version: 1, UpdatedAt: now.UTC(), Health: health, ReportIDs: reportIDs})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".health-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

func ReadHealthSnapshot(path string, now time.Time) (contracts.FunctionLogHealth, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return contracts.FunctionLogHealth{Status: "offline"}, nil
	}
	if err != nil {
		return contracts.FunctionLogHealth{}, err
	}
	defer file.Close()
	return ReadHealthSnapshotFile(file, now)
}

func ReadHealthSnapshotFile(file *os.File, now time.Time) (contracts.FunctionLogHealth, error) {
	if file == nil {
		return contracts.FunctionLogHealth{Status: "offline"}, nil
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxHealthSnapshotBytes+1))
	if err != nil || len(raw) > maxHealthSnapshotBytes || rejectDuplicateJSONKeys(raw) != nil {
		return contracts.FunctionLogHealth{Status: "incompatible"}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot healthSnapshot
	if decoder.Decode(&snapshot) != nil || decoder.Decode(&struct{}{}) != io.EOF || snapshot.Version != 1 || snapshot.UpdatedAt.IsZero() || !validReportIDs(snapshot.ReportIDs) {
		return contracts.FunctionLogHealth{Status: "incompatible"}, nil
	}
	if !validHealthStatus(snapshot.Health.Status) {
		return contracts.FunctionLogHealth{Status: "incompatible"}, nil
	}
	if snapshot.Health.Dropped > 0 && snapshot.Health.Status == "healthy" {
		snapshot.Health.Status = "dropped"
	}
	if now.Sub(snapshot.UpdatedAt) > HealthStaleAfter {
		snapshot.Health.Status = "offline"
	}
	if snapshot.UpdatedAt.Sub(now) > HealthStaleAfter {
		snapshot.Health = contracts.FunctionLogHealth{Status: "incompatible"}
	}
	snapshot.Health.Detail = ""
	return snapshot.Health, nil
}

func readHealthSnapshotForRestart(path string, now time.Time) (contracts.FunctionLogHealth, bool, error) {
	health, _, valid, err := readHealthSnapshotStateForRestart(path, now)
	return health, valid, err
}

func readHealthSnapshotStateForRestart(path string, now time.Time) (contracts.FunctionLogHealth, []string, bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return contracts.FunctionLogHealth{}, nil, false, nil
	}
	if err != nil {
		return contracts.FunctionLogHealth{}, nil, false, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxHealthSnapshotBytes+1))
	if err != nil {
		return contracts.FunctionLogHealth{}, nil, false, err
	}
	if len(raw) > maxHealthSnapshotBytes || rejectDuplicateJSONKeys(raw) != nil {
		return contracts.FunctionLogHealth{}, nil, false, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot healthSnapshot
	if decoder.Decode(&snapshot) != nil || decoder.Decode(&struct{}{}) != io.EOF || snapshot.Version != 1 || snapshot.UpdatedAt.IsZero() || !validHealthStatus(snapshot.Health.Status) || snapshot.UpdatedAt.Sub(now) > HealthStaleAfter || !validReportIDs(snapshot.ReportIDs) {
		return contracts.FunctionLogHealth{}, nil, false, nil
	}
	snapshot.Health.Detail = ""
	return snapshot.Health, append([]string(nil), snapshot.ReportIDs...), true, nil
}

func validReportIDs(ids []string) bool {
	if len(ids) > maxPersistedReportIDs {
		return false
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if len(id) > 128 || !projectIDPattern.MatchString(id) {
			return false
		}
		if _, exists := seen[id]; exists {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

func validHealthStatus(status string) bool {
	switch status {
	case "healthy", "dropped", "offline", "incompatible", "disabled", "not_installed", "storage_error":
		return true
	default:
		return false
	}
}
