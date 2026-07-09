# V1.2 服务架构落地跟踪

> 周期：2026.07.09 起（架构设计与分阶段落地）  
> 北极星：让 HnsX 从 V1.1 的 noop/echo 骨架演进为**真实 Agent 可运行、可约束、可观测、可评估**的 Harness-as-a-Service。  
> 架构基线：`docs/server-design/service-architecture.md`

---

## 总体里程碑

| 里程碑 | 目标 | 预计周期 | 验收标准 |
|---|---|---|---|
| M1 | Go 侧 DDD 重构完成 | 1 周 | `internal/` 各领域 model/repository/service 就位，现有 `pkg/` 逻辑迁入 |
| M2 | 真实 Agent 跑通 | 1-2 周 | `customer-service` domain 通过 Anthropic/OpenAI API 完成一次真实会话 |
| M3 | Tool 治理层落地 | 1 周 | API Agent 能调用 http/sql/python tool；CLI Agent 行为被审计 |
| M4 | Supervisor 编排 | 1-2 周 | `claude-triage` domain 的 supervisor → specialist 路由可运行 |
| M5 | Policy + Sandbox + Store | 1-2 周 | budget/denied-tools/human-approval 生效；container sandbox 可用；store 支持 postgres |
| M6 | 持久化与可观测性 | 1-2 周 | session/observation/trace 持久化到 Postgres；OTLP 导出到 Tempo/Grafana |
| M7 | Eval 平台化 | 1-2 周 | EvalSet 驱动批量 session、自动打分、baseline 对比 |
| M8 | 部署与扩展 | 1 周 | Docker Compose 一键启动 server + postgres + worker + tempo/grafana |

---

## M1：Go 侧 DDD 重构

### 目标
把 `hnsx-server/pkg/` 的技术分层代码按 bounded context 迁移到 `hnsx-server/internal/`，为后续复杂领域功能打好基础。

### 任务清单

- [x] **domain 领域**
  - [x] 定义 `Domain`、`DomainSpec`、`Harness` 等 model
  - [x] 定义 `DomainRepository` 接口 + in-memory 实现
  - [x] 实现 `DomainService`：注册、校验、版本管理
  - [x] 实现 Postgres `domain.Repository`（表 `domains` + `domain_versions`）
  - [ ] 把 `api/domains.go` 中的注册/加载逻辑迁入 `app/commands`

- [x] **session 领域**
  - [x] 定义 `Session`、`SessionState` model
  - [x] 定义 `SessionRepository` 接口 + in-memory 实现（后续接 Postgres）
  - [x] 实现 `SessionService`：Trigger、Cancel、Rerun、状态流转
  - [x] 实现 Postgres `session.Repository`（表 `sessions`）
  - [ ] 把 `api/sessions.go` 中的 session 逻辑迁入 `app/commands`
  - [x] 把 `pkg/session/broadcaster.go` 迁入 `internal/session/broadcaster`

- [ ] **worker 领域**
  - [ ] 定义 `Worker`、`Capability`、`Assignment` model
  - [ ] 定义 `WorkerRepository` 接口 + in-memory 实现
  - [ ] 实现 `WorkerService`：Register、Heartbeat、Assign、Evict
  - [ ] 把 `pkg/worker/registry.go` / `queue.go` 迁入 `internal/worker`
  - [ ] 在 `internal/worker` 中推导 `SessionRequest.required_capabilities`

- [ ] **telemetry 领域**
  - [ ] 定义 `Observation`、`Trace`、`Metric`、`AuditRecord` model
  - [ ] 定义 `TelemetryRepository` 接口
  - [ ] 实现 `TelemetryService`：Record、Query、Export OTLP
  - [ ] 把 `pkg/telemetry` 的 sink 逻辑迁入 `internal/telemetry`

