package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"supabase-manager/apps/provisioner/internal/compose"
	provisionerconfig "supabase-manager/apps/provisioner/internal/config"
	"supabase-manager/apps/provisioner/internal/health"
	"supabase-manager/apps/provisioner/internal/projectfs"
	"supabase-manager/apps/provisioner/internal/proxy"
	provisionerruntime "supabase-manager/apps/provisioner/internal/runtime"
	provisionerserver "supabase-manager/apps/provisioner/internal/server"
)

func main() {
	cfg, err := provisionerconfig.Load()
	if err != nil {
		slog.Error("invalid provisioner configuration", "error", err)
		os.Exit(1)
	}

	root, err := projectfs.New(cfg.ProjectRoot)
	if err != nil {
		slog.Error("initialize project root", "error", err)
		os.Exit(1)
	}
	dockerSource, err := health.NewDockerSource(cfg.DockerHost)
	if err != nil {
		slog.Error("initialize Docker client", "error", err)
		os.Exit(1)
	}
	proxyClient := proxy.Client(proxy.DisabledClient{})
	if cfg.NginxProxyMode == "managed" {
		proxyClient = proxy.NewManagedClient(cfg.NginxProxySocket, cfg.NginxProxyToken)
	}
	backend := provisionerruntime.NewBackend(root, compose.NewRunner(compose.OSExecutor{}), health.NewInspector(dockerSource), proxyClient)
	if cfg.AcceptanceInspectorFailOnce {
		backend.EnableAcceptanceInspectorFailure()
	}
	handler := provisionerserver.New(provisionerserver.Options{ManagerToken: cfg.ManagerToken, ProjectFS: root, Backend: backend})
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	run(server)
}

func run(server *http.Server) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("provisioner shutdown failed", "error", err)
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("provisioner server failed", "error", err)
		os.Exit(1)
	}
}
