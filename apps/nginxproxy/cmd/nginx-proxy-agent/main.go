package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"supabase-manager/apps/nginxproxy/internal/config"
	"supabase-manager/apps/nginxproxy/internal/server"
	"supabase-manager/apps/nginxproxy/internal/site"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	settings, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	listener, err := server.ListenUnix(settings.SocketPath)
	if err != nil {
		return err
	}
	defer listener.Close()

	store := site.NewStore(
		settings.SitesAvailable,
		settings.SitesEnabled,
		site.NewSystemRunner(site.OSExecutor{}, settings.NginxBinary, settings.SystemctlBinary),
	)
	handler := server.New(settings.Token, site.NewRenderer(site.TLSPaths{
		CertificateFile: settings.CertificateFile, CertificateKeyFile: settings.CertificateKeyFile,
	}), store)
	httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- httpServer.Serve(listener) }()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-shutdown:
		context, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(context)
	}
}
