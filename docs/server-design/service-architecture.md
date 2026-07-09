# HnsX 服务架构设计（V1.1 → V1.2）

> 本文描述 HnsX 从当前 V1.1 骨架到 V1.2 完整服务形态的架构设计。核心决策：**Go 负责 Control Plane（调度、治理、审计、API），Python 负责 Runtime Worker（Agent 执行、Tool/MCP、Sandbox、Store、Policy 运行时）**。

---

## 现状基线

当前仓库已经完成 V1.1 的骨架，并正在向 V1.2 演进：

- **Go Control Plane**：单一 Go module `hnsx-server`，内部按 DDD 组织。`hnsx-server/pkg/controlplane` 实现了 gRPC `WorkerService` + `SchedulerService`，`hnsx-server/pkg/worker` 实现了 in-memory `Registry` + `SessionQueue`。
- **CLI 与 Server 同库**：`hnsx-server/cmd/hnsx` 和 `hnsx-server/cmd/hnsx-server` 共享 `internal/app/commands` 与 `pkg/spec`、`pkg/runtime` 等轻量包，避免跨 module 维护。
- **REST API**：`hnsx-server/pkg/api` 支持 Domain 注册、Session trigger、SSE 事件流、Cancel；当 worker pool 启用时会把 Session 入队给 Python worker。
- **Python Worker**：`hnsx-worker/hnsx_worker/worker_service.py` 实现了 parent process（Register / Heartbeat / PullSession / StreamChannel），并为每个 session fork 一个 `session_runtime.py` 子进程。
- **子进程运行时**：`session_executor.py` 支持 `single` / `multi-turn` / `workflow`，`adapters/` 有真实 HTTP 调用（anthropic / openai）和本地桩子（noop / echo / ollama）。
- **协议**：`proto/hnsx/v1/worker.proto` + `observation.proto` 已经定义了 worker ↔ server 的契约。

**仍然缺失**：持久化、Worker 能力匹配调度、完整的 Session 状态机、Python 侧的 Tool/MCP/Sandbox/Store/Policy、Eval Runner、Secret 注入、真正的多 Agent 编排（supervisor/autonomous）。

---

## 目标架构

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│                              用户与消费层                                      │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │ hnsx CLI     │  │ Web Console  │  │ Python SDK   │  │ Node.js SDK  │      │
│  │   (Go)       │  │   (React/TS) │  │              │  │              │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
└─────────┼────────────────┼────────────────┼────────────────┼────────────────┘
          │                │                │                │
          │  HTTP/REST     │  HTTP/REST+SSE │  HTTP/gRPC     │  HTTP/gRPC
          │                │                │                │