- [ ] **secret 领域**
  - [ ] 定义 `Secret`、`SecretRef` model
  - [ ] 定义 `SecretRepository` 接口 + in-memory 实现
  - [ ] 实现 `SecretService`：Resolve、Inject、AccessAudit

- [ ] **policy 领域**
  - [ ] 定义 `Policy`、`Rule`、`Decision` model
  - [ ] 实现 `PolicyService`：编译期校验 + 运行时决策 hook

- [ ] **app 层**
  - [ ] `app/commands`：TriggerSession、CancelSession、RunEval
  - [ ] `app/queries`：ListSessions、GetTrace、ListWorkers

- [ ] **infrastructure 层**
  - [ ] `infrastructure/postgres`：各领域 repository 的 Postgres 实现
  - [ ] `infrastructure/grpcserver`：WorkerService / SchedulerService
  - [ ] `infrastructure/httpserver`：REST API 路由
  - [ ] `infrastructure/otel`：OpenTelemetry 初始化

### 验收标准

- `go test ./internal/...` 通过。
- `scripts/smoke.sh` 仍然通过（API 兼容）。
- 没有循环依赖；每个领域可独立测试。

---

## M2：真实 Agent 跑通

### 目标
让 Python worker 真正调用 Anthropic / OpenAI API，而不是 noop/echo。

### 任务清单

- [x] **adapters/anthropic.py**
  - [x] 实现真实 HTTP 调用（支持 `claude-sonnet-4` 等模型）
  - [x] 支持流式返回（stream chunks 转成 observation）
  - [x] 支持 function calling / tool use
  - [ ] 记录 cost（prompt/completion tokens、latency）

- [ ] **adapters/openai.py**
  - [ ] 实现真实 HTTP 调用（支持 `gpt-4o` 等模型）
  - [ ] 支持流式返回
  - [ ] 支持 function calling
  - [ ] 记录 cost

- [ ] **session_executor.py**
  - [ ] 实现多 turn loop（user → assistant → tool → assistant ...）
  - [ ] 支持 max_turns 限制
  - [ ] 正确 emit `turn_start`、`turn_end`、`agent_text`、`agent_invoke` observation

- [ ] **DomainSpec 适配**
  - [ ] 更新 `example-domains/customer-service/domain.yaml`，使用真实 adapter
  - [ ] 确保 agent 配置（model、adapter、prompt）能被 Python runtime 正确解析

### 验收标准

- `make worker-test` 通过（新增 anthropic/openai 的单元测试，可用 mock）。
- 启动 server + worker，触发 `customer-service` session，能在控制台看到真实的 assistant 文本输出。
- 网络错误能正确产生 `error` observation，session 状态变为 `failed`。

---

## M3：Tool 治理层落地

### 目标
明确 Tool 层对 API Agent 和 CLI Agent 的不同定位，并实现最小可用集。

### 任务清单

- [ ] **tools/registry.py**
  - [ ] 定义 Tool 注册接口
  - [ ] 支持按 capability / type 查找 tool
  - [ ] 调用前走 Policy 检查

- [ ] **tools/http.py**
  - [ ] 实现 GET/POST/PUT/DELETE HTTP 调用
  - [ ] 支持 secret 注入（header / query / body）
  - [ ] 支持 timeout、retry

- [ ] **tools/sql.py**
  - [ ] 实现基于 SQLAlchemy / raw SQL 的数据库查询
  - [ ] 支持只读模式（默认）和写模式（需 policy 显式允许）

- [ ] **tools/python.py**
  - [ ] 在 sandbox 内执行一段 Python 代码
  - [ ] 输出捕获为 observation

- [ ] **CLI Agent 的 Tool 审计**
  - [ ] 对 Claude Code / Codex adapter，捕获其 shell/file/edit 行为
  - [ ] 把捕获的行为 emit 为 `tool_call` / `tool_result` observation
  - [ ] 通过 Policy 拦截 denied operations

### 验收标准

