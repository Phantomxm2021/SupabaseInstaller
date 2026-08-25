package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"supabase-manager/apps/manager/internal/auth"
	managerconfig "supabase-manager/apps/manager/internal/config"
	"supabase-manager/apps/manager/internal/httpapi"
	"supabase-manager/apps/manager/internal/install"
	"supabase-manager/apps/manager/internal/lifecycle"
	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/apps/manager/internal/ports"
	"supabase-manager/apps/manager/internal/project"
	"supabase-manager/apps/manager/internal/provisioner"
	managersecrets "supabase-manager/apps/manager/internal/secrets"
	"supabase-manager/apps/manager/internal/store"
	"supabase-manager/apps/manager/webdist"
)

func main() {
	cfg, err := managerconfig.Load()
	if err != nil {
		slog.Error("invalid manager configuration", "error", err)
		os.Exit(1)
	}

	database, err := store.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("open manager database", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	cipher, err := managersecrets.NewCipher(cfg.MasterEncryptionKey)
	if err != nil {
		slog.Error("initialize encryption", "error", err)
		os.Exit(1)
	}
	now := time.Now
	adminAuth := auth.NewService(database, auth.NewPasswordHasher(auth.DefaultParams), rand.Reader, now)
	operations := operation.NewService(database, randomID, now)
	projects := project.NewService(database, randomID, now)
	provisionerHTTP := &http.Client{Timeout: 45 * time.Minute}
	provisionerClient := provisioner.NewClient(cfg.ProvisionerURL, cfg.ProvisionerToken, provisionerHTTP)
	allocator := ports.NewAllocator(database, cfg.PortRangeStart, cfg.PortRangeEnd, ports.NetworkProbe{})
	installer := install.NewOrchestrator(database, operations, allocator, cipher, provisionerClient, install.CryptoGenerator{Random: rand.Reader, Now: now}, now)
	lifecycleManager := lifecycle.NewService(database, operations, provisionerClient)
	api := httpapi.NewRouter(httpapi.RouterOptions{
		Auth:       httpapi.AuthOptions{Service: adminAuth, PublicOrigin: cfg.PublicOrigin, SecureCookies: cfg.SecureCookies},
		Projects:   httpapi.ProjectOptions{Projects: projects, Installer: installer, Lifecycle: lifecycleManager},
		Operations: operations,
	})
	assets := webdist.Embedded()
	if cfg.WebDistPath != "" {
		assets = os.DirFS(cfg.WebDistPath)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, request *http.Request) {
		if err := database.DB().PingContext(request.Context()); err != nil || !provisionerReady(request.Context(), cfg.ProvisionerURL, provisionerHTTP) {
			http.Error(w, "dependencies unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/api/", api)
	mux.Handle("/", webdist.NewHandler(fs.FS(assets)))
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	run(server)
}

func randomID() string {
	value := make([]byte, 18)
	if _, err := rand.Read(value); err != nil {
		panic("secure random source unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(value)
}

func provisionerReady(ctx context.Context, baseURL string, client *http.Client) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health/live", nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusNoContent
}

func run(server *http.Server) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("manager shutdown failed", "error", err)
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("manager server failed", "error", err)
		os.Exit(1)
	}
}
