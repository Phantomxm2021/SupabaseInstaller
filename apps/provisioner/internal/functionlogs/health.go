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

type healthSnapshot struct {
	Version   int                         `json:"version"`
	UpdatedAt time.Time                   `json:"updatedAt"`
	Health    contracts.FunctionLogHealth `json:"health"`
}

func WriteHealthSnapshot(path string, health contracts.FunctionLogHealth, now time.Time) error {
	if path == "" {
		return nil
	}
	health.Detail = ""
	raw, err := json.Marshal(healthSnapshot{Version: 1, UpdatedAt: now.UTC(), Health: health})
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
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) > 4096 || rejectDuplicateJSONKeys(raw) != nil {
		return contracts.FunctionLogHealth{}, errors.New("invalid health snapshot")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot healthSnapshot
	if decoder.Decode(&snapshot) != nil || decoder.Decode(&struct{}{}) != io.EOF || snapshot.Version != 1 || snapshot.UpdatedAt.IsZero() {
		return contracts.FunctionLogHealth{}, errors.New("invalid health snapshot")
	}
	if now.Sub(snapshot.UpdatedAt) > HealthStaleAfter {
		snapshot.Health.Status = "offline"
	}
	snapshot.Health.Detail = ""
	return snapshot.Health, nil
}
