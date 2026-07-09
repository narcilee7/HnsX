# HnsX Go 服务 Roadmap

> Go Control Plane 从"代码搬运"到"核心调度平台"的分阶段建设路线。
>
> 原则：Go 负责**调度、治理、审计、API**；Python Worker 负责**Agent 执行**；二者通过 gRPC 控制平面解耦。

---

## 0. 当前状态（已完成）

- [x] 合并 `hnsx-core/` + `hnsx/` CLI 到 `hnsx-server/`，单一 Go module、双 `cmd` 入口。
- [x] 建立无基础设施依赖的共享层：
  - `pkg/spec` — DomainSpec 模型 + loader
  - `pkg/runtime` — Adapter 接口、Runner、Observation、Workflow
  - `pkg/adapter` — noop / echo 内置 adapter
- [x] 初步 DDD 骨架：
  - `internal/domain/{model,repository,service}`
  - `internal/session/{model,repository,service,broadcaster}`
- [x] `make ci` 通过（go vet/test + TS lint/type-check/test + worker pytest）。

**结论**：只是代码搬运和目录归位，业务层还没真正按 DDD 和应用服务拆分。

---

## 1. 总体目标

把 `hnsx-server` 建设成异构语言 monorepo 里的 **Control Plane**：

1. **调度平台**：像 Ray 一样管理 Python Worker 池——注册、心跳、能力匹配、任务分发、Cancel、扩缩容感知。
2. **治理中枢**：Domain 管理、Session 生命周期、Policy（预算/权限/guardrail）、AuditLog。
3. **API 网关**：REST 给控制台，gRPC 给 Worker，SSE 给实时观测。
4. **可观测底座**：Trace / Metric / Cost 聚合，为 Eval 平台提供数据。

---

## 2. 关键设计决策

| 话题 | 决策 |
|---|---|
| **Tool 层要不要？** | 要，但不是重造 shell/http/sql。对 API Agent，Tool 是真实可调用函数；对 Claude Code/Codex 等 CLI Agent，Tool 是**策略声明 + 审计钩子**。Tool 层 = Agent 能力的统一治理接口。 |
| **Adapter 与 Tool 的关系** | 正交。Adapter 解决"Agent 怎么跑起来"；Tool 解决"Agent 能用什么"。 |
| **memory → store** | `memory` 太窄，改为 `store`。覆盖 context store（短期工作记忆）、knowledge store（长期知识）、ephemeral store（session 内临时状态）。 |
| **InMemoryRepository** | 仅用于测试和 no-db 快速启动。生产永远注入 Postgres 实现。DDD 领域层只依赖 `Repository` 接口。 |
| **Worker 拉取 vs 推送** | 采用 **Pull（long-poll）**。Worker 主动拉任务，Control Plane 无状态、易扩缩容；Cancel 通过独立控制通道下发。 |
| **Session = subprocess** | 参考 Ray，一个 Session 一个独立进程，崩溃只影响当前 Session，资源隔离清晰。 |
| **两个 cmd 共用一套 handler** | `cmd/hnsx`（CLI）和 `cmd/hnsx-server`（HTTP）共用 `internal/app/commands` + `queries`，只是入口不同：一个走 `os.Args`，一个走 `chi.Router`。 |

---

## 3. 分阶段 Roadmap

### Phase 1：DDD 领域落地（2–3 周）

把 `pkg/` 里还残留的业务逻辑按 bounded context 迁入 `internal/`，每个领域包含 `model/`、`repository/`（接口+InMemory+Postgres 同包）、`service/`。

| 领域 | 当前位置 | 目标位置 | 关键任务 |
|---|---|---|---|
| **Worker** | `pkg/worker/registry.go`、`queue.go` | `internal/worker/{model,repository,service}` | Worker 注册、心跳、能力标签、任务分配、状态机。 |
| **Telemetry** | `pkg/telemetry` | `internal/telemetry/{model,service}` | Sink 抽象、Observation → Trace/Metric/Cost 转换。 |
| **Policy** | `pkg/policy/engine.go`（空壳） | `internal/policy/{model,service}` | Budget、Permission、Guardrail 运行时决策。 |
| **Secret** | 缺失 | `internal/secret/{model,repository,service}` | Secret 存储、运行时注入、访问审计。 |
| **Evaluation** | 缺失 | `internal/evaluation/{model,repository,service}` | EvalSet、EvalRun、Baseline、Scorer。 |
| **Store** | 原 `pkg/memory` | `internal/store/{model,repository,service}` | context/knowledge/ephemeral 三类后端抽象。 |

