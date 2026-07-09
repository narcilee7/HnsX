-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    role VARCHAR(64) NOT NULL DEFAULT 'developer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, email)
);

CREATE TABLE IF NOT EXISTS domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain_id VARCHAR(255) NOT NULL,
    current_version VARCHAR(64) NOT NULL,
    description TEXT,
    status VARCHAR(64) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, domain_id)
);

CREATE TABLE IF NOT EXISTS domain_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain_uuid UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    version VARCHAR(64) NOT NULL,
    yaml_body TEXT NOT NULL,
    json_body JSONB,
    harness_hash VARCHAR(64) NOT NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, domain_uuid, version)
);

CREATE TABLE IF NOT EXISTS skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    skill_id VARCHAR(255) NOT NULL,
    current_version VARCHAR(64) NOT NULL,
    description TEXT,
    status VARCHAR(64) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, skill_id)
);

CREATE TABLE IF NOT EXISTS skill_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    skill_uuid UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    version VARCHAR(64) NOT NULL,
    yaml_body TEXT NOT NULL,
    json_body JSONB,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, skill_uuid, version)
);

CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_id VARCHAR(255) NOT NULL,
    domain_uuid UUID NOT NULL REFERENCES domains(id),
    domain_version VARCHAR(64) NOT NULL,
    orchestration VARCHAR(64) NOT NULL,
    state VARCHAR(64) NOT NULL DEFAULT 'pending',
    trigger_payload JSONB,
    result_payload JSONB,
    trace_id VARCHAR(255),
    runtime_id VARCHAR(255),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, session_id)
);

CREATE TABLE IF NOT EXISTS session_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_uuid UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    total_cost_usd DECIMAL(12, 6),
    total_prompt_tokens BIGINT,
    total_completion_tokens BIGINT,
    duration_ms BIGINT,
    agent_invocations INT,
    tool_invocations INT,
    policy_violations INT,
    human_approvals INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS eval_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    domain_uuid UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    set_id VARCHAR(255) NOT NULL,
    description TEXT,
    cases JSONB NOT NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, domain_uuid, set_id)
);

CREATE TABLE IF NOT EXISTS eval_cases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    eval_set_uuid UUID NOT NULL REFERENCES eval_sets(id) ON DELETE CASCADE,
    case_id VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    input JSONB NOT NULL,
    expect JSONB NOT NULL,
    scorer JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, eval_set_uuid, case_id)
);

CREATE TABLE IF NOT EXISTS eval_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    eval_set_uuid UUID NOT NULL REFERENCES eval_sets(id),
    domain_uuid UUID NOT NULL REFERENCES domains(id),
    domain_version VARCHAR(64) NOT NULL,
    orchestration VARCHAR(64) NOT NULL,
    state VARCHAR(64) NOT NULL DEFAULT 'running',
    score DECIMAL(5, 4),
    total_cases INT,
    passed_cases INT,
    total_cost_usd DECIMAL(12, 6),
    duration_ms BIGINT,
    baseline_run_uuid UUID REFERENCES eval_runs(id),
    report_url TEXT,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS eval_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    eval_run_uuid UUID NOT NULL REFERENCES eval_runs(id) ON DELETE CASCADE,
    case_uuid UUID NOT NULL REFERENCES eval_cases(id),
    session_uuid UUID REFERENCES sessions(id),
    score DECIMAL(5, 4),
    passed BOOLEAN NOT NULL DEFAULT FALSE,
    actual JSONB,
    details JSONB,
    duration_ms BIGINT,
    cost_usd DECIMAL(12, 6),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    policy_id VARCHAR(255) NOT NULL,
    domain_uuid UUID REFERENCES domains(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    rules JSONB NOT NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, policy_id)
);

CREATE TABLE IF NOT EXISTS secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    secret_id VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    description TEXT,
    last_used_at TIMESTAMPTZ,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, secret_id)
);