┌─────────┼────────────────┴────────────────┴────────────────┼────────────────┐
│         │           HnsX Control Plane（Go）                │                │
│         │  ┌─────────────────────────────────────────────┐ │                │
│         │  │ HTTP API Gateway （chi）                     │ │                │
│         │  │ Domain Registry / Versioning                 │ │                │
│         │  │ Session Scheduler + Worker Registry          │ │                │
│         │  │ Eval Runner + Benchmark Registry             │ │                │
│         │  │ Secret / Policy / Sandbox Profile Store      │ │                │
│         │  │ AuditLog + Cost Aggregation                  │ │                │
│         │  └─────────────────────────────────────────────┘ │                │
│         │                          │                        │                │
│         │  ┌───────────────────────┴──────────────────────┐ │                │
│         │  │ gRPC Control Plane                             │ │                │
│         │  │ WorkerService / SchedulerService               │ │                │
│         │  └──────────────────────────────────────────────┘ │                │
│         │                          │                         │                │
│         │  ┌───────────────────────┴──────────────────────┐ │                │
│         │  │ Telemetry / Observability Aggregation        │ │                │
│         │  │ OTLP / PostgreSQL / SSE fan-out              │ │                │
│         │  └──────────────────────────────────────────────┘ │                │
│         └────────────────────────────────────────────────────┘                │
│                                      │                                        │
│                           gRPC over mTLS / TLS                               │
│                                      │                                        │
│         ┌────────────────────────────┴────────────────────────────┐          │
│         │              HnsX Runtime Worker（Python）               │          │
│         │  ┌─────────────┐ ┌─────────────┐ ┌──────────────────┐  │          │
│         │  │ Worker      │ │ Session     │ │ Agent Adapter    │  │          │
│         │  │ Parent      │ │ Subprocess  │ │ Registry         │  │          │
│         │  │ Process     │ │ (per session)│ │ (Anthropic/      │  │          │
│         │  │             │ │             │ │  OpenAI/Claude/  │  │          │
│         │  │ - Register  │ │ - Domain    │ │  Codex/Ollama)   │  │          │
│         │  │ - Heartbeat │ │   Loader    │ └──────────────────┘  │          │
│         │  │ - Pull      │ │ - Harness   │ ┌──────────────────┐  │          │
│         │  │   Session   │ │   Runner    │ │ Tool Registry    │  │          │
│         │  │ - Stream    │ │ - Turn Loop │ │ + MCP Client     │  │          │
│         │  │   Channel   │ │ - Transition│ └──────────────────┘  │          │
│         │  └─────────────┘ │ - Policy/   │ ┌──────────────────┐  │          │
│         │                  │   Sandbox   │ │ Store Backend    │  │          │
│         │                  │   checks    │ │ (context/        │  │          │
│         │                  │ - Obs emit  │ │  knowledge/      │  │          │
│         │                  └─────────────┘ │  ephemeral)      │  │          │
│         │                                  └──────────────────┘  │          │
│         └────────────────────────────────────────────────────────┘          │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## 分层职责

### 1. Control Plane（Go）——按 DDD 组织

**核心原则**：只做"调度、治理、审计、API"，不做 Agent 执行；代码按 bounded context 组织，而不是按技术层。

```text
hnsx-server/
  cmd/
    hnsx/             # CLI 入口：validate / run / version / eval（共用 app/commands）
    hnsx-server/      # Server 入口：HTTP/gRPC 服务
  pkg/
    spec/             # DomainSpec 模型 + loader（原 hnsx-core/domain + loader，无重依赖）
    runtime/          # Adapter 接口 + Runner + Observation（原 hnsx-core/runtime + observation）
    adapter/          # 内置 adapter：noop / echo（原 hnsx-core/adapter）
    api/              # HTTP handler：协议转换，调用 app/commands 和 app/queries
    controlplane/     # gRPC WorkerService / SchedulerService
    db/               # Postgres 连接与迁移
    worker/           # Worker registry + session queue（逐步迁入 internal/worker）
    telemetry/        # OTel + sink（逐步迁入 internal/telemetry）
    session/          # Executor / broadcaster（ broadcaster 已迁入 internal/session）
  internal/
    config/           # 配置加载
    domain/           # Domain 聚合：model / repository / service
    session/          # Session 聚合：model / repository / service / broadcaster
    worker/           # Worker 聚合（待建）
    evaluation/       # Eval 聚合（待建）
    telemetry/        # Telemetry 聚合（待建）
    policy/           # Policy 聚合（待建）
    secret/           # Secret 聚合（待建）
    app/
      commands/       # ValidateDomain / RegisterDomain / TriggerSession / CancelSession / RunLocalSession
      queries/        # ListSessions / GetTrace / ListDomains
    infrastructure/
      postgres/       # 各领域 repository 的 Postgres 实现
      grpcserver/     # gRPC 服务实现
      httpserver/     # HTTP 路由与中间件
      otel/           # OpenTelemetry 初始化
```

每个领域内包含自己的 model、repository（或按 Go 习惯叫 store）、service/usecase。当前 `pkg/` 下的实现逐步迁移到对应领域，最终 `pkg/` 只保留真正跨进程复用的库：

- `pkg/spec`、`pkg/runtime`、`pkg/adapter`：给 CLI 和 Server 共享的纯领域契约，**严禁引入 chi/pgx/grpc/otel 等基础设施依赖**。
- `pkg/api`、`pkg/controlplane`、`pkg/db`：基础设施层，未来迁入 `internal/infrastructure/`。

