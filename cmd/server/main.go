package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"github.com/BennerG/geotrace/internal/api"
	"github.com/BennerG/geotrace/internal/config"
	"github.com/BennerG/geotrace/internal/enricher"
	"github.com/BennerG/geotrace/internal/ingest"
	"github.com/BennerG/geotrace/internal/store"
	"github.com/BennerG/geotrace/internal/ws"
)

func main() {
	_ = godotenv.Load()

	// structured logging
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

	// root context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// store
	slog.Info("connecting to postgres")
	st, err := store.New(ctx, cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	defer st.Close()

	// migrations
	slog.Info("running migrations", "dir", cfg.MigrationsDir)
	if err := store.Migrate(ctx, st.Pool(), cfg.MigrationsDir); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	// event channels
	rawEvents := make(chan *store.Event, cfg.ChannelBuffer)
	broadcast := make(chan *store.Event, cfg.ChannelBuffer)

	// enricher
	slog.Info("loading maxmind db", "path", cfg.MaxMindDBPath)
	enc, err := enricher.New(cfg.MaxMindDBPath, st, rawEvents, broadcast, cfg.EnricherWorkers)
	if err != nil {
		return fmt.Errorf("enricher: %w", err)
	}
	defer enc.Close()

	// router
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// API handlers
	apiHandler := api.New(st)
	// health - confirms server is running and checks postgres connection
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Ping(r.Context()); err != nil {
			http.Error(w, "db unreachable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","time":%q}`, time.Now().UTC().Format(time.RFC3339))
	})
	r.Get("/events", apiHandler.Events)
	r.Get("/stats", apiHandler.Stats)
	r.Get("/summary", apiHandler.Summary)

	// ingest
	ingestHandler := ingest.New(cfg.IngestAPIKey, rawEvents)
	r.Post("/ingest", ingestHandler.ServeHTTP)

	// websocket
	hub := ws.NewHub(broadcast, cfg.AllowedOrigins)
	r.Get("/ws", hub.ServeHTTP)

	// static frontend
	if _, err := dist.Open("dist/index.html"); err == nil {
		staticFS, _ := fs.Sub(dist, "dist")
		fileServer := http.FileServer(http.FS(staticFS))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if _, err := staticFS.Open(path); err != nil {
				r.URL.Path = "/"
			}
			fileServer.ServeHTTP(w, r)
		})
	}

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// triggers graceful shutdown of all other components.
	g, ctx := errgroup.WithContext(ctx)

	// enricher worker pool
	g.Go(func() error {
		slog.Info("enricher starting", "workers", cfg.EnricherWorkers)
		return enc.Run(ctx)
	})

	// broadcast channel drain
	g.Go(func() error {
		slog.Info("ws hub starting")
		return hub.Run(ctx)
	})

	// HTTP server
	g.Go(func() error {
		slog.Info("geotrace listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server: %w", err)
		}
		return nil
	})

	// shutdown trigger — when ctx is cancelled (signal received), shut down
	// the HTTP server which unblocks ListenAndServe above
	g.Go(func() error {
		<-ctx.Done()
		slog.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil {
		return err
	}

	slog.Info("geotrace stopped cleanly")
	return nil
}
