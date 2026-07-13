// Package db — SQLite backend for hnsx-server.
//
// OpenSQLite is the embedded-mode counterpart to Open. It returns a *DB
// backed by a single SQLite file at the given path. WAL mode is enabled
// to allow concurrent readers while a session is writing observations.
//
// As of v1.0 the existing goose migrations under /go/migrations target
// Postgres-specific syntax (gen_random_uuid, JSONB, TIMESTAMPTZ). They are
// NOT automatically portable to SQLite — running Migrate on a SQLite DB
// will fail with a parser error. Porting the schema is tracked for v1.1;
// in the meantime OpenSQLite is wired so the daemon boots and serves the
// API surface, and Postgres remains the recommended backend.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go sqlite driver (no cgo)
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// OpenSQLite opens a SQLite file at path. The parent directory is created
// if missing. WAL + busy_timeout are enabled to make observation writes
// from a worker concurrent with control-plane reads.
//
// The returned *DB uses the same struct shape as the Postgres path so the
// rest of the server doesn't branch on backend.
func OpenSQLite(path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("db.OpenSQLite: empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("db.OpenSQLite: abs path: %w", err)
	}

	// DSN parameters: WAL mode for concurrent reads during writes, and a
	// 5s busy_timeout so SQLite waits instead of erroring on transient
	// writer collisions. _pragma foreign_keys enforces FK constraints
	// (off by default in SQLite).
	dsn := abs + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"

	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("db.OpenSQLite: gorm open: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("db.OpenSQLite: underlying sql.DB: %w", err)
	}
	// SQLite serializes writers; one open connection avoids "database is
	// locked" errors under load. Reads scale via WAL.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0) // never expire — the file is local

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db.OpenSQLite: ping: %w", err)
	}

	return &DB{SQL: sqlDB, GormDB: gormDB, DSN: dsn}, nil
}

// sqliteDriverName is what sql.Open-style drivers register themselves as.
// Both modernc.org/sqlite and mattn/go-sqlite3 use "sqlite3".
const sqliteDriverName = "sqlite3"

// IsSQLite reports whether the receiver's underlying sql.DB is backed by
// SQLite. Used by Migrate to pick the right goose dialect.
func (d *DB) IsSQLite() bool {
	if d == nil || d.SQL == nil {
		return false
	}
	// database/sql/driver.Driver has no public Name() method and
	// sql.Register has no inverse lookup. Inspect the driver's runtime
	// type name instead — "Driver" is the convention used by gorm's
	// postgres and sqlite adapters.
	t := reflect.TypeOf(d.SQL.Driver()).String()
	return strings.Contains(t, "sqlite")
}

// Ensure sql.DB is imported (used by the *sql.DB signature in helpers).
var _ = sql.ErrNoRows