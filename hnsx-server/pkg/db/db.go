// Package db wraps the Postgres connectivity used by hnsx-server.
//
// Phase 1 exposes:
//
//   - Open(ctx, dsn): returns a *DB with a stdlib *sql.DB (for goose migrations)
//     and a GORM *gorm.DB (for repositories and telemetry sinks).
//   - NoDB(): sentinel for environments with no DB configured.
//   - Close: releases the stdlib connection.
//
// All exported names are safe for concurrent use.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DB holds stdlib and gorm connections against the same DSN.
type DB struct {
	SQL    *sql.DB
	GormDB *gorm.DB
	DSN    string
}

// Open establishes a Postgres connection pool and GORM session. If dsn is
// empty, returns (NoDB(), nil) so the server can boot in DB-less mode.
func Open(ctx context.Context, dsn string) (*DB, error) {
	if dsn == "" {
		return NoDB(), nil
	}

	// GORM opens the connection using the Postgres driver (pgx under the hood).
	gormDB, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("db: open gorm: %w", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("db: get underlying sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(16)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	return &DB{SQL: sqlDB, GormDB: gormDB, DSN: dsn}, nil
}

// NoDB returns a sentinel DB with no underlying connections.
func NoDB() *DB { return &DB{} }

// IsNoDB reports whether the receiver has no underlying sql.DB/gormDB.
func (d *DB) IsNoDB() bool { return d == nil || d.SQL == nil || d.GormDB == nil }

// Close releases all connections. Safe to call on NoDB.
func (d *DB) Close() {
	if d == nil {
		return
	}
	if d.SQL != nil {
		_ = d.SQL.Close()
	}
}
