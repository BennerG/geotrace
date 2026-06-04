// internal/store/migrate.go
// Simple migration runner. Reads *.sql files from a directory in lexicographic
// order and runs each in a transaction. Tracks applied migrations in a
// schema_migrations table so reruns are safe (idempotent).
package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate applies all unapplied *.sql files from migrationsDir to the database.
// Safe to call on every startup — already-applied migrations are skipped.
// Migrations are applied in filename order, so use a numeric prefix (001_, 002_).
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	// Ensure the tracking table exists
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT        PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	if err != nil {
		return fmt.Errorf("migrate: create tracking table: %w", err)
	}

	// Read all .sql files from the migrations directory
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("migrate: read dir %q: %w", migrationsDir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, filename := range files {
		applied, err := isMigrationApplied(ctx, pool, filename)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		sql, err := os.ReadFile(filepath.Join(migrationsDir, filename))
		if err != nil {
			return fmt.Errorf("migrate: read %q: %w", filename, err)
		}

		// Each migration runs in its own transaction
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migrate: begin tx for %q: %w", filename, err)
		}

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrate: apply %q: %w", filename, err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (filename) VALUES ($1)`, filename,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrate: record %q: %w", filename, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migrate: commit %q: %w", filename, err)
		}

		fmt.Printf("migrate: applied %s\n", filename)
	}

	return nil
}

func isMigrationApplied(ctx context.Context, pool *pgxpool.Pool, filename string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`,
		filename,
	).Scan(&exists)
	return exists, err
}
