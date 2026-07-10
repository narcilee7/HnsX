// Package config is the single source of runtime configuration for
// hnsx-server. It resolves values from (in order of precedence):
//
//  1. Optional YAML config file (--config / HNSX_CONFIG_PATH).
//  2. Environment variables (HNSX_* prefix).
//  3. Built-in defaults.
//
// Phase 1 keeps the schema intentionally minimal — only the knobs the
// server needs to boot are listed below.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the resolved runtime configuration.
type Config struct {
	// HTTPAddr is the listen address for the Control Plane HTTP server.
	HTTPAddr string `yaml:"http_addr"`

	// GRPCAddr is the listen address for the V1.1 WorkerService +
	// SchedulerService gRPC surface. Empty disables the gRPC server.
	GRPCAddr string `yaml:"grpc_addr"`

	// DatabaseURL is the Postgres connection string. Empty disables the DB.
	DatabaseURL string `yaml:"database_url"`

	// MigrationsDir is the directory containing *.sql migrations to apply
	// before the server starts accepting traffic.
	MigrationsDir string `yaml:"migrations_dir"`

	// OTel controls OpenTelemetry exporter selection.
	OTel OTelConfig `yaml:"otel"`

	// Log controls structured logging.
	Log LogConfig `yaml:"log"`

	// DomainCache controls whether DomainSpec YAML contents are cached
	// in memory after first load.
	DomainCache bool `yaml:"domain_cache"`

	// Redis is the optional Redis configuration. When present, the worker
	// scheduling queue is backed by Redis so multiple Control Plane
	// instances can share the same queue.
	Redis RedisConfig `yaml:"redis"`
}

// RedisConfig selects a Redis server and queue key namespace.
type RedisConfig struct {
	// Addr is the Redis server address (e.g. "127.0.0.1:6379").
	Addr string `yaml:"addr"`
	// Password is the Redis AUTH password. Optional.
	Password string `yaml:"password"`
	// DB is the Redis logical database number. Optional.
	DB int `yaml:"db"`
	// QueueKeyPrefix is the Redis key prefix for the session queue.
	// Defaults to "hnsx:queue".
	QueueKeyPrefix string `yaml:"queue_key_prefix"`
}

// OTelConfig selects an OpenTelemetry exporter.
type OTelConfig struct {
	// Exporter is one of "stdout", "otlp", "none".
	Exporter string `yaml:"exporter"`
	// OTLPEndpoint is the OTLP gRPC endpoint (e.g. "127.0.0.1:4317").
	OTLPEndpoint string `yaml:"otlp_endpoint"`
	// ServiceName is the service.name resource attribute.
	ServiceName string `yaml:"service_name"`
}

// LogConfig controls zap logger defaults.
type LogConfig struct {
	Level string `yaml:"level"` // debug | info | warn | error
}

// Default returns a Config populated with reasonable defaults for local dev.
func Default() *Config {
	return &Config{
		HTTPAddr:      "127.0.0.1:50051",
		GRPCAddr:      "127.0.0.1:50061",
		DatabaseURL:   "",
		MigrationsDir: "go/migrations",
		OTel: OTelConfig{
			Exporter:     "stdout",
			OTLPEndpoint: "127.0.0.1:4317",
			ServiceName:  "hnsx-server",
		},
		Log: LogConfig{
			Level: "info",
		},
		DomainCache: true,
		Redis: RedisConfig{
			Addr:           "127.0.0.1:6379",
			QueueKeyPrefix: "hnsx:queue",
		},
	}
}

// Load resolves configuration by merging defaults, an optional YAML config
// file, and environment overrides. A missing config file is fine — defaults
// and env are still applied.
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		if _, err := os.Stat(path); err == nil {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read config file %s: %w", path, err)
			}
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config file %s: %w", path, err)
			}
		}
	}

	applyEnv(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyEnv overlays HNSX_* environment variables onto cfg.
func applyEnv(cfg *Config) {
	if _, ok := os.LookupEnv("HNSX_HTTP_ADDR"); ok {
		cfg.HTTPAddr = os.Getenv("HNSX_HTTP_ADDR")
	}
	if _, ok := os.LookupEnv("HNSX_GRPC_ADDR"); ok {
		cfg.GRPCAddr = os.Getenv("HNSX_GRPC_ADDR")
	}
	if _, ok := os.LookupEnv("HNSX_DATABASE_URL"); ok {
		cfg.DatabaseURL = os.Getenv("HNSX_DATABASE_URL")
	}
	if v := os.Getenv("HNSX_MIGRATIONS_DIR"); v != "" {
		cfg.MigrationsDir = v
	}
	if v := os.Getenv("HNSX_OTEL_EXPORTER"); v != "" {
		cfg.OTel.Exporter = v
	}
	if v := os.Getenv("HNSX_OTEL_OTLP_ENDPOINT"); v != "" {
		cfg.OTel.OTLPEndpoint = v
	}
	if v := os.Getenv("HNSX_OTEL_SERVICE_NAME"); v != "" {
		cfg.OTel.ServiceName = v
	}
	if v := os.Getenv("HNSX_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("HNSX_REDIS_ADDR"); v != "" {
		cfg.Redis.Addr = v
	}
	if v := os.Getenv("HNSX_REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("HNSX_REDIS_DB"); v != "" {
		if db, err := strconv.Atoi(v); err == nil {
			cfg.Redis.DB = db
		}
	}
	if v := os.Getenv("HNSX_REDIS_QUEUE_PREFIX"); v != "" {
		cfg.Redis.QueueKeyPrefix = v
	}
}

// Validate enforces structural invariants.
func (c *Config) Validate() error {
	if c.HTTPAddr == "" {
		return errors.New("config.http_addr is required")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		return errors.New("config.database_url is required")
	}
	switch c.OTel.Exporter {
	case "stdout", "otlp", "none", "":
		// ok
	default:
		return fmt.Errorf("config.otel.exporter %q is invalid (allowed: stdout, otlp, none)", c.OTel.Exporter)
	}
	if c.OTel.Exporter == "otlp" && c.OTel.OTLPEndpoint == "" {
		return errors.New("config.otel.otlp_endpoint is required when exporter is otlp")
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error", "":
		// ok
	default:
		return fmt.Errorf("config.log.level %q is invalid (allowed: debug, info, warn, error)", c.Log.Level)
	}
	if c.MigrationsDir != "" {
		// Resolve to absolute to be friendly to callers that chdir.
		if abs, err := filepath.Abs(c.MigrationsDir); err == nil {
			c.MigrationsDir = abs
		}
	}
	return nil
}

// PostgresEnabled reports whether a database connection is configured.
func (c *Config) PostgresEnabled() bool { return strings.TrimSpace(c.DatabaseURL) != "" }

// RedisEnabled reports whether a Redis connection is configured for the
// session scheduling queue.
func (c *Config) RedisEnabled() bool { return strings.TrimSpace(c.Redis.Addr) != "" }

// MigrationsToRun returns the duration budget for running migrations on
// startup. Currently a single shot; future version may retry.
func (c *Config) MigrationsToRun() time.Duration { return 30 * time.Second }
