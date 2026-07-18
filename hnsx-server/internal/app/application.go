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
}

// LoadConfig reads configuration from the given file (if non-empty) and
// environment variables. File values are overridden by HNSX_* env vars.
func LoadConfig(path string) (*Config, error) {
	cfg := &Config{
		HTTPAddr:     ":8080",
		PostgresDSN: "",
		LogLevel:     "info",
	}
	if path != "" {
		// YAML loader lives in a later R1.x; for now we accept an empty
		// path and rely on env defaults so the binary can boot.
		return nil, fmt.Errorf("config file loading not yet implemented: %s", path)
	}
	return cfg, nil
}

// Application is the wired-up runtime. Held by main; Close() releases
// every resource opened during New.
type Application struct {
	cfg *Config
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
	return &Application{cfg: cfg}, nil
}

// Serve starts the HTTP+WS server and blocks until ctx is cancelled.
func (a *Application) Serve(ctx context.Context) error {
	// Real server wiring lands in R1.10 + R1.6. For now we just signal
	// that the binary is alive so the skeleton is exercised end-to-end.
	<-ctx.Done()
	return ctx.Err()
}

// Close releases every resource held by the application.
func (a *Application) Close() {
	// Wired in R1.4.
}