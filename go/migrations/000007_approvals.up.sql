-- +goose Up
-- +goose StatementBegin

-- Approval records track the human-in-the-loop gate. Each row is
-- either pending (session suspended waiting on a human) or resolved
-- (approved / rejected / expired) with reviewer + comment.
CREATE TABLE IF NOT EXISTS approvals (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    approval_id  VARCHAR(128) NOT NULL,
    session_id   VARCHAR(128) NOT NULL,
    domain_id    VARCHAR(255),
    action       VARCHAR(255) NOT NULL,
    resource     VARCHAR(255),
    risk_level   VARCHAR(32),
    context      JSONB,
    status       VARCHAR(32) NOT NULL DEFAULT 'pending',
    requested_by VARCHAR(255),
    reviewed_by  VARCHAR(255),
    comment      TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at  TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_approvals_tenant_id
    ON approvals (tenant_id, approval_id);
CREATE INDEX IF NOT EXISTS idx_approvals_session
    ON approvals (session_id, status);
CREATE INDEX IF NOT EXISTS idx_approvals_domain
    ON approvals (domain_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_approvals_status
    ON approvals (tenant_id, status, created_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_approvals_status;
DROP INDEX IF EXISTS idx_approvals_domain;
DROP INDEX IF EXISTS idx_approvals_session;
DROP INDEX IF EXISTS idx_approvals_tenant_id;
DROP TABLE IF EXISTS approvals;

-- +goose StatementEnd
