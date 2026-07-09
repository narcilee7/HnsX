# API 设计

> HnsX 服务端对外提供两套接口：REST API 供 Console 和 SDK 使用，gRPC API 供 CLI 和内部服务使用。底层共享同一份 Protobuf 协议定义。

---

## 1. 设计原则

1. **协议优先**：`proto/hnsx/v1/` 是 API 的唯一真相源。
2. **REST 对外**：Console / SDK 通过 HTTP/REST + JSON 调用。
3. **gRPC 对内**：CLI / Runtime Worker 通过 gRPC 调用。
4. **实时推送**：Session Observation 通过 SSE 推送。
5. **资源命名**：使用名词复数，路径语义清晰。
6. **版本化**：API 路径前缀 `/api/v1/`。
7. **分页**：列表接口统一 `limit` / `offset`。
8. **错误统一**：HTTP status + code + message 的 JSON 错误体。

---

## 2. 错误响应

```json
{
  "code": "DOMAIN_NOT_FOUND",
  "message": "Domain 'customer-service' not found",
  "details": {
    "domain_id": "customer-service"
  }
}
```

| HTTP Status | 场景 |
|---|---|
| 200 | 成功 |
| 201 | 创建成功 |
| 204 | 删除成功，无返回体 |
| 400 | 请求参数错误 |
| 401 | 未认证 |
| 403 | 无权限 |
| 404 | 资源不存在 |
| 409 | 资源冲突（如重复 ID） |
| 422 | 业务校验失败 |
| 500 | 服务端错误 |

---

## 3. Domain API

### 3.1 列出 Domain

```http
GET /api/v1/domains?limit=20&offset=0
```

响应：

```json
{
  "items": [
    {
      "id": "customer-service",
      "version": "0.1.0",
      "description": "Routes customer questions.",
      "status": "active",
      "created_at": "2026-07-08T12:00:00Z",
      "updated_at": "2026-07-08T12:00:00Z"
    }
  ],
  "total": 1,
  "limit": 20,
  "offset": 0
}
```

### 3.2 获取 Domain

```http
GET /api/v1/domains/:id
```

响应：

```json
{
  "id": "customer-service",
  "version": "0.1.0",
  "description": "...",
  "harness": { ... },
  "eval": { ... },
  "status": "active",
  "created_at": "2026-07-08T12:00:00Z",
  "updated_at": "2026-07-08T12:00:00Z"
}
```

### 3.3 注册 Domain

```http
POST /api/v1/domains
Content-Type: application/json

{
  "id": "customer-service",
  "version": "0.1.0",
  "description": "...",
  "harness": { ... },
  "eval": { ... }
}
```

### 3.4 更新 Domain

```http
PUT /api/v1/domains/:id
Content-Type: application/json

{
  "version": "0.2.0",
  "description": "...",
  "harness": { ... }
}
```

### 3.5 删除 Domain

```http
DELETE /api/v1/domains/:id
```

### 3.6 获取 Domain 版本列表

```http
GET /api/v1/domains/:id/versions
```

### 3.7 获取 Domain 指定版本

```http
GET /api/v1/domains/:id/versions/:version
```

### 3.8 验证 Domain

```http
POST /api/v1/domains/:id/validate
Content-Type: application/json

{
  "harness": { ... }
}
```

---

## 4. Session API

### 4.1 列出 Session

```http
GET /api/v1/sessions?domain=customer-service&state=running&limit=20&offset=0
```

### 4.2 获取 Session

```http
GET /api/v1/sessions/:id
```

响应：

```json
{
  "id": "sess-001",
  "domain_id": "customer-service",
  "domain_version": "0.1.0",
  "orchestration": "supervisor",
  "state": "completed",
  "trigger": { "question": "Why was I charged twice?" },
  "result": { "response": "..." },
  "trace_id": "trace-001",
  "started_at": "2026-07-08T12:00:00Z",
  "completed_at": "2026-07-08T12:00:05Z",
  "summary": {
    "total_cost_usd": 0.05,
    "duration_ms": 5000,
    "agent_invocations": 2,
    "tool_invocations": 1
  }
}
```

