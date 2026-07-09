# 数据库表设计

> HnsX 服务端使用 PostgreSQL 作为持久化存储。Trace 和 Metric 优先走 OpenTelemetry → Tempo / Prometheus，数据库只保留元数据、审计和聚合指标。

---

## 1. 设计原则

1. **元数据为主**：Domain、Session、EvalRun、AuditLog 等元数据存在 PostgreSQL。
2. **不存原始 Trace**：原始 Observation 序列通过 OTLP 送到 Tempo，数据库只存 Trace ID 和摘要。
3. **审计独立**：AuditLog 单独表，不可变，支持合规查询。
4. **版本化**：Domain、Skill 支持多版本，保留历史。
5. **多租户预留**：`tenant_id` 字段预留，当前阶段可固定为 `default`。
6. **索引优化**：按 `domain_id`、`session_id`、`created_at` 高频查询建立索引。

---

## 2. 表清单

| 表名 | 说明 |
|---|---|
| `tenants` | 租户信息 |
| `users` | 用户账号 |
| `domains` | Domain 注册信息 |
| `domain_versions` | Domain 历史版本 |
| `skills` | Skill 注册信息 |
| `skill_versions` | Skill 历史版本 |
| `sessions` | Session 元数据 |
| `session_summaries` | Session 聚合摘要 |
| `eval_sets` | EvalSet 定义 |
| `eval_cases` | EvalCase 定义 |
| `eval_runs` | 评测运行记录 |
| `eval_results` | 单个 Case 评测结果 |
| `policies` | Policy 配置 |
| `secrets` | Secret 存储（加密） |
| `runtimes` | Runtime Worker 注册 |
| `audit_logs` | 审计日志 |
| `approvals` | 人工审批记录 |

---

## 3. 详细表结构

### 3.1 tenants

```sql
CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### 3.2 users

```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    email VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    role VARCHAR(64) NOT NULL DEFAULT 'developer', -- admin, developer, operator, security
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, email)
);
```

### 3.3 domains

```sql
CREATE TABLE domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    domain_id VARCHAR(255) NOT NULL,               -- 用户定义的 domain id
    current_version VARCHAR(64) NOT NULL,
    description TEXT,
    status VARCHAR(64) NOT NULL DEFAULT 'active',  -- active, archived, deprecated
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, domain_id)
);

CREATE INDEX idx_domains_tenant_id ON domains(tenant_id);
CREATE INDEX idx_domains_domain_id ON domains(domain_id);
```

### 3.4 domain_versions

```sql
CREATE TABLE domain_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    domain_uuid UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    version VARCHAR(64) NOT NULL,
    yaml_body TEXT NOT NULL,
    json_body JSONB,
    harness_hash VARCHAR(64) NOT NULL,             -- sha256 of normalized harness
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, domain_uuid, version)
);

CREATE INDEX idx_domain_versions_domain_uuid ON domain_versions(domain_uuid);
```

### 3.5 skills

```sql
CREATE TABLE skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    skill_id VARCHAR(255) NOT NULL,
    current_version VARCHAR(64) NOT NULL,
    description TEXT,
    status VARCHAR(64) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, skill_id)
);

CREATE INDEX idx_skills_tenant_id ON skills(tenant_id);
```

### 3.6 skill_versions

```sql
CREATE TABLE skill_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    skill_uuid UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    version VARCHAR(64) NOT NULL,
    yaml_body TEXT NOT NULL,
    json_body JSONB,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, skill_uuid, version)
);

CREATE INDEX idx_skill_versions_skill_uuid ON skill_versions(skill_uuid);
```

### 3.7 sessions

```sql
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    session_id VARCHAR(255) NOT NULL,              -- 运行时的 session id
    domain_uuid UUID NOT NULL REFERENCES domains(id),
    domain_version VARCHAR(64) NOT NULL,
    orchestration VARCHAR(64) NOT NULL,            -- single, supervisor, workflow, autonomous
    state VARCHAR(64) NOT NULL DEFAULT 'pending',  -- pending, running, completed, failed, paused
    trigger_payload JSONB,
    result_payload JSONB,
    trace_id VARCHAR(255),                         -- 对应 Tempo trace id
    runtime_id VARCHAR(255),                       -- 执行该 session 的 runtime
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, session_id)
);