| 领域 | 职责 | 当前状态 | 下一步 |
|---|---|---|---|
| `domain` | DomainSpec 加载、校验、版本、引用解析 | `pkg/spec` + `internal/domain` | 稳定；loader 已从 `hnsx-core` 迁入 |
| `session` | Session 状态机、调度、广播、Cancel | `internal/session` + `pkg/api` | 状态机已 DDD 化；剩余 handler 逻辑迁入 `app/commands` |
| `worker` | Worker 注册、心跳、能力、分配 | `pkg/worker` | 迁入 `internal/worker`，接入 Postgres `runtimes` |
| `evaluation` | EvalSet、EvalRun、Scorer、Baseline | 缺失 | 新增 |
| `telemetry` | Observation/Trace/Metric/AuditLog | `pkg/telemetry` | 统一 observation → trace/metric/audit 转换 |
| `policy` | 规则存储、编译期校验、运行时决策点 | `pkg/policy` 空壳 | 实现 allowed/denied/budget/human-approval |
| `secret` | Secret 存储、运行时注入、访问审计 | 缺失 | 新增 |
| `app` | 编排各领域完成 Trigger / Cancel / Eval / RunLocal 用例 | 已规划 | 把 `pkg/api/domains.go`、`pkg/api/sessions.go` 业务逻辑迁入 |
| `infrastructure` | HTTP/gRPC/Postgres/OTel 具体实现 | `pkg/api` / `pkg/controlplane` / `pkg/db` | 作为基础设施接入各领域 |

### 2. Runtime Worker（Python）

**核心原则**：真正的 Agent 执行平面，按 session 隔离（subprocess），所有能力可插拔。

| 模块 | 职责 | 当前状态 | 下一步 |
|---|---|---|---|
| `worker_service.py` | parent process | 已实现 | 增加 worker capability 声明、优雅 drain |
| `session_runtime.py` | subprocess entry | 已实现 | 增加 secret/policy/sandbox/store 初始化 |
| `session_executor.py` | 编排执行 | single/workflow 骨架 | 实现完整状态机、supervisor/autonomous |
| `adapters/` | Agent 适配器 | noop/echo/anthropic/openai/ollama 桩子 | 补齐 Claude Code / Codex / 流式 / tool calling |
| `tools/` | Tool 能力治理层 | 缺失 | 对 API Agent：http/shell/sql/python/file；对 CLI Agent：约束+审计 |
| `mcp/` | MCP Client | 缺失 | 接入外部 MCP server |
| `sandbox/` | 隔离后端 | 缺失 | none / process / container / microvm / delegate |
| `store/` | 存储后端（原 memory） | 缺失 | context / knowledge / ephemeral；in_memory / postgres / redis |
| `policy/` | 运行时策略 | 缺失 | 预算、工具白名单、人工审批暂停 |
| `observation.py` | Observation 生成 | 基础 | 统一所有事件的 observation 化 |

---

## 关键数据流

### 1. Trigger → Schedule → Execute → Observe

```text
Console/CLI/SDK
   │
   ▼
POST /api/v1/sessions
   │
   ▼
Control Plane: validate domain → create Session row (pending)
                → enqueue SessionRequest into SessionQueue
   │
   ▼
Python Worker: PullSession (long-poll) → match capability
                → fork SessionRuntime subprocess
                → AckSession
   │
   ▼
SessionRuntime: load DomainSpec → init Adapter/Tool/MCP/Sandbox/Store
                → run Turn loop → emit Observations (stdout JSONL)
   │
   ▼
Worker Parent: pump observations → StreamChannel (gRPC bidi)
   │
   ▼
Control Plane: persist observations → fan-out to SSE clients
                → update Session state
                → aggregate cost/latency
                → write AuditLog (if policy event)
   │
   ▼
Console/SSE: real-time observation stream
```

### 2. Cancel 流