- API Agent 能通过 function calling 调用 http tool 并拿到结果。
- CLI Agent 执行 shell 命令时被记录到 observation，且 denied tool 会被 policy 拦截。
- `example-domains` 中至少有一个 domain 使用 tool。

---

## M4：Supervisor 编排

### 目标
实现 `supervisor` orchestration：supervisor agent 动态 dispatch specialist。

### 任务清单

- [ ] **harness/transition.py**
  - [ ] 实现 JMESPath 表达式求值
  - [ ] 支持 `$.observations[-1].output.intent == 'billing'` 等 transition

- [ ] **harness/runner.py**
  - [ ] 实现 supervisor loop
  - [ ] supervisor agent 产生 `routing_decision` observation
  - [ ] 根据 transition 规则切换到 specialist agent
  - [ ] specialist 完成后根据 exit 条件结束或返回 supervisor

- [ ] **session_executor.py**
  - [ ] 把 runner 接入，支持 `mode: supervisor`

- [ ] **DomainSpec 适配**
  - [ ] 更新 `example-domains/claude-triage/domain.yaml`
  - [ ] 定义 triage / billing / technical agents + transitions

### 验收标准

- `claude-triage` domain 跑通：用户问题 → triage 路由 → specialist 回答。
- routing_decision observation 能被 Eval 消费。
- 路由错误时 session 标记为 `failed` 并给出原因。

---

## M5：Policy + Sandbox + Store

### 目标
让运行时约束真正生效，并统一存储抽象。

### 任务清单

- [ ] **policy/engine.py**
  - [ ] 实现 budget 检查（累计 cost / max_cost）
  - [ ] 实现 allowed/denied tools 检查
  - [ ] 实现 human_approval 触发（暂停 session，等待审批）
  - [ ] 每次检查产生 `policy_check` / `policy_violation` observation

- [ ] **sandbox/backend.py**
  - [ ] 定义 SandboxBackend 接口
  - [ ] 实现 `none` backend
  - [ ] 实现 `process` backend（Linux namespace + seccomp，可选）
  - [ ] 实现 `container` backend（Docker / Podman）

- [ ] **store/backend.py**
  - [ ] 定义 StoreBackend 接口
  - [ ] 实现 `in_memory` backend
  - [ ] 实现 `postgres` backend（context + knowledge）
  - [ ] 实现 `redis` backend（ephemeral）

- [ ] **Secret 注入**
  - [ ] Control Plane 解析 `{secret.XXX}` 并注入 SessionRequest
  - [ ] Python runtime 通过环境变量接收 secrets
  - [ ] 审计 secret 访问

### 验收标准

- 超预算 session 被暂停或失败。
- denied tool 调用被 policy 拦截。
- human_approval 触发后 session 进入 `paused`，审批后继续。
- container sandbox 能限制 tool 执行环境。
- store 支持跨 turn 的 context 传递。

---

## M6：持久化与可观测性

### 目标
把 observation、session、trace 持久化到 Postgres，并接入 OTLP。

### 任务清单

- [ ] **Postgres schema**
  - [ ] `sessions` 表
  - [ ] `observations` 表
  - [ ] `traces` 表（或 observation 聚合视图）
  - [ ] `eval_runs` 表
  - [ ] `audit_log` 表
  - [ ] `workers` 表

- [ ] **Go repository 实现**
  - [ ] `internal/session/repository/postgres`
  - [ ] `internal/telemetry/repository/postgres`
  - [ ] `internal/worker/repository/postgres`

- [ ] **OTLP exporter**
  - [ ] 把 observation 转成 OTel span / event
  - [ ] 支持 OTLP gRPC export
  - [ ] 本地开发 fallback 到 stdout / jsonl

- [ ] **Grafana / Tempo 集成**
  - [ ] 提供默认 dashboard JSON
  - [ ] Console Observability 页面嵌入 Grafana

### 验收标准

