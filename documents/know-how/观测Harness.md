# 我们如何观测 Harness 与 Agent

> 观测是 Harness 的眼睛。没有细粒度观测，约束无法生效、评估无法量化、系统无法进化。

---

## 1. 观测目标

HnsX 的观测体系要回答：

| 问题 | 观测目的 |
|---|---|
| 这次 Session 发生了什么？ | 可审计、可复盘 |
| Agent 调用了什么？ | 工具使用、成本、延迟 |
| Harness 在哪里介入了？ | Policy、Sandbox、Transition |
| 为什么路由到这个 Agent？ | 可解释性 |
| 是否触发了安全策略？ | 审计与合规 |
| 这次运行花了多少钱？ | 成本控制 |
| 系统整体健康度如何？ | 监控与告警 |

---

## 2. 核心概念

| 概念 | 定义 |
|---|---|
| **Observation** | 一次可被审计的事件，是观测的最小单位。 |
| **Trace** | 一个 Session 的所有 Observation 组成的有序序列。 |
| **Span** | 一次有边界的操作，例如一个 Agent turn、一个 Tool call、一个 Transition。 |
| **Metric** | 聚合后的数值指标，例如成本、延迟、token 数。 |
| **AuditLog** | 不可变的操作日志，用于安全与合规。 |
| **Telemetry** | Observation / Trace / Metric / AuditLog 的统称。 |

关系：

```text
Telemetry
├── Observation（事件）
├── Trace（Observation 序列）
├── Metric（聚合数值）
└── AuditLog（合规日志）
```

---

## 3. Observation 模型

### 3.1 核心字段

```yaml
observation:
  observation_id: "obs-001"
  trace_id: "trace-001"
  session_id: "sess-001"
  domain_id: "customer-service"
  domain_version: "0.1.0"
  step_id: "triage"
  agent_id: "triage"
  parent_id: "obs-000"      # 父 observation，支持嵌套
  kind: "text"              # 事件类型
  role: "assistant"         # user / assistant / tool / system
  payload: {}               # 类型特定的内容
  metadata:
    model: "claude-sonnet-4"
    provider: "anthropic"
    prompt_tokens: 120
    completion_tokens: 80
    cost_usd: 0.002
    latency_ms: 850
  created_at: "2026-07-08T12:00:00Z"
```

### 3.2 Observation 类型

| Kind | 说明 | Payload 示例 |
|---|---|---|
| `session_start` | Session 开始 | `{ trigger: {...} }` |
| `session_end` | Session 结束 | `{ state: "completed", result: {...} }` |
| `step_start` | Step 开始 | `{ step_id: "triage" }` |
| `step_end` | Step 结束 | `{ step_id: "triage", output: {...} }` |
| `turn_start` | Turn 开始 | `{ role: "user", content: "..." }` |
| `turn_end` | Turn 结束 | `{ role: "assistant", content: "..." }` |
| `text` | Agent 文本输出 | `{ content: "..." }` |
| `tool_call` | Tool 调用请求 | `{ tool_id: "search", arguments: {...} }` |
| `tool_result` | Tool 返回结果 | `{ tool_id: "search", result: {...} }` |
| `routing_decision` | 路由决策 | `{ intent: "billing", confidence: 0.95 }` |
| `transition` | Step 间转移 | `{ from: "triage", to: "billing", reason: "..." }` |
| `policy_check` | Policy 检查 | `{ rule: "budget", result: "pass" }` |
| `policy_violation` | Policy 违规 | `{ rule: "denied_tool", details: "..." }` |
| `human_approval` | 人工审批事件 | `{ approved: true, actor: "..." }` |
| `sandbox_event` | Sandbox 事件 | `{ backend: "container", action: "execute" }` |
| `cost` | 成本记录 | `{ prompt_tokens: 100, completion_tokens: 50, cost_usd: 0.01 }` |
| `error` | 错误事件 | `{ code: "...", message: "..." }` |
| `memory_read` / `memory_write` | Memory 操作 | `{ key: "...", value: "..." }` |

### 3.3 Observation 的嵌套关系

一个 Step 内部可以有多个 Turn，一个 Turn 内部可以有多个 Tool call：

```text
session_start
└── step_start (triage)
    ├── turn_start (user)
    ├── text (assistant: reasoning)
    ├── tool_call (search)
    ├── tool_result
    ├── text (assistant: routing_decision)
    └── step_end
└── transition (triage → billing)
└── step_start (billing)
    ├── turn_start
    ├── text (assistant: final answer)
    └── step_end
└── session_end
```