### 4.3 触发 Session

```http
POST /api/v1/sessions
Content-Type: application/json

{
  "domain_id": "customer-service",
  "domain_version": "0.1.0",
  "trigger": { "question": "Why was I charged twice?" }
}
```

响应：

```json
{
  "id": "sess-001",
  "state": "running"
}
```

### 4.4 重跑 Session

```http
POST /api/v1/sessions/:id/rerun
```

### 4.5 取消 Session

```http
POST /api/v1/sessions/:id/cancel
```

### 4.6 获取 Session Trace

```http
GET /api/v1/sessions/:id/trace
```

响应：

```json
{
  "trace_id": "trace-001",
  "session_id": "sess-001",
  "observations": [ ... ]
}
```

### 4.7 实时订阅 Session 事件

```http
GET /api/v1/sessions/:id/events
Accept: text/event-stream
```

SSE 事件：

```
event: observation
data: {"kind":"text","agent_id":"billing","payload":{"content":"..."}}

event: state
data: {"state":"completed"}

event: done
data: {}
```

---

## 5. Trace API

### 5.1 查询 Trace

```http
GET /api/v1/traces?domain=customer-service&session=sess-001&from=2026-07-01&to=2026-07-08&limit=20
```

响应：

```json
{
  "items": [
    {
      "trace_id": "trace-001",
      "session_id": "sess-001",
      "domain_id": "customer-service",
      "status": "completed",
      "started_at": "2026-07-08T12:00:00Z",
      "duration_ms": 5000
    }
  ],
  "total": 1
}
```

### 5.2 获取 Trace 详情

```http
GET /api/v1/traces/:traceId
```

---

## 6. Eval API

### 6.1 列出 EvalSet

```http
GET /api/v1/domains/:id/evals
```

### 6.2 获取 EvalSet

```http
GET /api/v1/domains/:id/evals/:setId
```

### 6.3 创建 EvalSet

```http
POST /api/v1/domains/:id/evals
Content-Type: application/json

{
  "id": "routing-accuracy",
  "description": "...",
  "cases": [ ... ]
}
```

### 6.4 更新 EvalSet

```http
PUT /api/v1/domains/:id/evals/:setId
```

### 6.5 删除 EvalSet

```http
DELETE /api/v1/domains/:id/evals/:setId
```

### 6.6 运行 Eval

```http
POST /api/v1/domains/:id/evals/:setId/run
Content-Type: application/json

{
  "orchestration": "supervisor",
  "baseline_run_id": "eval-run-001"
}
```

响应：

```json
{
  "eval_run_id": "eval-run-002",
  "state": "running"
}
```

### 6.7 获取 EvalRun

```http
GET /api/v1/domains/:id/evals/:setId/runs/:runId
```

响应：

```json
{
  "id": "eval-run-002",
  "set_id": "routing-accuracy",
  "domain_id": "customer-service",
  "state": "completed",
  "score": 0.92,
  "total_cases": 50,
  "passed_cases": 46,
  "total_cost_usd": 1.5,
  "duration_ms": 120000,
  "baseline_run_id": "eval-run-001",
  "cases": [ ... ],
  "created_at": "2026-07-08T12:00:00Z"
}
```

### 6.8 列出 EvalRun

```http
GET /api/v1/domains/:id/evals/:setId/runs?limit=20&offset=0
```

---

## 7. Metric API

### 7.1 获取 Domain 指标

```http
GET /api/v1/metrics?domain=customer-service&from=2026-07-01&to=2026-07-08
```

响应：

```json
{
  "domain_id": "customer-service",
  "total_sessions": 150,
  "completed_sessions": 140,
  "failed_sessions": 10,
  "total_cost_usd": 12.5,
  "avg_duration_ms": 2500,
  "agent_invocations": 300,
  "tool_invocations": 120
}
```

### 7.2 获取 Agent 指标

```http
GET /api/v1/metrics/agents?domain=customer-service
```

---

## 8. Audit API

### 8.1 查询 AuditLog

```http
GET /api/v1/audit?action=tool_call&decision=deny&limit=100
```