**验收标准**：
- `pkg/` 只保留真正跨 CLI/Server 复用的库（`spec`、`runtime`、`adapter`、`version`）。
- 每个新领域都有 `*_test.go` + Postgres 集成测试。
- `go test ./...` 通过。

---

### Phase 2：应用层与 API 重构（1–2 周）

把 `pkg/api/domains.go`、`pkg/api/sessions.go` 里的业务逻辑抽到应用层，handler 只做协议转换。

```text
internal/app/
  commands/
    validate_domain.go    # ValidateDomain
    register_domain.go    # RegisterDomain
    update_domain.go      # UpdateDomain
    trigger_session.go    # TriggerSession
    cancel_session.go     # CancelSession
    run_local.go          # hnsx run 本地执行
  queries/
    list_domains.go       # ListDomains / GetDomain
    list_sessions.go      # ListSessions / GetSession
    get_trace.go          # GetSessionTrace / Replay
```

**验收标准**：
- `pkg/api` handler 平均行数 < 30，只负责 decode/encode/route。
- CLI 和 HTTP 调用同一套 command/query。
- `cmd/hnsx` 支持 `hnsx domains register`、`hnsx sessions trigger` 等子命令。

---

### Phase 3：Worker 调度平台化（2–3 周）

把 Python Worker 从"能连上"变成"能调度"。

| 能力 | 说明 |
|---|---|
| **Capability 匹配** | DomainSpec 推导出所需能力（adapter 类型、tool 集合、sandbox 策略），`SessionQueue.Dequeue` 按 capability 匹配 Worker。 |
| **Worker 生命周期** | 注册 → 心跳 → 空闲 → 忙碌 → 离线/驱逐。状态持久化到 Postgres `runtimes` 表。 |
| **任务分发** | `PullSession` long-poll；分配后 Ack；超时不 Ack 重新入队。 |
| **Cancel 控制面** | `SendCancel` 通过 Registry 推送到 Worker 的 StreamChannel；Worker SIGTERM 子进程。 |
| **负载感知** | Worker 上报并发数/资源使用；调度优先分配给最空闲且匹配的 Worker。 |
| **自动重试** | Worker 崩溃或子进程非零退出时，按 Domain 策略重试或标记 failed。 |

**验收标准**：
- 启动 2+ Python Worker，session 被分配到不同 Worker。
- Cancel 请求能在 5s 内终止子进程。
- Worker 离线后，其未完成任务重新入队。

---

### Phase 4：Tool 与 Store 层（2 周）

| 能力 | 说明 |
|---|---|
| **Tool Registry** | `harness.tools` 声明工具；Go 侧维护注册表，做权限预检和审计。 |
| **Tool 实现网关** | 对 API Agent，Go 提供 Tool 调用实现（HTTP/SQL/Shell/Python function）；对 CLI Agent，Tool 实现可空，仅做策略校验。 |
| **Secret 注入** | Tool config 中 `${secret:xxx}` 在运行时被解析，Secret service 提供值并记录访问。 |
| **Store Backend** | `store.context.backend`、`store.knowledge.backend` 支持 `in_memory` / `postgres` / `redis`（未来）。 |
| **MCP 集成** | DomainSpec 中 `mcp.servers` 接入 Worker 的 MCP 客户端。 |

**验收标准**：
- Domain 声明 tool + secret，Worker 执行时拿到注入值。
- Tool 调用被记录到 AuditLog。
- Store 跨 turn 可读写，session rerun 可复用 knowledge。

---

### Phase 5：Policy 与审计（1–2 周）

| 能力 | 说明 |
|---|---|
| **Budget** | max cost / max turns / max tokens；运行时拦截超额调用。 |
| **Permission** | file_write / file_delete / network / shell；Tool 调用前校验。 |
| **Guardrail** | 基于输出内容/状态的事件规则；action 包括 block / log / human_approval。 |
| **AuditLog** | 每次 Tool 调用、Policy 命中、Secret 访问生成不可变记录。 |
| **Human-in-the-loop** | guardrail action=human_approval 时暂停 session，控制台审批后恢复。 |