- session 重启后仍能查询历史 observation。
- `/api/v1/traces` 返回持久化的 trace 数据。
- Tempo 能查询到 session trace。

---

## M7：Eval 平台化

### 目标
实现 EvalSet 驱动的批量评测、自动打分、baseline 对比。

### 任务清单

- [ ] **Go evaluation 领域**
  - [ ] `EvalSet`、`EvalCase`、`EvalRun` model
  - [ ] `EvalRepository` + Postgres 实现
  - [ ] `EvalService`：批量触发、收集结果、汇总分数

- [ ] **Python scorer**
  - [ ] `exact`、`contains`、`jmespath`、`structured_match`
  - [ ] `llm-judge`（调用低成本 LLM）

- [ ] **Eval API**
  - [ ] `POST /api/v1/domains/:id/evals/:setId/run`
  - [ ] `GET /api/v1/domains/:id/evals/:setId/runs/:runId`

- [ ] **Baseline 对比**
  - [ ] EvalRun 关联 baseline_run_id
  - [ ] 报告展示 case 级别 Δ

### 验收标准

- 运行 eval 后产生 EvalRun 报告。
- 报告包含总分、case 明细、baseline 对比。
- CI 能通过 `hnsx eval` 做质量门禁。

---

## M8：部署与扩展

### 目标
提供本地开发一键启动和生产扩展路径。

### 任务清单

- [ ] **Docker Compose**
  - [ ] `deployments/local/docker-compose.yml` 包含 server / postgres / worker / tempo / grafana
  - [ ] worker 自动注册到 server

- [ ] **Helm chart（可选）**
  - [ ] server Deployment + Service
  - [ ] worker Deployment（可水平扩展 replicas）
  - [ ] postgres / redis / tempo 依赖

- [ ] **mTLS**
  - [ ] worker ↔ server gRPC 支持 TLS / mTLS

### 验收标准

- `docker compose up` 后控制台能触发 session 并看到 observation。
- 增加 worker replica 后 session 能分发到多个 worker。

---

## 跨里程碑依赖

```text
M1 (DDD) ──▶ M2 (真实 Agent) ──▶ M3 (Tool) ──▶ M4 (Supervisor)
  │              │                  │               │
  ▼              ▼                  ▼               ▼
M6 (持久化) ◀── M5 (Policy/Sandbox/Store) ◀──────────┘
  │
  ▼
M7 (Eval)
  │
  ▼
M8 (部署)
```

---

## 本周（2026.07.09）重点

1. ✅ 评审并定稿 `docs/server-design/service-architecture.md`。
2. ✅ 启动 **M1 Go 侧 DDD 重构**：完成 `internal/domain` 和 `internal/session` 的 model + repository + service，并迁移 `pkg/session/broadcaster`。
3. ✅ 启动 **M2 Python 侧真实 Agent**：实现 `adapters/anthropic.py` 流式调用 + tool use，并补充 mock 测试。
4. 🔄 下一步：把 `pkg/api` 中的 domain/session handler 逻辑迁入 `app/commands` / `app/queries`；补齐 OpenAI adapter 流式与 tool use；在 `session_executor.py` 中接入流式输出为 observation。

---

## 今日变更摘要

- 工程化：`package.json` + `Makefile` + `.changeset/` + `MONOREPO.md`
- Go DDD：`hnsx-server/internal/{domain,session}/` 骨架 + 测试
- Go Postgres：`internal/domain/repository/postgres/` + `internal/session/repository/postgres/` + 集成测试
- Python Agent：`adapters/base.py` + `adapters/anthropic.py` 流式/tool use + 测试
- 全链路：`make ci` 通过

---

## 不在这轮做的事

- 多租户 SaaS
- 复杂 Workflow DAG 可视化编辑器
- 自研 Agent 底座
- 模型微调平台
- Kafka / NATS 等消息队列（当前用 in-memory queue + long-poll）
- 企业 SSO / RBAC（当前阶段用 API Key）
