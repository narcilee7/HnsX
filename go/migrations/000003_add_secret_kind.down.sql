-- +goose Down
-- +goose StatementBegin

ALTER TABLE secrets DROP COLUMN IF EXISTS kind;

-- +goose StatementEnd