通过 `parent_id` 和 `trace_id` 可以重建完整树形结构。

---

## 4. Trace 模型

Trace 是一个 Session 的完整执行记录：

```yaml
trace:
  trace_id: "trace-001"
  session_id: "sess-001"
  domain_id: "customer-service"
  domain_version: "0.1.0"
  orchestration: "supervisor"
  status: "completed"
  started_at: "2026-07-08T12:00:00Z"
  completed_at: "2026-07-08T12:00:05Z"
  duration_ms: 5000
  total_cost_usd: 0.05
  observations:
    - { ... }
    - { ... }
```

Trace 必须：

- 不可变
- 完整记录所有 Observation
- 支持按 session_id / domain_id / time range 查询
- 支持导出为 OTLP / OpenTelemetry 格式

---

## 5. Metric 模型

### 5.1 会话级指标

| 指标 | 类型 | 说明 |
|---|---|---|
| `hnsx_session_total` | counter | Session 总数 |
| `hnsx_session_duration_ms` | histogram | Session 耗时 |
| `hnsx_session_cost_usd` | histogram | Session 成本 |
| `hnsx_session_state` | counter | 按 state 分类的 Session 数 |

### 5.2 Agent 级指标

| 指标 | 类型 | 说明 |
|---|---|---|
| `hnsx_agent_invocations_total` | counter | Agent 调用次数 |
| `hnsx_agent_duration_ms` | histogram | Agent 调用耗时 |
| `hnsx_agent_prompt_tokens` | counter | prompt token 数 |
| `hnsx_agent_completion_tokens` | counter | completion token 数 |
| `hnsx_agent_cost_usd` | counter | Agent 调用成本 |

### 5.3 Tool 级指标

| 指标 | 类型 | 说明 |
|---|---|---|
| `hnsx_tool_invocations_total` | counter | Tool 调用次数 |
| `hnsx_tool_duration_ms` | histogram | Tool 调用耗时 |
| `hnsx_tool_errors_total` | counter | Tool 错误数 |

### 5.4 Policy 级指标

| 指标 | 类型 | 说明 |
|---|---|---|
| `hnsx_policy_checks_total` | counter | Policy 检查次数 |
| `hnsx_policy_violations_total` | counter | Policy 违规次数 |
| `hnsx_human_approvals_total` | counter | 人工审批次数 |

---

## 6. AuditLog 模型

AuditLog 是不可变的安全/合规日志，与 Trace 分离：

```yaml
audit_record:
  record_id: "audit-001"
  timestamp: "2026-07-08T12:00:00Z"
  session_id: "sess-001"
  domain_id: "customer-service"
  action: "tool_call"
  actor: "agent:billing"
  resource: "tool:shell"
  decision: "denied"
  reason: "shell is in denied_tools list"
  details: {}
```

审计事件包括：

- 敏感 Tool 调用
- Policy 违规
- Human approval/rejection
- Domain 注册/更新/删除
- Secret 访问
- 成本超限

---

## 7. 数据采集架构

```text
Runtime Worker
│
├─► Observation Collector（内存缓冲）
│       │
│       ▼
│   Telemetry Processor（添加 metadata、采样、聚合）
│       │
│       ▼
│   Telemetry Router
│       │
│       ├──► OTLP Exporter ──▶ Tempo / Jaeger / Collector
│       ├──► Metric Exporter ──▶ Prometheus
│       ├──► Audit Exporter ──▶ PostgreSQL / SIEM
│       └──► Local Sink ──▶ SQLite / jsonl / stdout
│
└─► Console / SDK 通过 SSE 实时订阅 Observation
```

### 7.1 采集原则

- **低开销**：Observation 收集不能显著影响运行时性能。
- **不丢数据**：关键审计事件必须可靠投递。
- **可采样**：高频率事件可以采样，审计事件不采样。
- **实时性**：Console 通过 SSE 实时看到 Observation。

---

## 8. 与 OpenTelemetry 集成

HnsX 优先使用 OpenTelemetry 作为观测协议：

- **Trace** → OTLP Trace → Tempo / Jaeger
- **Metric** → OTLP Metric 或 Prometheus Exporter
- **Log** → OTLP Log

映射关系：

| HnsX 概念 | OTel 概念 |
|---|---|
| Session | Trace |
| Step / Turn | Span |
| Observation | SpanEvent / LogRecord |
| Metric | Metric |
| AuditLog | LogRecord（不可变） |