```text
POST /api/v1/sessions/:id/cancel
   │
   ▼
Control Plane: lookup session → find assigned worker_id
                → SendCancel via Registry.Inbound channel
                → StreamChannel push CancelSessionCommand
   │
   ▼
Python Worker: receive cancel → SIGTERM subprocess
                → subprocess emits session_end/cancelled
                → status update → Control Plane updates state
```

### 3. Eval 流

```text
POST /api/v1/domains/:id/evals/:setId/run
   │
   ▼
Control Plane: load EvalSet → create EvalRun (running)
                → for each case: enqueue SessionRequest with eval_run_id tag
   │
   ▼
Worker pool executes sessions（同 Trigger 流）
   │
   ▼
Control Plane: collect SessionFinalResult + observations
                → apply Scorer → update EvalRun case scores
                → compare baseline → publish report
```

---

## 关键架构决策

### 1. 为什么 Session = subprocess？

参考 Ray：每个 session 一个独立 Python 子进程，提供：

- **强隔离**：一个 session 的内存/文件状态不会污染另一个。
- **可取消**：SIGTERM / SIGKILL 是干净的终止机制。
- **可观察**：子进程的 stdout/stderr 直接作为 observation 来源。
- **可扩展**：worker 可以跑在另一台机器上，只负责进程管理。

代价：启动延迟 ~100ms；未来可预 fork worker pool 优化。

### 2. 为什么 Worker 拉取（Pull）而不是服务器推送（Push）？

- Worker 更清楚自己的资源状态（free slots、当前 running sessions）。
- 长轮询让 worker 可以优雅地控制并发。
- 网络拓扑更简单：worker 只需 outbound 连接到 server。
- 与 Ray Core 的 worker → scheduler 请求模型一致。

### 3. Capability 匹配调度

`PullSessionRequest.required_capabilities` 与 `SessionRequest.required_capabilities` 做交集匹配：

```text
capability 格式：
  provider:anthropic
  model:claude-sonnet-4
  sandbox:container
  tool:shell
  mcp:crm-mcp
```

Control Plane 的 `SessionQueue.Dequeue` 已支持基础匹配；下一步把 capability 从 DomainSpec 推导出来。

### 4. Tool 层与 Adapter 的关系

这是最容易混淆的地方：

- **Adapter**：Harness 调用外部 Agent 的桥接层。它解决"**怎么让 Agent 跑起来**"：Anthropic API、OpenAI API、Claude Code CLI、Codex CLI、Ollama 等。
- **Tool**：Agent 可调用的外部能力。它解决"**Agent 能用什么**"：企业内部 API、数据库、文件系统、shell 等。

两种 Agent 类型对 Tool 层的需求不同：

| Agent 类型 | 自带 tool 能力？ | HnsX Tool 层的作用 |
|---|---|---|
| Claude Code / Codex CLI | 自带 shell、file、edit | **约束与审计层**，不复刻 tool；通过 Policy + Sandbox 划定边界，通过 Observation 记录行为 |
| OpenAI / Anthropic API | 需要调用方提供 functions/tools | **能力注册层**，提供统一 Tool Registry、secret 注入、策略拦截、调用审计 |
| 其他自研 Agent | 视情况而定 | 按需接入 |

所以 Tool 层不是"重新实现 shell/http/sql"，而是**Agent 能力的统一治理接口**：

- 对 API Agent：Tool 是真实可调用的函数实现。
- 对 CLI Agent：Tool 是策略声明 + 审计钩子，Agent 自己决定用不用。

### 5. memory → store

原 `memory` 概念太狭窄，只暗示"长期记忆"。改名为 `store` 后覆盖：

- **context store**：当前 session / turn 的上下文（短期工作记忆）。
- **knowledge store**：跨 session 的长期知识、向量检索结果。
- **ephemeral store**：session 内的临时计算状态。

DomainSpec 中也从：

```yaml
memory:
  backend: postgres
```

改为：

```yaml
store:
  context:
    backend: in_memory
  knowledge:
    backend: postgres
```

### 6. Secret 不落在 DomainSpec

