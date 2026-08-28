package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	if err := ensureAuthDirectory(settings.AuthDirectory); err != nil {
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
		settings.AuthDirectory,
		site.NewSystemRunner(site.OSExecutor{}, settings.NginxBinary, settings.SystemctlBinary),
	)
	handler := server.New(settings.Token, site.NewRenderer(site.TLSPaths{
		CertificateFile: settings.CertificateFile, CertificateKeyFile: settings.CertificateKeyFile, AuthDirectory: settings.AuthDirectory,
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

// ensureAuthDirectory keeps the credential directory readable by the Nginx
// worker. The directory contains only per-site htpasswd files, which are
// already written with restrictive file permissions by the site store. A
// 0700 directory prevents Nginx from traversing the path and turns every
// protected Studio request into a 500 response after an agent restart.
func ensureAuthDirectory(path string) error {
	cleanPath := filepath.Clean(path)
	parentPath := filepath.Dir(cleanPath)
	if parentPath == cleanPath || parentPath == string(filepath.Separator) {
		return fmt.Errorf("Studio credential directory must have a dedicated parent directory")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create Studio credential directory: %w", err)
	}
	if err := os.Chmod(parentPath, 0o711); err != nil {
		return fmt.Errorf("set Studio credential parent directory permissions: %w", err)
	}
	if err := os.Chmod(cleanPath, 0o755); err != nil {
		return fmt.Errorf("set Studio credential directory permissions: %w", err)
	}
	return nil
}
