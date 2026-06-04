// internal/config/config.go
// Package config loads and validates all GeoTrace configuration from environment
// variables. Using a typed Config struct (rather than os.Getenv scattered
// throughout the codebase) makes it trivial to see everything the service needs
// at a glance, and makes tests easy to construct.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all runtime configuration for the GeoTrace sidecar service.
type Config struct {
	// DatabaseDSN is the Postgres connection string.
	// Format: postgres://user:pass@host:5432/dbname?sslmode=disable
	DatabaseDSN string

	// MaxMindDBPath is the filesystem path to the GeoLite2-City.mmdb file.
	// Download from: https://dev.maxmind.com/geoip/geolite2-free-geolocation-data
	MaxMindDBPath string

	// MigrationsDir is the path to the directory containing *.sql migration files.
	// Defaults to "./migrations" relative to the working directory.
	MigrationsDir string

	// IngestAPIKey is the shared secret that clients must send in the
	// X-GeoTrace-Key header when posting events to /ingest.
	// If empty, the ingest endpoint is unauthenticated (dev mode only).
	IngestAPIKey string

	// Port is the TCP port the GeoTrace HTTP server listens on.
	// Default: 8090 (avoids conflict with bennerg.com on 8080/443)
	Port int

	// EnricherWorkers is the number of goroutines that drain the ingest
	// channel and perform MaxMind lookups + Postgres writes in parallel.
	// Default: 4 — enough for any single-droplet workload.
	EnricherWorkers int

	// ChannelBuffer is the size of the buffered event channel between
	// the ingest handler and the enricher goroutines.
	// If the channel fills (e.g. Postgres is slow), ingest events are dropped
	// rather than blocking the caller's request.
	// Default: 512
	ChannelBuffer int

	// CORSOrigins is the allowed CORS origin for the React dashboard.
	// Default: "*" in dev, should be "https://bennerg.com" in production.
	CORSOrigins string
}

// Load reads Config from environment variables.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	cfg := &Config{
		MigrationsDir:   getEnv("MIGRATIONS_DIR", "./migrations"),
		IngestAPIKey:    getEnv("INGEST_API_KEY", ""),
		Port:            getEnvInt("PORT", 8090),
		EnricherWorkers: getEnvInt("ENRICHER_WORKERS", 4),
		ChannelBuffer:   getEnvInt("CHANNEL_BUFFER", 512),
		CORSOrigins:     getEnv("CORS_ORIGINS", "*"),
	}

	// Required fields
	cfg.DatabaseDSN = os.Getenv("DATABASE_DSN")
	if cfg.DatabaseDSN == "" {
		return nil, fmt.Errorf("config: DATABASE_DSN is required")
	}

	cfg.MaxMindDBPath = os.Getenv("MAXMIND_DB_PATH")
	if cfg.MaxMindDBPath == "" {
		return nil, fmt.Errorf("config: MAXMIND_DB_PATH is required")
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("config: PORT %d is out of range", cfg.Port)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