响应：

```json
{
  "items": [
    {
      "id": "audit-001",
      "timestamp": "2026-07-08T12:00:00Z",
      "session_id": "sess-001",
      "domain_id": "customer-service",
      "action": "tool_call",
      "actor": "agent:billing",
      "resource": "tool:shell",
      "decision": "deny",
      "reason": "shell is in denied_tools list"
    }
  ],
  "total": 1
}
```

---

## 9. Approval API

### 9.1 列出待审批

```http
GET /api/v1/approvals?status=pending&limit=20
```

### 9.2 审批通过

```http
POST /api/v1/approvals/:id/approve
Content-Type: application/json

{
  "comment": "Approved by operator"
}
```

### 9.3 审批拒绝

```http
POST /api/v1/approvals/:id/reject
Content-Type: application/json

{
  "comment": "Too risky"
}
```

---

## 10. Runtime API

### 10.1 列出 Runtime Worker

```http
GET /api/v1/runtimes
```

### 10.2 获取 Runtime

```http
GET /api/v1/runtimes/:id
```

---

## 11. Secret API

### 11.1 列出 Secret

```http
GET /api/v1/secrets
```

### 11.2 创建 Secret

```http
POST /api/v1/secrets
Content-Type: application/json

{
  "id": "openai_api_key",
  "value": "sk-..."
}
```

### 11.3 更新 Secret

```http
PUT /api/v1/secrets/:id
```

### 11.4 删除 Secret

```http
DELETE /api/v1/secrets/:id
```

---

## 12. Policy API

### 12.1 列出 Policy

```http
GET /api/v1/policies
```

### 12.2 创建 Policy

```http
POST /api/v1/policies
Content-Type: application/json

{
  "id": "no-shell",
  "name": "Deny Shell Tool",
  "rules": {
    "denied_tools": ["shell"]
  }
}
```

### 12.3 绑定 Policy 到 Domain

```http
POST /api/v1/domains/:id/policies
Content-Type: application/json

{
  "policy_id": "no-shell"
}
```

---

## 13. gRPC 服务

CLI 和 Runtime Worker 使用 gRPC：

| 服务 | 说明 |
|---|---|
| `Registry` | Domain 注册/发现 |
| `Scheduler` | Session 调度 |
| `Discovery` | Runtime 发现 |
| `TelemetryService` | Trace / Metric 上报 |
| `EvalService` | Eval 触发/查询 |
| `Runtime` | 触发 Session 流式执行 |

定义见 `proto/hnsx/v1/control_plane.proto` 和 `proto/hnsx/v1/runtime.proto`。

---

## 14. 认证

当前阶段使用 API Key：

```http
Authorization: Bearer hnsx-api-key-xxx
```

未来支持：

- OAuth 2.0 / OIDC
- JWT
- mTLS（Runtime Worker 之间）

---

## 15. API 实现阶段

### Phase 1

- Domain CRUD
- Session trigger + SSE
- Trace query

### Phase 2

- Eval API
- Metric API
- Audit API

### Phase 3

- Approval API
- Runtime API
- Secret / Policy API

### Phase 4

- gRPC 服务完整实现
- 认证与权限

---

## 16. 与 Web Console 的对应

| Console 页面 | 主要 API |
|---|---|
| Dashboard | `GET /api/v1/metrics`, `GET /api/v1/sessions` |
| Domain 列表 | `GET /api/v1/domains` |
| Domain 详情 | `GET/PUT /api/v1/domains/:id`, `POST /api/v1/domains/:id/validate` |
| Session 列表 | `GET /api/v1/sessions` |
| Session 详情 | `GET /api/v1/sessions/:id`, `GET /api/v1/sessions/:id/events` |
| Trace 查询 | `GET /api/v1/traces` |
| Eval | `GET/POST /api/v1/domains/:id/evals/...` |
| Observability | Grafana 嵌入 |
| AuditLog | `GET /api/v1/audit` |
| Approvals | `GET/POST /api/v1/approvals` |
| Settings | Secret / Policy / Runtime API |
