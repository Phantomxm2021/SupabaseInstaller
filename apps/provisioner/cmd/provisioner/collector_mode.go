package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"supabase-manager/apps/provisioner/internal/functionlogs"
)

const collectorMode = "function-log-collector"

type collectorModeConfig struct {
	ProjectID        string
	DatabasePath     string
	FunctionsRoot    string
	ProjectEnvPath   string
	FunctionsEnvPath string
	ListenAddr       string
}

func loadCollectorModeConfig(getenv func(string) string) (collectorModeConfig, error) {
	config := collectorModeConfig{
		ProjectID:        strings.TrimSpace(getenv("FUNCTION_LOG_PROJECT_ID")),
		DatabasePath:     strings.TrimSpace(getenv("FUNCTION_LOG_DATABASE_PATH")),
		FunctionsRoot:    strings.TrimSpace(getenv("FUNCTION_LOG_FUNCTIONS_ROOT")),
		ProjectEnvPath:   strings.TrimSpace(getenv("FUNCTION_LOG_PROJECT_ENV")),
		FunctionsEnvPath: strings.TrimSpace(getenv("FUNCTION_LOG_FUNCTIONS_ENV")),
		ListenAddr:       strings.TrimSpace(getenv("FUNCTION_LOG_LISTEN_ADDR")),
	}
	missing := make([]string, 0, 5)
	for name, value := range map[string]string{
		"FUNCTION_LOG_PROJECT_ID": config.ProjectID, "FUNCTION_LOG_DATABASE_PATH": config.DatabasePath,
		"FUNCTION_LOG_FUNCTIONS_ROOT": config.FunctionsRoot, "FUNCTION_LOG_PROJECT_ENV": config.ProjectEnvPath,
		"FUNCTION_LOG_FUNCTIONS_ENV": config.FunctionsEnvPath,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return collectorModeConfig{}, fmt.Errorf("missing required function log collector configuration: %s", strings.Join(missing, ", "))
	}
	if config.ListenAddr == "" {
		config.ListenAddr = "0.0.0.0:8081"
	}
	return config, nil
}

func runCollectorProcess() error {
	config, err := loadCollectorModeConfig(os.Getenv)
	if err != nil {
		return err
	}
	redactor, err := functionlogs.LoadRedactor(config.ProjectEnvPath, config.FunctionsEnvPath)
	if err != nil {
		return errors.New("load function log redactor")
	}
	store, err := functionlogs.Open(config.DatabasePath, functionlogs.Options{Redactor: redactor})
	if err != nil {
		return errors.New("open function log store")
	}
	defer store.Close()
	collector, err := functionlogs.NewCollector(functionlogs.CollectorOptions{
		ProjectID: config.ProjectID, Store: store, Redactor: redactor, FunctionsRoot: config.FunctionsRoot, Logger: slog.Default(),
	})
	if err != nil {
		return err
	}
	defer collector.Close()
	server := &http.Server{
		Addr: config.ListenAddr, Handler: functionlogs.NewCollectorHandler(collector), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024,
	}
	return run(server)
}
