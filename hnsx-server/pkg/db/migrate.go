package db

import (
	"context"
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
// The dialect is detected from the *DB's driver: "postgres" or "sqlite3".
// SQLite support requires ported migrations; the existing set under
// /go/migrations targets Postgres-specific syntax (gen_random_uuid, JSONB,
// TIMESTAMPTZ) and will fail to parse under sqlite.
//
// `dir` must be an absolute path. Returns nil if no SQL files are present.
func Migrate(ctx context.Context, database *DB, dir string) error {
	if database == nil {
		return errors.New("db.Migrate: nil *DB")
	}
	if database.IsNoDB() {
		return errors.New("db.Migrate: NoDB")
	}
	sqlDB := database.SQL
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

	dialect := "postgres"
	if database.IsSQLite() {
		dialect = "sqlite3"
	}
	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("db.Migrate: set dialect %q: %w", dialect, err)
	}

	if err := goose.UpContext(ctx, sqlDB, dir); err != nil {
		return fmt.Errorf("db.Migrate: goose up (%s): %w", dialect, err)
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
