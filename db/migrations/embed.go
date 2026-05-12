package migrations

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var migrationsFS embed.FS

// Up applies all pending up-migrations to the database.
// It tracks applied migrations in a schema_migrations table.
func Up(ctx context.Context, pool *pgxpool.Pool) error {
	// Ensure migrations tracking table exists
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	// Read and sort migration files
	entries, err := fs.Glob(migrationsFS, "*.up.sql")
	if err != nil {
		return fmt.Errorf("list migration files: %w", err)
	}
	sort.Strings(entries)

	for _, entry := range entries {
		// Check if already applied
		var count int
		err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE filename = $1", entry).Scan(&count)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", entry, err)
		}
		if count > 0 {
			continue
		}

		// Read and apply
		sql, err := migrationsFS.ReadFile(entry)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", entry, err)
		}

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", entry, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (filename) VALUES ($1)", entry); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", entry, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", entry, err)
		}
	}

	return nil
}

// DownFS returns the embedded SQL files for down-migrations.
func DownFS() embed.FS {
	return migrationsFS
}

// MigrationFile returns the base name of a migration file (without .up.sql suffix).
func MigrationFile(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}
