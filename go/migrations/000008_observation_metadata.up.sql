-- +goose Up
-- +goose StatementBegin

ALTER TABLE observations ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_observations_metadata ON observations USING GIN (metadata);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_observations_metadata;
ALTER TABLE observations DROP COLUMN IF EXISTS metadata;

-- +goose StatementEnd