CREATE INDEX idx_sessions_tenant_id ON sessions(tenant_id);
CREATE INDEX idx_sessions_domain_uuid ON sessions(domain_uuid);
CREATE INDEX idx_sessions_state ON sessions(state);
CREATE INDEX idx_sessions_created_at ON sessions(created_at);
```

### 3.8 session_summaries

```sql
CREATE TABLE session_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
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

CREATE INDEX idx_session_summaries_session_uuid ON session_summaries(session_uuid);
CREATE INDEX idx_session_summaries_cost ON session_summaries(total_cost_usd);
```

### 3.9 eval_sets

```sql
CREATE TABLE eval_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    domain_uuid UUID NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    set_id VARCHAR(255) NOT NULL,
    description TEXT,
    cases JSONB NOT NULL,                          -- EvalCase 数组
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, domain_uuid, set_id)
);

CREATE INDEX idx_eval_sets_domain_uuid ON eval_sets(domain_uuid);
```

### 3.10 eval_cases

可选独立表，当 cases 数量大时拆分：

```sql
CREATE TABLE eval_cases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    eval_set_uuid UUID NOT NULL REFERENCES eval_sets(id) ON DELETE CASCADE,
    case_id VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    input JSONB NOT NULL,
    expect JSONB NOT NULL,
    scorer JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, eval_set_uuid, case_id)
);

CREATE INDEX idx_eval_cases_eval_set_uuid ON eval_cases(eval_set_uuid);
```

### 3.11 eval_runs

```sql
CREATE TABLE eval_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    eval_set_uuid UUID NOT NULL REFERENCES eval_sets(id),
    domain_uuid UUID NOT NULL REFERENCES domains(id),
    domain_version VARCHAR(64) NOT NULL,
    orchestration VARCHAR(64) NOT NULL,
    state VARCHAR(64) NOT NULL DEFAULT 'running',  -- running, completed, failed
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

CREATE INDEX idx_eval_runs_eval_set_uuid ON eval_runs(eval_set_uuid);
CREATE INDEX idx_eval_runs_domain_uuid ON eval_runs(domain_uuid);
CREATE INDEX idx_eval_runs_created_at ON eval_runs(created_at);
```

### 3.12 eval_results

```sql
CREATE TABLE eval_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
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

CREATE INDEX idx_eval_results_eval_run_uuid ON eval_results(eval_run_uuid);
CREATE INDEX idx_eval_results_case_uuid ON eval_results(case_uuid);
```

### 3.13 policies

```sql
CREATE TABLE policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    policy_id VARCHAR(255) NOT NULL,
    domain_uuid UUID REFERENCES domains(id) ON DELETE SET NULL, -- NULL 表示租户级 policy
    name VARCHAR(255) NOT NULL,
    rules JSONB NOT NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, policy_id)
);

CREATE INDEX idx_policies_domain_uuid ON policies(domain_uuid);
```

### 3.14 secrets

```sql
CREATE TABLE secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    secret_id VARCHAR(255) NOT NULL,               -- 配置中引用的名字，如 openai_api_key
    value TEXT NOT NULL,                           -- 加密存储
    description TEXT,
    last_used_at TIMESTAMPTZ,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, secret_id)
);

CREATE INDEX idx_secrets_tenant_id ON secrets(tenant_id);
```

### 3.15 runtimes

```sql
CREATE TABLE runtimes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    runtime_id VARCHAR(255) NOT NULL,
    version VARCHAR(64),
    region VARCHAR(128),
    capabilities JSONB,
    last_heartbeat_at TIMESTAMPTZ,
    status VARCHAR(64) NOT NULL DEFAULT 'active',  -- active, inactive
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, runtime_id)
);