CREATE TABLE IF NOT EXISTS runtimes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    runtime_id VARCHAR(255) NOT NULL,
    version VARCHAR(64),
    region VARCHAR(128),
    capabilities JSONB,
    last_heartbeat_at TIMESTAMPTZ,
    status VARCHAR(64) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, runtime_id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_uuid UUID REFERENCES sessions(id),
    domain_uuid UUID REFERENCES domains(id),
    action VARCHAR(255) NOT NULL,
    actor VARCHAR(255) NOT NULL,
    actor_type VARCHAR(64) NOT NULL,
    resource VARCHAR(255),
    resource_type VARCHAR(64),
    decision VARCHAR(64),
    reason TEXT,
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_uuid UUID NOT NULL REFERENCES sessions(id),
    step_id VARCHAR(255),
    action VARCHAR(255) NOT NULL,
    resource VARCHAR(255),
    risk_level VARCHAR(64),
    context JSONB,
    status VARCHAR(64) NOT NULL DEFAULT 'pending',
    requested_by VARCHAR(255) NOT NULL,
    reviewed_by UUID REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    comment TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_domains_tenant_id ON domains(tenant_id);
CREATE INDEX IF NOT EXISTS idx_domains_domain_id ON domains(domain_id);
CREATE INDEX IF NOT EXISTS idx_domain_versions_domain_uuid ON domain_versions(domain_uuid);
CREATE INDEX IF NOT EXISTS idx_skills_tenant_id ON skills(tenant_id);
CREATE INDEX IF NOT EXISTS idx_skill_versions_skill_uuid ON skill_versions(skill_uuid);
CREATE INDEX IF NOT EXISTS idx_sessions_tenant_id ON sessions(tenant_id);
CREATE INDEX IF NOT EXISTS idx_sessions_domain_uuid ON sessions(domain_uuid);
CREATE INDEX IF NOT EXISTS idx_sessions_state ON sessions(state);
CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);
CREATE INDEX IF NOT EXISTS idx_session_summaries_session_uuid ON session_summaries(session_uuid);
CREATE INDEX IF NOT EXISTS idx_session_summaries_cost ON session_summaries(total_cost_usd);
CREATE INDEX IF NOT EXISTS idx_eval_sets_domain_uuid ON eval_sets(domain_uuid);
CREATE INDEX IF NOT EXISTS idx_eval_cases_eval_set_uuid ON eval_cases(eval_set_uuid);
CREATE INDEX IF NOT EXISTS idx_eval_runs_eval_set_uuid ON eval_runs(eval_set_uuid);
CREATE INDEX IF NOT EXISTS idx_eval_runs_domain_uuid ON eval_runs(domain_uuid);
CREATE INDEX IF NOT EXISTS idx_eval_results_eval_run_uuid ON eval_results(eval_run_uuid);
CREATE INDEX IF NOT EXISTS idx_eval_results_case_uuid ON eval_results(case_uuid);
CREATE INDEX IF NOT EXISTS idx_policies_domain_uuid ON policies(domain_uuid);
CREATE INDEX IF NOT EXISTS idx_secrets_tenant_id ON secrets(tenant_id);
CREATE INDEX IF NOT EXISTS idx_runtimes_status ON runtimes(status);
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_id ON audit_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_logs_session_uuid ON audit_logs(session_uuid);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor);
CREATE INDEX IF NOT EXISTS idx_approvals_tenant_id ON approvals(tenant_id);
CREATE INDEX IF NOT EXISTS idx_approvals_session_uuid ON approvals(session_uuid);
CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);
CREATE INDEX IF NOT EXISTS idx_approvals_expires_at ON approvals(expires_at);

-- Default tenant
INSERT INTO tenants (id, name, slug)
VALUES ('00000000-0000-0000-0000-000000000000', 'default', 'default')
ON CONFLICT (slug) DO NOTHING;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS approvals;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS runtimes;
DROP TABLE IF EXISTS secrets;
DROP TABLE IF EXISTS policies;
DROP TABLE IF EXISTS eval_results;
DROP TABLE IF EXISTS eval_runs;
DROP TABLE IF EXISTS eval_cases;
DROP TABLE IF EXISTS eval_sets;
DROP TABLE IF EXISTS session_summaries;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS skill_versions;
DROP TABLE IF EXISTS skills;
DROP TABLE IF EXISTS domain_versions;
DROP TABLE IF EXISTS domains;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;

-- +goose StatementEnd
