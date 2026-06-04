// cmd/server/main.go
// GeoTrace sidecar service entrypoint.
// Initializes all components in dependency order, then blocks until
// a SIGINT/SIGTERM signal triggers graceful shutdown.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/BennerG/geotrace/internal/config"
	"github.com/BennerG/geotrace/internal/store"
)

func main() {
	// Load .env file if present (dev convenience — ignored in production
	// where env vars are set by systemd or the container runtime)
	_ = godotenv.Load()

	// Structured logging — slog is stdlib in Go 1.21+
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Root context — cancelled on shutdown signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Store ──────────────────────────────────────────────────────────────
	slog.Info("connecting to postgres")
	st, err := store.New(ctx, cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer st.Close()

	// ── Migrations ─────────────────────────────────────────────────────────
	// Run on every startup — safe because migrate.go is idempotent.
	// In production, swap this for a separate migration job if your team
	// prefers explicit migration control.
	slog.Info("running migrations", "dir", cfg.MigrationsDir)
	if err := store.Migrate(ctx, st.Pool(), cfg.MigrationsDir); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// ── HTTP server ────────────────────────────────────────────────────────
	// Remaining components (enricher, WebSocket hub, ingest handler, REST API)
	// are wired in subsequent steps. For now, a health endpoint confirms
	// the service is running and Postgres is reachable.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Ping(ctx); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","time":%q}`, time.Now().UTC().Format(time.RFC3339))
	})

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Run server in a goroutine so we can wait for shutdown signal
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("geotrace listening", "port", cfg.Port)
		serverErr <- srv.ListenAndServe()
	}()

	// Block until shutdown signal or server error
	select {
	case err := <-serverErr:
		return fmt.Errorf("server: %w", err)
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	// Graceful shutdown — give in-flight requests 15 seconds to finish
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	slog.Info("geotrace stopped cleanly")
	return nil
}
