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
	"supabase-manager/apps/manager/internal/authadmin"
	managerconfig "supabase-manager/apps/manager/internal/config"
	managerconfiguration "supabase-manager/apps/manager/internal/configuration"
	managerfunctions "supabase-manager/apps/manager/internal/functions"
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
	projects := project.NewServiceWithCipher(database, randomID, now, cipher)
	authAdmin := authadmin.New(database, cipher, &http.Client{Timeout: 20 * time.Second}, authadmin.GatewayAtHost("host.docker.internal"))
	provisionerHTTP := &http.Client{Timeout: provisioner.DefaultRequestTimeout}
	provisionerClient := provisioner.NewClient(cfg.ProvisionerURL, cfg.ProvisionerToken, provisionerHTTP)
	functionSpool, err := managerfunctions.NewSpool(cfg.FunctionUploadSpoolDir)
	if err != nil {
		slog.Error("initialize function upload spool", "error", err)
		os.Exit(1)
	}
	functionManager := managerfunctions.NewService(database, operations, functionSpool, provisionerClient, now)
	if err := functionManager.Resume(context.Background(), projects.Get); err != nil {
		slog.Error("resume functions operations failed", "error", err)
	}
	allocator := ports.NewAllocatorWithContextProbe(database, cfg.PortRangeStart, cfg.PortRangeEnd, ports.NetworkProbe{}, provisionerClient)
	installer := install.NewOrchestrator(database, operations, allocator, cipher, provisionerClient, install.CryptoGenerator{Random: rand.Reader, Now: now}, now)
	configurationManager := managerconfiguration.NewOrchestrator(database, operations, allocator, provisionerClient, cipher, now)
	if err := configurationManager.Resume(context.Background(), projects.Get); err != nil {
		slog.Error("resume configuration operations failed", "error", err)
	}
	// Resume is durable: a worker whose lease expired while the process was
	// unavailable is retried by this scheduler once the lease can be acquired.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := configurationManager.Resume(context.Background(), projects.Get); err != nil {
				slog.Error("scheduled configuration resume failed", "error", err)
			}
			if err := functionManager.Resume(context.Background(), projects.Get); err != nil {
				slog.Error("scheduled functions resume failed", "error", err)
			}
		}
	}()
	lifecycleManager := lifecycle.NewService(database, operations, provisionerClient)
	api := httpapi.NewRouter(httpapi.RouterOptions{
		Auth:          httpapi.AuthOptions{Service: adminAuth, PublicOrigin: cfg.PublicOrigin, SecureCookies: cfg.SecureCookies},
		Projects:      httpapi.ProjectOptions{Projects: projects, AuthAdmin: authAdmin, Installer: installer, Inspector: provisionerClient, Lifecycle: lifecycleManager, ManagedTLS: provisionerClient},
		HostResources: provisionerClient,
		Operations:    operations,
		Configuration: configurationManager,
		Functions:     functionManager,
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
