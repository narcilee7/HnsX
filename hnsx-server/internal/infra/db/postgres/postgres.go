// Package postgres hosts the Postgres-backed implementations of every
// domain Repo port declared under internal/domain/*.
//
// R1.4 uses GORM (gorm.io/gorm + gorm.io/driver/postgres) over pgx. The
// choice was deliberate: GORM's struct-tag-driven migrations + AutoMigrate
// covers our schema without a parallel SQL file. Repos translate between
// GORM records (which ARE the domain entities — see the gorm tags in
// internal/domain/*) and the domain port interfaces.
//
// Layering invariant (enforced by code review):
//   * this package imports only stdlib + gorm + zap + domain ports
//   * domain/* never imports this package
//   * service/* imports only domain ports; app/* wires concrete impls in
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/agent"
	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
	"github.com/hnsx-io/hnsx/server/internal/domain/daemon"
	"github.com/hnsx-io/hnsx/server/internal/domain/eval"
	"github.com/hnsx-io/hnsx/server/internal/domain/harness"
	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
	"github.com/hnsx-io/hnsx/server/internal/domain/observation"
	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
	"github.com/hnsx-io/hnsx/server/internal/domain/squad"
	"github.com/hnsx-io/hnsx/server/internal/domain/workspace"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Config carries pool + migration settings.
type Config struct {
	// DSN is a libpq-style connection string. Required.
	// Example: postgres://hnsx:hnsx@127.0.0.1:5432/hnsx?sslmode=disable
	DSN string

	// MaxOpenConns caps the connection pool. Default 25.
	MaxOpenConns int
	// MaxIdleConns keeps idle connections warm. Default 5.
	MaxIdleConns int
	// ConnMaxLifetime recycles connections after this duration. Default 30m.
	ConnMaxLifetime time.Duration
	// LogQueries enables GORM's verbose SQL logger at info level.
	LogQueries bool
}

// DB wraps a *gorm.DB. Every repo in this package takes a *DB; services
// receive concrete *postgres.WorkspaceRepo, *postgres.AgentRepo, etc.
type DB struct {
	*gorm.DB
	logger *zap.Logger
}

// Open constructs the connection pool, applies AutoMigrate, and pings.
// Returns a usable *DB or an error explaining the failure (DSN parse,
// dial error, migration error, ...).
//
// AutoMigrate runs in dependency order: workspaces first (FK target),
// then agents/issues/squads/daemons, then observations (no FK — events
// are append-only and idempotent by ID).
func Open(ctx context.Context, cfg Config, logger *zap.Logger) (*DB, error) {
	if cfg.DSN == "" {
		return nil, errors.New("postgres: empty DSN")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	gormCfg := &gorm.Config{
		Logger:                                   newGormLogger(logger, cfg.LogQueries),
		DisableForeignKeyConstraintWhenMigrating: false,
		NowFunc:                                  func() time.Time { return time.Now().UTC() },
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres: get sql.DB: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	} else {
		sqlDB.SetMaxOpenConns(25)
	}
	if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	} else {
		sqlDB.SetMaxIdleConns(5)
	}
	if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	} else {
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	d := &DB{DB: db, logger: logger}
	if err := d.migrate(ctx); err != nil {
		sqlDB, _ := d.DB.DB()
		_ = sqlDB.Close()
		return nil, fmt.Errorf("postgres: migrate: %w", err)
	}

	logger.Info("postgres: opened + migrated",
		zap.Int("max_open", cfg.MaxOpenConns),
		zap.Int("max_idle", cfg.MaxIdleConns),
	)
	return d, nil
}

// migrate runs AutoMigrate on every domain model. GORM creates tables
// that don't exist and adds missing columns; it does not drop or alter
// existing columns (so production rollouts stay safe).
func (d *DB) migrate(_ context.Context) error {
	return d.AutoMigrate(
		&workspace.Workspace{},
		&agent.Agent{},
		&issue.Issue{},
		&squad.Squad{},
		&daemon.Daemon{},
		&observation.Observation{},
		// R3 value modules
		&harness.Harness{},
		&policy.Policy{},
		&eval.EvalSet{},
		&eval.Run{},
		&approval.Approval{},
	)
}

// Close closes the underlying connection pool. Safe to call multiple times.
func (d *DB) Close() error {
	if d == nil || d.DB == nil {
		return nil
	}
	sqlDB, err := d.DB.DB()
	if err != nil {
		return err
	}
	d.logger.Info("postgres: closing")
	return sqlDB.Close()
}

// newGormLogger adapts a zap logger to GORM's logger interface so SQL
// queries can land in the same sink as the rest of the application.
func newGormLogger(logger *zap.Logger, verbose bool) gormlogger.Interface {
	level := gormlogger.Warn
	if verbose {
		level = gormlogger.Info
	}
	return gormlogger.New(
		zapToGormWriter{logger: logger},
		gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  level,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
}

// zapToGormWriter is a minimal io.Writer adapter so we can pipe GORM's
// line-based logger into zap. We just route to logger.Info; the message
// contains the SQL.
//
// GORM's logger.Writer requires both Write and Printf. Printf is invoked
// for slow-query / error paths; we route those to logger.Warn so they
// stand out from the normal SQL log.
type zapToGormWriter struct{ logger *zap.Logger }

func (z zapToGormWriter) Write(p []byte) (int, error) {
	z.logger.Info("gorm", zap.String("sql", string(p)))
	return len(p), nil
}

func (z zapToGormWriter) Printf(format string, args ...any) {
	z.logger.Warn("gorm", zap.String("msg", fmt.Sprintf(format, args...)))
}