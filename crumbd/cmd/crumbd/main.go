// Command crumbd is crumb's self-hosted sync server: a single static binary
// storing one versioned encrypted blob per vault, authenticated by SSH
// signature challenge.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"crumbd/internal/config"
	"crumbd/internal/db"
	"crumbd/internal/httpapi"
	"crumbd/internal/store"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml (optional; env vars and defaults apply if omitted)")
	flag.Parse()

	if err := run(*configPath); err != nil {
		slog.Error("crumbd exited with error", "error", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	sqlDB, err := db.Open(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer sqlDB.Close()

	st := store.New(sqlDB)
	handler := httpapi.NewRouter(st, cfg)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("crumbd listening", "addr", cfg.ListenAddr, "database", cfg.DatabasePath, "registration_mode", cfg.RegistrationMode)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}
}