- DomainSpec 中 Tool/MCP 配置用占位符 `{secret.XXX}`。
- Control Plane 在 enqueue 前解析并注入到 `SessionRequest.secrets`。
- Python subprocess 通过环境变量或加密 stdin 接收 secrets。
- 审计日志记录 secret 访问（不记录值）。

### 7. Policy 两层生效

- **编译期**（Control Plane）：检查 allowed/denied tool、预算上限、secret 解析。
- **运行期**（Python Worker）：每次 tool call 前再过 Policy；超预算时暂停；敏感操作触发 human_approval。

### 8. Observation 是唯一真相源

所有运行时事件都转成 Observation：

- Python Worker 通过 stdout JSONL 发给 parent。
- Parent 通过 gRPC StreamChannel 发给 Control Plane。
- Control Plane 写 DB + SSE fan-out + OTLP export。
- Eval 直接消费 Observation 序列。

---

## 模块拆分建议

### Go 侧：按 DDD 迁移

**目标结构**（V1.2 推进中）：

```text
hnsx-server/
  cmd/
    hnsx/              # CLI 入口：validate / run / version
    hnsx-server/       # Server 入口：HTTP + gRPC 服务
  pkg/
    spec/              # DomainSpec 模型 + loader（CLI 与 Server 共享，无重依赖）
    runtime/           # Adapter 接口 + Runner + Observation 模型
    adapter/           # noop / echo 等内置 adapter
    api/               # HTTP handler（逐步变薄，只负责协议转换）
    controlplane/      # gRPC 服务
    db/                # Postgres 连接
    worker/            # Worker registry + queue（待迁入 internal/worker）
    telemetry/         # OTel + sink（待迁入 internal/telemetry）
    session/           # Executor（broadcaster 已迁 internal/session）
  internal/
    config/
    domain/{model,repository,service}
    session/{model,repository,service,broadcaster}
    worker/            # 待建
    evaluation/        # 待建
    telemetry/         # 待建
    policy/            # 待建
    secret/            # 待建
    app/
      commands/        # 复用：CLI 和 HTTP 共用同一套命令实现
      queries/         # 复用：CLI 和 HTTP 共用同一套查询实现
    infrastructure/
      postgres/
      grpcserver/
      httpserver/
      otel/
```

**关键约束**：

- `pkg/spec`、`pkg/runtime`、`pkg/adapter` 只放纯领域契约，**不能依赖 chi、pgx、grpc、otel**。
- `cmd/hnsx` 只 import `pkg/spec`、`pkg/runtime`、`pkg/adapter`、`internal/app/commands`，保证 CLI 二进制精简。
- `cmd/hnsx-server` 可以 import 所有基础设施包。

**迁移路径**：

1. 把 `hnsx-core/` 中的 model、loader、runtime、adapter、observation 迁入 `hnsx-server/pkg/spec`、`pkg/runtime`、`pkg/adapter`。
2. 删除独立 `hnsx-core` module 和 `hnsx/` CLI module，合并到 `hnsx-server`。
3. 在 `internal/app/` 新建 `commands/` 和 `queries/`，把 `pkg/api/domains.go`、`pkg/api/sessions.go` 中的业务逻辑迁进去。
4. 把 `pkg/worker`、`pkg/session`、`pkg/telemetry` 逐步迁入对应 `internal/` 领域。
5. `pkg/` 最终只保留与 CLI / SDK 共享的运行时库。

### Python 侧新增

```text
hnsx-worker/hnsx_worker/
  adapters/
    claude_code.py  # Claude Code CLI 进程管理
    codex.py        # OpenAI Codex CLI 进程管理
    base.py         # 已存在，增加流式 invoke_stream
  tools/
    registry.py     # Tool 注册 + 策略拦截
    http.py         # API Agent 用
    shell.py        # API Agent 用（可选）
    sql.py          # API Agent 用（可选）
  mcp/
    client.py
  sandbox/
    backend.py
    none.py
    process.py
    container.py
  store/
    backend.py
    context.py      # 短期上下文
    knowledge.py    # 长期知识
    ephemeral.py    # 临时状态
  engine/
    engine.py
  harness/
    loader.py       # DomainSpec 加载与引用校验
    runner.py       # 完整 orchestration 状态机
    transition.py   # JMESPath transition 求值
  observation.py    # 统一 observation 生成
```

