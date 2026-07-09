-- +goose Up
-- +goose StatementBegin

-- Lightweight local-mode observation sink. When OTel is unavailable or
-- undesirable, the runtime can persist the same observations into this
-- table for later querying / replay. Schema mirrors the canonical
-- Observation model in pkg/runtime (kind / session_id / step_id /
-- agent_id / payload / cost_usd / trace_id).
CREATE TABLE IF NOT EXISTS observations (
    id           BIGSERIAL PRIMARY KEY,
    trace_id     VARCHAR(64),
    session_id   VARCHAR(128) NOT NULL,
    domain_id    VARCHAR(255),
    domain_version VARCHAR(64),
    step_id      VARCHAR(255),
    agent_id     VARCHAR(255),
    kind         VARCHAR(64)  NOT NULL,
    payload      JSONB,
    cost_usd     DECIMAL(12, 6),
    prompt_tokens     INTEGER,
    completion_tokens INTEGER,
    latency_ms   BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_observations_session_id
    ON observations (session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_observations_trace_id
    ON observations (trace_id);
CREATE INDEX IF NOT EXISTS idx_observations_kind
    ON observations (kind);
CREATE INDEX IF NOT EXISTS idx_observations_agent_id
    ON observations (agent_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_observations_agent_id;
DROP INDEX IF EXISTS idx_observations_kind;
DROP INDEX IF EXISTS idx_observations_trace_id;
DROP INDEX IF EXISTS idx_observations_session_id;
DROP TABLE IF EXISTS observations;

-- +goose StatementEnd