**验收标准**：
- 超 budget 的 session 被自动终止。
- 被 Policy 拦截的 Tool 调用返回明确错误码。
- AuditLog 可通过 API 查询。

---

### Phase 6：可观测性与 Eval（2 周）

| 能力 | 说明 |
|---|---|
| **Trace 持久化** | Observation 流写入 Postgres / Tempo；API 支持按 session_id 查询完整 trace。 |
| **Cost 聚合** | 按 session / domain / worker 汇总 cost、latency、token。 |
| **Eval 平台** | EvalSet YAML → 批量触发 session → Scorer 打分 → Baseline 对比。 |
| **Regression Gate** | CI 中跑 EvalSet，分数低于 baseline 自动 block PR。 |

**验收标准**：
- 控制台可查看任意 session 的完整 trace 时间线。
- `make eval` 跑一个 EvalSet 并生成报告。

---

### Phase 7：部署与扩展（2 周）

| 能力 | 说明 |
|---|---|
| **持久化 Queue** | SessionQueue 从内存改为 Postgres/Redis，支持多实例 Control Plane。 |
| **Worker 自动发现** | K8s DaemonSet / Deployment，Worker 启动后自注册。 |
| ** graceful shutdown** | Control Plane 停止前 draining 请求；Worker 停止前完成或移交手中 session。 |
| **Multi-tenant** | `tenant_id` 贯穿 domain / session / worker / audit。 |
| **混沌测试** | 随机 kill worker / 网络分区，验证任务不丢、状态一致。 |

**验收标准**：
- 两个 Control Plane 实例共用一个 Postgres queue，session 不重复分发。
- Worker 被 kill 后，session 最终状态正确。

---

## 4. 近期优先级（接下来 1–2 周）

1. **Worker 领域 DDD 化** — 当前 `pkg/worker` 还是基础设施逻辑，先迁到 `internal/worker`。
2. **应用层 commands/queries 落地** — 把 `pkg/api` 的业务逻辑抽出去。
3. **Capability 匹配调度** — 让多个 Python Worker 能真正被调度起来跑 session。
4. **Policy 运行时空壳填实** — 先把 budget/permission 校验接进 executor 路径。

---

## 5. 风险与待决策

| 风险 | 说明 | 建议 |
|---|---|---|
| **Worker 拉取超时设计** | long-poll 多久合适？太短增加 QPS，太长影响 Cancel 响应。 | 先 30s long-poll + 5s 内 Cancel 控制通道。 |
| **Store 后端选型** | context store 用 postgres 还是 redis？ | Phase 1 只支持 in_memory/postgres；redis 放到 Phase 7。 |
| **Tool 实现的归属** | 有些 Tool 用 Go 实现（查 DB），有些用 Python（跑脚本）。 | Tool Registry 在 Go，具体实现可下沉到 Worker；Go 做网关 + 审计。 |
| **Observation 存储成本** | 高频 observation 可能撑爆 DB。 | 支持 TTL + 归档到对象存储；Phase 6 再做。 |
| **Human approval 状态机** | 需要 pause/resume 语义，影响 session 状态机。 | Phase 5 引入 `paused` 状态，与 `pending/running` 区分。 |

---

## 6. 与 Python Worker 的边界

| 职责 | Go Control Plane | Python Worker |
|---|---|---|
| DomainSpec 校验 | ✅ 入口校验 + 版本管理 | ❌ 只消费 |
| Session 状态机 | ✅ 唯一真相源 | ❌ 只上报 |
| Worker 调度 | ✅ capability 匹配、分发、Cancel | ❌ 只拉取/上报 |
| Adapter 调用 | ❌ | ✅ Anthropic/OpenAI/ClaudeCode 等 |
| Tool 执行 | ✅ 网关 + 审计（部分实现） | ✅ 实际调用（MCP/Shell/Python） |
| Observation 产生 | ❌ | ✅ stdout JSONL / gRPC stream |
| Observation 持久化/广播 | ✅ | ❌ |
| Policy 决策 | ✅ | ❌ 只接收 deny 结果 |
| Cost 聚合 | ✅ | ❌ 上报原始 usage |

---

> 下一批可执行动作：先完成 Phase 1 的 `internal/worker` 迁移 + Phase 2 的 `app/commands` 拆分，同时把 `pkg/api` handler 彻底变薄。