---

## 实施路线图

### Phase 1：骨架跑通（当前 V1.1 Step 2 目标）

1. Python Worker 能注册、心跳、拉取 session、fork 子进程。
2. `noop` / `echo` adapter 跑通完整 Trigger → SSE 链路。
3. Go Control Plane 把 observation 从 worker 转发到 SSE。
4. Cancel 从 REST API 传到 worker 再 SIGTERM 子进程。

### Phase 2：真实 Agent 与 Tool 治理

1. 实现 `anthropic` / `openai` adapter 的真实 HTTP 调用（流式 + function calling）。
2. 对 API Agent：实现 Tool Registry（http / sql / python），让 Agent 通过 function calling 调用。
3. 对 CLI Agent（Claude Code / Codex）：实现 Policy 边界观测，记录 shell/file/edit 行为为 observation。
4. 在 `session_executor.py` 中实现多 turn loop 和 tool call 处理。

### Phase 3：编排策略

1. 实现 `supervisor`：supervisor agent 产生 routing_decision，Harness 做 transition。
2. 实现 `workflow`：基于 JMESPath exit / transition 的 DAG。
3. 实现 `autonomous`：agent 自己决定 tool / agent 调用，Harness 只审计和否决。

### Phase 4：约束与治理

1. Python Policy 引擎：budget、allowed/denied tools、human approval 暂停。
2. Sandbox backend：container / process。
3. Store backend：context / knowledge / ephemeral；in_memory → postgres / redis。
4. Secret Store 和注入。

### Phase 5：持久化与可观测性

1. Postgres schema：sessions / observations / traces / eval_runs / audit_log / workers。
2. Control Plane 持久化 session 状态、observations。
3. OTLP exporter 接入 Tempo / Grafana。
4. Metric aggregation：cost、latency、token。

### Phase 6：Eval 平台化

1. EvalSet / EvalCase 数据模型。
2. Eval Runner：批量触发、收集 observation、打分。
3. Scorer：exact / contains / jmespath / structured_match / llm-judge。
4. Baseline 对比与 CI 集成。

### Phase 7：部署与扩展

1. Docker Compose：server + postgres + worker + tempo/grafana。
2. Kubernetes / Helm：worker Deployment 可水平扩展。
3. mTLS：worker ↔ server。
4. Multi-tenant 预留接口。

---

## 风险与取舍

| 风险 | 应对 |
|---|---|
| Python 子进程启动延迟高 | 先接受 ~100ms；未来用预 fork pool 或长驻 session runtime。 |
| Claude Code / Codex 是 CLI 工具，难以流式观测 | Adapter 用 stdin/stdout 或 MCP 桥接；observation 从解析输出构造。 |
| Policy 在 Python 运行时生效，可能绕过 | 关键审计事件必须回写 Control Plane AuditLog；敏感 tool 调用双签。 |
| Observation 量过大 | DB 按 domain/time 分区；高频 metric 可采样；审计事件不采样。 |
| Worker 与 Server 网络中断 | Heartbeat + PullSession 超时后，server 把 session 标记为 failed 或重新入队。 |

---

## 下一步建议

建议分两条线并行推进：

1. **Go 侧 DDD 重构**：先把 `internal/domain`、`internal/session`、`internal/worker` 的 model + repository + service 搭起来，把现有 `pkg/api`、`pkg/worker`、`pkg/session` 的逻辑逐步迁入。这是后续所有复杂功能的基础。

2. **Python 侧真实 Agent 跑通**：
   - 实现 `adapters/anthropic.py` 的流式调用 + function calling。
   - 实现 `tools/registry.py` + `tools/http.py`，让 API Agent 能调用外部 API。
   - 让 `customer-service` domain 通过 Anthropic API 跑通一次带 tool call 的真实会话。

这两条线在 Phase 2 交汇：Go 负责调度与治理，Python 负责 Agent 执行与 Tool 治理。
