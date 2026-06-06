package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseDSN     string
	MaxMindDBPath   string
	MigrationsDir   string
	IngestAPIKey    string
	Port            int
	EnricherWorkers int
	ChannelBuffer   int
	CORSOrigins     string
}

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
