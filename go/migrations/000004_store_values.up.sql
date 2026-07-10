-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS store_values (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    namespace VARCHAR(64) NOT NULL,
    key VARCHAR(1024) NOT NULL,
    value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(namespace, key)
);

CREATE INDEX IF NOT EXISTS idx_store_values_namespace_key ON store_values(namespace, key);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS store_values;

-- +goose StatementEnd