这样可以直接复用 Grafana、Tempo、Jaeger、Prometheus 等开源工具。

---

## 9. 与 Harness 的集成点

### 9.1 采集点

| 采集点 | 产生 Observation |
|---|---|
| Session 创建 | `session_start` |
| Step 开始/结束 | `step_start` / `step_end` |
| Turn 开始/结束 | `turn_start` / `turn_end` |
| Agent 产生输出 | `text` / `routing_decision` |
| Tool 调用 | `tool_call` / `tool_result` |
| Transition 触发 | `transition` |
| Policy 检查 | `policy_check` / `policy_violation` |
| Sandbox 执行 | `sandbox_event` |
| Human 审批 | `human_approval` |
| Memory 操作 | `memory_read` / `memory_write` |
| 错误 | `error` |
| Session 结束 | `session_end` |

### 9.2 上下文注入

每个 Observation 自动携带：

- trace_id / session_id / domain_id
- step_id / agent_id
- 当前 orchestration 模式
- Domain 版本
- 时间戳

### 9.3 与 Policy 联动

Policy 检查本身产生 Observation，违规事件同时进入 AuditLog。

### 9.4 与 Eval 联动

Eval 直接消费 Observation 序列进行断言。

---

## 10. Console 展示

### 10.1 Session 实时视图

- 时间线展示 Observation 序列
- 嵌套展开 Step / Turn / Tool call
- 实时 SSE 推送

### 10.2 Trace 查询

- 按 domain / session / time range 查询
- 嵌入 Grafana / Tempo UI
- 支持导出 JSON

### 10.3 Metric 大盘

- 成本趋势
- Agent 调用次数
- Policy 违规次数
- Session 成功率

### 10.4 AuditLog 视图

- 不可变记录列表
- 按 action / actor / decision 过滤
- 合规报告导出

---

## 11. 存储策略

| 数据类型 | 本地 | 团队 | SaaS |
|---|---|---|---|
| Observation | SQLite / jsonl | PostgreSQL | PostgreSQL + 对象存储 |
| Trace | SQLite / jsonl | Tempo / Jaeger | Tempo / 对象存储 |
| Metric | stdout / Prometheus | Prometheus / Grafana | OTLP Collector |
| AuditLog | SQLite | PostgreSQL | 合规存储 / SIEM |

---

## 12. 实现路线图

### Phase 1：基础观测

- Observation 数据结构
- 内存 collector + stdout sink
- Session / Step / Turn / Tool call 基础事件
- `hnsx traces` CLI 查询本地 SQLite

### Phase 2：标准协议

- OTLP exporter
- Tempo / Grafana 集成
- Metric 聚合与导出

### Phase 3：审计与安全

- AuditLog 模型
- Policy 违规审计
- 不可变存储

### Phase 4：实时与可视化

- SSE 实时推送
- Console Trace 视图
- Metric 大盘

---

## 13. 设计原则

1. **全面观测**：每个 token、每次 tool call、每次 transition、每次 policy 检查都要被记录。
2. **结构清晰**：Observation 类型明确，支持嵌套关系。
3. **标准协议**：优先 OpenTelemetry，不造私有协议。
4. **审计独立**：AuditLog 与 Trace 分离，满足合规要求。
5. **成本可控**：采样与缓冲设计，不影响运行时性能。
6. **实时可用**：Console 能实时看到 Session 执行。

---

## 14. 示例 Observation 序列

```yaml
session_id: "sess-001"
domain_id: "customer-service"
observations:
  - kind: session_start
    payload:
      trigger:
        question: "Why was I charged twice?"

  - kind: step_start
    step_id: triage
    agent_id: triage

  - kind: turn_start
    role: user
    payload:
      content: "Why was I charged twice?"

  - kind: text
    agent_id: triage
    payload:
      content: "I'll help you check your billing issue."

  - kind: routing_decision
    agent_id: triage
    payload:
      intent: billing
      confidence: 0.97

  - kind: step_end
    step_id: triage

  - kind: transition
    payload:
      from: triage
      to: billing
      reason: "routing_decision.intent == billing"

  - kind: step_start
    step_id: billing
    agent_id: billing

  - kind: text
    agent_id: billing
    payload:
      content: "I see two charges on July 1. The second one is a pending authorization..."

  - kind: step_end
    step_id: billing

  - kind: session_end
    payload:
      state: completed
      total_cost_usd: 0.05
```
