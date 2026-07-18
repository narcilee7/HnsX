// Package app is the composition root for hnsxd. It is the only package
// allowed to import across all layers (domain / service / infra / api /
// grpc / ws / cli / server) and to wire concrete implementations to ports.
//
// The split:
//
//	domain.*   — pure entities + ports (no infra imports)
//	service.*  — orchestrates domain via ports (no concrete infra imports)
//	infra.*    — implements domain ports
//	api/grpc/ws/cli — transports, depend only on service layer
//	app        — wires the above; nothing else is allowed to do so
//
// New resources are added by:
//
//  1. declaring the entity + repo port in internal/domain/<resource>/
//  2. implementing the repo in internal/infra/db/postgres/
//  3. adding commands/queries in internal/service/<resource>/
//  4. exposing transport via internal/api/<resource>/{router,handler,dto}
//
// app.New wires all four steps together.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
	agentinfra "github.com/hnsx-io/hnsx/server/internal/infra/agentruntime" // concrete backends
)

// Config holds runtime configuration for hnsxd. Loaded from
// HNSX_* environment variables and an optional YAML file.
type Config struct {
	// HTTPAddr is the bind address for the HTTP+WS server (default ":8080").
	HTTPAddr string `yaml:"http_addr" env:"HNSX_HTTP_ADDR"`
	// PostgresDSN is the connection string for the HnsX Postgres database.
	PostgresDSN string `yaml:"postgres_dsn" env:"HNSX_POSTGRES_DSN"`
	// LogLevel is one of debug, info, warn, error (default "info").
	LogLevel string `yaml:"log_level" env:"HNSX_LOG_LEVEL"`
	// ClaudeExecutable overrides the `claude` CLI lookup path (default: PATH).
	ClaudeExecutable string `yaml:"claude_executable" env:"HNSX_CLAUDE_EXECUTABLE"`
}

// LoadConfig reads configuration from the given file (if non-empty) and
// environment variables. File values are overridden by HNSX_* env vars.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		HTTPAddr:         getEnv("HNSX_HTTP_ADDR", ":8080"),
		PostgresDSN:      os.Getenv("HNSX_POSTGRES_DSN"),
		LogLevel:         getEnv("HNSX_LOG_LEVEL", "info"),
		ClaudeExecutable: getEnv("HNSX_CLAUDE_EXECUTABLE", ""),
	}
	if path != "" {
		return nil, fmt.Errorf("config file loading not yet implemented: %s", path)
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Application is the wired-up runtime. Held by main; Close() releases
// every resource opened during New.
type Application struct {
	cfg      *Config
	logger   *slog.Logger
	Backends agentruntime.Registry

	// Wired components go here as R1.4+ lands:
	//   dbPool    *pgxpool.Pool
	//   workspaceRepo workspace.Repo
	//   workspaceSvc  *workspacesvc.Service
	//   router        *gin.Engine
	//   ...
}

// New constructs the application: loads config, opens DB pools, wires
// repos -> services -> handlers -> router.
func New(ctx context.Context, cfg *Config) (*Application, error) {
	if cfg == nil {
		return nil, errors.New("app: nil config")
	}

	logger, err := newLogger(cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}

	registry := agentinfra.NewRegistry(logger)
	claudeRunner := agentinfra.NewClaudeRunner(cfg.ClaudeExecutable, logger)
	registry.Register(agentinfra.NewClaudeBackend(claudeRunner))
	logger.Info("app: agent backends registered",
		"backends", strings.Join(registry.List(), ", "),
	)

	return &Application{
		cfg:      cfg,
		logger:   logger,
		Backends: registry,
	}, nil
}

// Serve starts the HTTP+WS server and blocks until ctx is cancelled.
func (a *Application) Serve(ctx context.Context) error {
	// Real server wiring lands in R1.6 (gin router). For now we just signal
	// that the binary is alive so the skeleton is exercised end-to-end.
	a.logger.Info("app: serve (skeleton mode — HTTP/WS server lands in R1.6)",
		"http_addr", a.cfg.HTTPAddr,
	)
	<-ctx.Done()
	return ctx.Err()
}

// Close releases every resource held by the application.
func (a *Application) Close() {
	a.logger.Info("app: closing")
}

// Logger returns the application logger so callers (CLI subcommands,
// HTTP handlers) can use the same configured sink.
func (a *Application) Logger() *slog.Logger { return a.logger }

// newLogger builds a slog logger at the configured level.
func newLogger(level string) (*slog.Logger, error) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info", "":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q", level)
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return slog.New(h), nil
}