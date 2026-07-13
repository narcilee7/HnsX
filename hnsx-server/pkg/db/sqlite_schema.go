package db

import "fmt"

// SQLite schema — Domain CRUD only.
//
// The existing /go/migrations goose set targets Postgres-specific syntax
// (gen_random_uuid, JSONB, TIMESTAMPTZ, UUID type). Porting all 8 files is a
// significant undertaking tracked for v1.1. This file ships the minimum
// schema needed to make the daemon's headline use case — register a domain
// from Python / Console / CLI, then list / get it — actually work end to
// end over embedded SQLite.
//
// Other repos (audit, eval, session, trace, secret, worker) continue to
// require Postgres. When SQLite mode is on, those endpoints surface a
// "schema missing" error at first call — by design for v1.0. We document
// this in the README's daemon-mode section so users pick Postgres when
// they need the full surface, or SQLite when they're prototyping the
// Domain CRUD path.

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS domains (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain_id       TEXT NOT NULL,
    current_version TEXT NOT NULL,
    description     TEXT,
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    UNIQUE(tenant_id, domain_id)
);

CREATE INDEX IF NOT EXISTS idx_domains_tenant_id ON domains(tenant_id);
CREATE INDEX IF NOT EXISTS idx_domains_domain_id ON domains(domain_id);

CREATE TABLE IF NOT EXISTS domain_versions (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain_uuid   TEXT NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    version       TEXT NOT NULL,
    yaml_body     TEXT NOT NULL,
    json_body     TEXT,
    harness_hash  TEXT NOT NULL DEFAULT '',
    created_by    TEXT,
    created_at    TEXT NOT NULL,
    UNIQUE(tenant_id, domain_uuid, version)
);

CREATE INDEX IF NOT EXISTS idx_domain_versions_domain_uuid ON domain_versions(domain_uuid);
`

// EnsureSQLiteSchema applies the SQLite-only minimum schema. Idempotent
// (every CREATE uses IF NOT EXISTS). Called by application bootstrap when
// the SQLite backend is active.
//
// We use a single transactional exec rather than goose because the
// PostgreSQL-specific features of the existing goose set don't translate
// cleanly; a hand-curated SQLite DDL keeps the surface explicit.
func EnsureSQLiteSchema(database *DB) error {
	if database == nil {
		return nil
	}
	if !database.IsSQLite() {
		return nil
	}
	if database.SQL == nil {
		return nil
	}
	if _, err := database.SQL.Exec(sqliteSchema); err != nil {
		return fmt.Errorf("db.EnsureSQLiteSchema: %w", err)
	}
	// Seed the default tenant so Domain CRUD works out of the box. The
	// default tenant ID is defined as tenant.DefaultID in the application
	// code; the row here lets `INSERT INTO domains (tenant_id, ...)`
	// succeed without the caller having to know about tenants.
	seedDefaultTenant := `INSERT OR IGNORE INTO tenants (id, name, slug, created_at, updated_at)
        VALUES ('00000000-0000-0000-0000-000000000000', 'default', 'default', datetime('now'), datetime('now'))`
	if _, err := database.SQL.Exec(seedDefaultTenant); err != nil {
		return fmt.Errorf("db.EnsureSQLiteSchema: seed default tenant: %w", err)
	}
	return nil
}