CREATE INDEX idx_runtimes_status ON runtimes(status);
```

### 3.16 audit_logs

```sql
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_uuid UUID REFERENCES sessions(id),
    domain_uuid UUID REFERENCES domains(id),
    action VARCHAR(255) NOT NULL,
    actor VARCHAR(255) NOT NULL,
    actor_type VARCHAR(64) NOT NULL,               -- user, agent, system
    resource VARCHAR(255),
    resource_type VARCHAR(64),
    decision VARCHAR(64),                          -- allow, deny, approve, reject
    reason TEXT,
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_tenant_id ON audit_logs(tenant_id);
CREATE INDEX idx_audit_logs_timestamp ON audit_logs(timestamp);
CREATE INDEX idx_audit_logs_session_uuid ON audit_logs(session_uuid);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor);
```

### 3.17 approvals

```sql
CREATE TABLE approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    session_uuid UUID NOT NULL REFERENCES sessions(id),
    step_id VARCHAR(255),
    action VARCHAR(255) NOT NULL,
    resource VARCHAR(255),
    risk_level VARCHAR(64),                        -- low, medium, high, critical
    context JSONB,
    status VARCHAR(64) NOT NULL DEFAULT 'pending', -- pending, approved, rejected, expired
    requested_by VARCHAR(255) NOT NULL,
    reviewed_by UUID REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    comment TEXT,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_approvals_tenant_id ON approvals(tenant_id);
CREATE INDEX idx_approvals_session_uuid ON approvals(session_uuid);
CREATE INDEX idx_approvals_status ON approvals(status);
CREATE INDEX idx_approvals_expires_at ON approvals(expires_at);
```

---

## 4. ER 图

```text
tenants
├── users
├── domains
│   ├── domain_versions
│   ├── sessions
│   │   ├── session_summaries
│   │   ├── approvals
│   │   └── audit_logs
│   ├── eval_sets
│   │   ├── eval_cases
│   │   ├── eval_runs
│   │   │   └── eval_results
│   ├── policies
│   └── secrets
├── skills
│   └── skill_versions
├── runtimes
└── audit_logs
```

---

## 5. 关键查询示例

### 5.1 查询 Domain 最新版本

```sql
SELECT * FROM domain_versions
WHERE domain_uuid = ? AND version = (
    SELECT current_version FROM domains WHERE id = ?
);
```

### 5.2 查询 Domain 最近 Session

```sql
SELECT * FROM sessions
WHERE domain_uuid = ?
ORDER BY created_at DESC
LIMIT 20;
```

### 5.3 查询成本排行

```sql
SELECT domain_id, SUM(total_cost_usd) as cost
FROM sessions s
JOIN session_summaries ss ON s.id = ss.session_uuid
GROUP BY domain_id
ORDER BY cost DESC;
```

### 5.4 查询审计事件

```sql
SELECT * FROM audit_logs
WHERE tenant_id = ? AND action = 'tool_call' AND decision = 'deny'
ORDER BY timestamp DESC
LIMIT 100;
```

---

## 6. 存储策略

| 数据 | 存储位置 | 理由 |
|---|---|---|
| Domain YAML | PostgreSQL | 元数据，需要版本化和查询 |
| Session 元数据 | PostgreSQL | 列表查询、状态跟踪 |
| Observation 序列 | Tempo (via OTLP) | 大规模、时序、可视化 |
| Metric | Prometheus / Grafana | 聚合、告警 |
| AuditLog | PostgreSQL | 合规、不可变 |
| Secret | PostgreSQL（加密） | 安全注入 |
| EvalRun 报告 | PostgreSQL | 版本对比、回溯 |

---

## 7. 多租户说明

当前阶段：

- 所有表带 `tenant_id`，默认单租户 `default`。
- 查询必须带 `tenant_id` 过滤。
- Secret、Policy 支持租户级和 Domain 级。

未来 SaaS 阶段：

- 增加行级安全策略（RLS）。
- 敏感数据隔离存储。
- 按租户分库/分表可选。

---

## 8. 迁移策略

使用 `golang-migrate` 或类似工具管理 schema 版本：

```
migrations/
├── 000001_init_schema.up.sql
├── 000001_init_schema.down.sql
├── 000002_add_audit_indexes.up.sql
└── ...
```

启动 Control Plane 时自动执行未完成迁移。

---

## 9. 设计 checklist

- [ ] 表职责是否单一？
- [ ] 高频查询是否有索引？
- [ ] 多租户字段是否预留？
- [ ] 版本化是否支持？
- [ ] Secret 是否加密存储？
- [ ] AuditLog 是否不可变？
- [ ] Trace 是否 offload 到 Tempo？
- [ ] 是否有迁移方案？
