-- +goose Up
-- +goose StatementBegin

ALTER TABLE secrets ADD COLUMN IF NOT EXISTS kind VARCHAR(64) DEFAULT 'generic';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE secrets DROP COLUMN IF EXISTS kind;

-- +goose StatementEnd
