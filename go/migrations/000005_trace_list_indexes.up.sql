-- +goose Up
-- +goose StatementBegin

-- Compress the ListSummaries hot path. The per-trace GROUP BY in
-- internal/trace/repository/postgres.go::ListSummaries filters by
-- (domain_id, agent_id, created_at range). Without this composite index,
-- Postgres falls back to idx_observations_session_id which never matches
-- for domain-only queries, and the planner switches to a sequential scan
-- once the table grows past a few hundred thousand rows.
CREATE INDEX IF NOT EXISTS idx_observations_domain_id_created_at
    ON observations (domain_id, created_at);
CREATE INDEX IF NOT EXISTS idx_observations_agent_id_created_at
    ON observations (agent_id, created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_observations_agent_id_created_at;
DROP INDEX IF EXISTS idx_observations_domain_id_created_at;

-- +goose StatementEnd
