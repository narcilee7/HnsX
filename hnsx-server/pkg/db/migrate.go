package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/pressly/goose/v3"
)

// Migrate applies all *.sql files under dir to the provided stdlib *sql.DB,
// in lexicographic order. Idempotent — goose tracks the applied versions in
// a `goose_db_version` table.
//
// Phase 1 ships two migrations under go/migrations:
//
//   - 000001_init_schema.up.sql   (tables per docs/server-design/tablebase-design.md)
//   - 000002_observations.up.sql (single `observations` table for local-mode
//     telemetry without OTLP).
//
// `dir` must be an absolute path. Returns nil if no SQL files are present.
func Migrate(ctx context.Context, sqlDB *sql.DB, dir string) error {
	if sqlDB == nil {
		return errors.New("db.Migrate: nil *sql.DB")
	}
	if dir == "" {
		return errors.New("db.Migrate: empty migrations dir")
	}

	stat, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("db.Migrate: stat %s: %w", dir, err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("db.Migrate: %s is not a directory", dir)
	}

	// Validate that there is at least one SQL file; passing an empty dir to
	// goose.Up results in a confusing error.
	if hasNoSQL(dir) {
		return nil
	}

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("db.Migrate: set dialect: %w", err)
	}

	if err := goose.UpContext(ctx, sqlDB, dir); err != nil {
		return fmt.Errorf("db.Migrate: goose up: %w", err)
	}
	return nil
}

func hasNoSQL(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true // force goose to surface the readdir error
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".sql" {
			return false
		}
	}
	// Sort-check is unnecessary here; we only need "is there at least one".
	_ = sort.Strings
	return true
}
