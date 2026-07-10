package config

import (
	"os"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.HTTPAddr == "" {
		t.Fatal("default http addr missing")
	}
	if c.MigrationsDir == "" {
		t.Fatal("default migrations dir missing")
	}
}

func TestValidate_OK(t *testing.T) {
	c := Default()
	c.DatabaseURL = "postgres://hnsx:hnsx@127.0.0.1:5432/hnsx?sslmode=disable"
	c.OTel.Exporter = "stdout"
	c.Log.Level = "info"
	if err := c.Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}
}

func TestValidate_RequiresDatabaseURL(t *testing.T) {
	c := Default()
	c.DatabaseURL = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected database_url required")
	}
}

func TestValidate_RejectsBadExporter(t *testing.T) {
	c := Default()
	c.DatabaseURL = "postgres://x"
	c.OTel.Exporter = "ftp"
	if err := c.Validate(); err == nil {
		t.Fatal("expected exporter error")
	}
}

func TestValidate_RequiresOTLPEndpoint(t *testing.T) {
	c := Default()
	c.DatabaseURL = "postgres://x"
	c.OTel.Exporter = "otlp"
	c.OTel.OTLPEndpoint = ""
	if err := c.Validate(); err == nil {
		t.Fatal("expected OTLP endpoint required")
	}
}

func TestLoad_DefaultsWhenNoFile(t *testing.T) {
	t.Setenv("HNSX_DATABASE_URL", "postgres://hnsx:hnsx@127.0.0.1:5432/hnsx?sslmode=disable")
	c, err := Load("/nonexistent.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTPAddr != Default().HTTPAddr {
		t.Fatalf("addr = %q", c.HTTPAddr)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("HNSX_HTTP_ADDR", "127.0.0.1:9999")
	t.Setenv("HNSX_LOG_LEVEL", "debug")
	t.Setenv("HNSX_DATABASE_URL", "postgres://hnsx:hnsx@127.0.0.1:5432/hnsx?sslmode=disable")
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTPAddr != "127.0.0.1:9999" {
		t.Errorf("addr = %q", c.HTTPAddr)
	}
	if c.Log.Level != "debug" {
		t.Errorf("level = %q", c.Log.Level)
	}
}

func TestLoad_YAMLOverride(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/c.yaml"
	if err := os.WriteFile(path, []byte(`
http_addr: "127.0.0.1:7777"
database_url: "postgres://x"
log:
  level: warn
`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTPAddr != "127.0.0.1:7777" {
		t.Errorf("addr = %q", c.HTTPAddr)
	}
	if c.Log.Level != "warn" {
		t.Errorf("level = %q", c.Log.Level)
	}
}

func TestPostgresEnabled(t *testing.T) {
	c := Default()
	if c.PostgresEnabled() {
		t.Fatal("default should be DB-less")
	}
	c.DatabaseURL = "postgres://x"
	if !c.PostgresEnabled() {
		t.Fatal("DSN set should enable postgres")
	}
}
