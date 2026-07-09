// Package db wraps the Postgres connectivity used by hnsx-server.
//
// Phase 1 exposes:
//
//   - Open(ctx, dsn): returns a *DB with both a *pgxpool.Pool (for runtime
//     reads/writes) and a stdlib *sql.DB (for goose migrations and
//     compatibility).
//   - NoDB(): sentinel for environments with no DB configured.
//   - Close: releases the pool + stdlib connection.
//
// All exported names are safe for concurrent use.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// DB holds both pgxpool and stdlib connections against the same DSN.
type DB struct {
	Pool *pgxpool.Pool
	SQL  *sql.DB
	DSN  string
}

// Open establishes a Postgres connection pool and stdlib *sql.DB. If dsn is
// empty, returns (NoDB(), nil) so the server can boot in DB-less mode.
func Open(ctx context.Context, dsn string) (*DB, error) {
	if dsn == "" {
		return NoDB(), nil
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse dsn: %w", err)
	}
	cfg.MaxConns = 16
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: new pool: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(probeCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}

	// stdlib *sql.DB for goose / general compatibility. The connection
	// string returned by stdlib.RegisterConnConfig is registered against
	// the "pgx" driver and can be used with sql.Open.
	connStr := stdlib.RegisterConnConfig(cfg.ConnConfig)
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: open stdlib: %w", err)
	}
	sqlDB.SetMaxOpenConns(int(cfg.MaxConns))
	sqlDB.SetMaxIdleConns(int(cfg.MinConns))
	sqlDB.SetConnMaxLifetime(cfg.MaxConnLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.MaxConnIdleTime)

	return &DB{Pool: pool, SQL: sqlDB, DSN: dsn}, nil
}

// NoDB returns a sentinel DB with no underlying connections.
func NoDB() *DB { return &DB{} }

// IsNoDB reports whether the receiver has no underlying pool/sql.DB.
func (d *DB) IsNoDB() bool { return d == nil || d.Pool == nil || d.SQL == nil }

// Close releases both connections. Safe to call on NoDB.
func (d *DB) Close() {
	if d == nil {
		return
	}
	if d.Pool != nil {
		d.Pool.Close()
	}
	if d.SQL != nil {
		_ = d.SQL.Close()
	}
}

// ErrNoRows is a convenience re-export so callers don't have to import
// pgx directly just to compare against pgx.ErrNoRows.
var ErrNoRows = errors.New("db: no rows in result set")
