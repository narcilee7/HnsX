# HnsX 工程治理与架构落地（2026.07.09）

> 本文记录本轮工程治理的结论、已完成的重构、以及仍然悬而未决的下一步。
> 北极星文档：`docs/vision.md`
> 服务架构设计：`docs/server-design/service-architecture.md`
> 迭代路线：`docs/project_management/2026.07.09/v1_2_roadmap.md`

---

## 本轮目标

1. 把异构语言 monorepo 的工程化基线打牢（pnpm workspace + Makefile + changesets）。
2. 把 `hnsx-server` 从糙放的 `pkg/` 大杂烩迁到按 DDD 组织的 `internal/`。
3. 澄清几个关键架构概念：
   - Tool 层到底要不要？做什么？
   - Adapter 层与 Tool 层的关系。
   - `memory` 改名为 `store`。
   - 为什么仓库里有 `InMemoryRepository`，以及它和 Postgres 的关系。

---

## 1. Monorepo 工程化基线

### 已落地

| 项 | 状态 | 说明 |
|---|---|---|
| pnpm workspace | ✅ | `hnsx-console` / `observability` / `sdk/node` |
| Changesets | ✅ | `.changeset/` + `pnpm changeset` |
| 统一 Makefile | ✅ | `make ci` = proto-lint + go vet/test + TS lint/type-check/test + worker-test |
| Go modules | ✅ | 主 module `hnsx-server/`，双 cmd：`cmd/hnsx`（CLI）+ `cmd/hnsx-server`（控制面）；`sdk/go/` 为 placeholder，尚未纳入 `go.work` |
| Python worker | ✅ | `hnsx-worker/` 独立包 + venv + pytest |
| 文档约定 | ✅ | `MONOREPO.md` 描述包边界、版本管理、提交流程 |

### 待完善

- [ ] 把 `make ci` 拆成更快的 `make ci-fast`（不跑 worker-test）和 `make ci-full`，本地开发只跑前者。
- [ ] 给 `hnsx-server` 增加 `golangci-lint` 配置（当前只有 `go vet`）。
- [ ] 给 `hnsx-worker` 增加 `ruff` / `mypy` 到 `make lint`。
- [ ] 引入 `buf breaking` 到 CI，防止 proto 不兼容变更。

---

## 2. Go 侧 DDD 重构

### 目标结构

```text
hnsx-server/internal/
  domain/
    model/         # Domain, DomainSpec, Harness 等聚合根
    repository/    # 接口 + InMemory 实现 + Postgres 实现（同包）
    service/       # 注册、校验、版本管理用例
  session/
    model/         # Session, SessionState
    repository/    # 接口 + InMemory 实现 + Postgres 实现（同包）
    service/       # 创建、状态流转、Cancel、Rerun
    broadcaster/   # SSE 广播（从 pkg/session 迁入）
```

### 关键约定

- **以 bounded context 为顶层**，不是以存储技术为顶层。
  - ❌ 不要 `postgres/domain/`、`postgres/session/`。
  - ✅ 要 `domain/repository/postgres.go`、`session/repository/postgres.go`。
- **Repository 接口与实现同包**：`repository.go` 放接口 + `InMemoryRepository`，`postgres.go` 放 `PostgresRepository`。
  - 这样测试可以无缝替换，生产代码只依赖 `repository.Repository` 接口。
- **Service 只操作 model + repository**，不依赖 HTTP/gRPC/DB 具体技术。
- **`pkg/` 逐步瘦身**：只保留真正跨进程复用的库，业务逻辑迁到 `internal/`。

### 已完成

- [x] `internal/domain/{model,repository,service}` + 测试 + Postgres 集成测试。
- [x] `internal/session/{model,repository,service,broadcaster}` + 测试 + Postgres 集成测试。
- [x] `pkg/session/broadcaster.go` 迁入 `internal/session/broadcaster/`。
- [x] `pkg/api/server.go`、`pkg/api/sessions.go`、`pkg/session/executor.go` 更新 import，使用新的 broadcaster 包。
- [x] `go test ./...` 通过（含 Postgres 集成测试，需本地 `make db-up`）。

### 仍然要做的 DDD 迁移

- [ ] `internal/worker/{model,repository,service}`：把 `pkg/worker/registry.go`、`queue.go` 迁进来。
- [ ] `internal/telemetry/{model,repository,service}`：把 `pkg/telemetry` 的 sink 逻辑迁进来。
- [ ] `internal/policy/{model,service}`：把 `pkg/policy` 从空壳变成可运行。
- [ ] `internal/secret/{model,repository,service}`：新增。
- [ ] `internal/evaluation/{model,repository,service}`：新增。
- [ ] `internal/app/commands` + `internal/app/queries`：把 `pkg/api/domains.go`、`pkg/api/sessions.go` 里的业务逻辑抽到应用层，handler 只做协议转换。

---

## 3. 架构概念澄清

### 3.1 Tool 层要不要？

**结论：要，但不是重新发明 shell/http/sql。**

Tool 层是 **Agent 能力的统一治理接口**，对不同类型 Agent 作用不同：

| Agent 类型 | 自带 tool 能力？ | HnsX Tool 层的作用 |
|---|---|---|
| Claude Code / Codex CLI | 自带 shell、file、edit | **约束与审计层**：通过 Policy + Sandbox 划定边界，通过 Observation 记录行为 |
| Anthropic / OpenAI API | 需要调用方提供 functions | **能力注册层**：统一 Tool Registry、secret 注入、策略拦截、调用审计 |
| 自研 Agent | 视情况而定 | 按需接入 |

所以：

- 对 API Agent：Tool 是真实可调用的函数实现（http/sql/python/file 等）。
- 对 CLI Agent：Tool 是策略声明 + 审计钩子，Agent 自己决定用不用，HnsX 负责记录和拦截。

### 3.2 Adapter 层与 Tool 层的关系

这是最容易混淆的地方：

- **Adapter**：解决"**怎么让 Agent 跑起来**"——Anthropic API、OpenAI API、Claude Code CLI、Codex CLI、Ollama 等。
- **Tool**：解决"**Agent 能用什么**"——企业内部 API、数据库、文件系统、shell 等。

两者不是替代关系，是**正交层**。

### 3.3 memory → store

原 `memory` 太狭窄，只暗示"长期记忆”。改名为 `store` 后覆盖：

- **context store**：当前 session / turn 的短期工作记忆。
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

### 3.4 为什么有 InMemoryRepository？我们不是用 PostgreSQL 吗？

**InMemoryRepository 不是生产实现，是测试和 no-db 模式的依赖。**

设计原因：

1. **测试隔离**：单元测试不依赖 Postgres，启动快、无状态、可并行。
2. **接口契约清晰**：`Repository` 接口生产用 Postgres，测试用 InMemory，调用方无感知。
3. **no-db 模式**：未来本地快速启动 / CI 快速验证可以用 InMemory。
4. **DDD 标准做法**：领域层只依赖接口，具体持久化是基础设施细节。

Postgres 实现放在同一包的 `postgres.go` 中，由 main 根据配置选择注入。

---

## 4. 服务架构总览

```text
用户/消费层
  │
  ▼
HTTP/REST + SSE  ──▶  HnsX Control Plane（Go）
                       │
                       ├─ Domain 管理
                       ├─ Session 生命周期 + 调度
                       ├─ Worker 注册 + 能力匹配
                       ├─ Policy / Secret / Eval
                       ├─ Telemetry + AuditLog
                       │
                       ▼
              gRPC over TLS/mTLS
                       │
                       ▼
              HnsX Runtime Worker（Python）
                       │
                       ├─ Worker parent process（注册/心跳/拉取 session）
                       ├─ Session subprocess（每 session 一个）
                       │   ├─ Domain Loader
                       │   ├─ Adapter Registry
                       │   ├─ Tool Registry + MCP Client
                       │   ├─ Policy / Sandbox / Store
                       │   └─ Observation emit
                       │
                       ▼
              Postgres / Tempo / Grafana
```

核心决策：

- **Go = Control Plane**：调度、治理、审计、API、持久化。
- **Python = Runtime Worker**：Agent 执行、Tool/MCP、Sandbox、Store、Policy 运行时。
- **每 session 一个 Python subprocess**：参考 Ray worker 设计，强隔离、可取消、可观测。
- **Worker 拉取（Pull）而非服务器推送（Push）**：worker 更清楚自身资源状态，网络拓扑更简单。
- **Observation 是唯一真相源**：所有运行时事件都转成 Observation，经 gRPC 回传 Control Plane，再写 DB + SSE fan-out + OTLP export。

---

## 5. 下一步（按优先级）

### 近期（本周）

1. **把 `pkg/api` 中的 domain/session handler 逻辑迁入 `internal/app/commands` 和 `internal/app/queries`**
   - `TriggerSession`、`CancelSession`、`RerunSession`
   - `RegisterDomain`、`ListDomains`、`GetDomain`
   - handler 只负责 HTTP 协议转换，业务逻辑在 app 层。

2. **把 `pkg/worker` 迁入 `internal/worker`**
   - `Registry`、`SessionQueue`、`Capability` 匹配
   - 定义 `WorkerRepository` 接口 + InMemory + Postgres

3. **补齐 `internal/telemetry`**
   - 把 `pkg/telemetry` 的 sink 逻辑迁进来
   - Observation → Trace/Metric/Audit 的转换

### 中期（2-3 周）

4. **Policy 引擎**
   - budget、allowed/denied tools、human approval 暂停
   - 编译期校验（Control Plane）+ 运行期决策（Python Worker）

5. **Secret 注入**
   - DomainSpec 中用 `{secret.XXX}` 占位
   - Control Plane 在 enqueue 前解析并注入 `SessionRequest`
   - Python subprocess 通过环境变量接收

6. **Store 抽象**
   - context / knowledge / ephemeral
   - in_memory / postgres / redis backend

7. **Eval 平台**
   - `EvalSet`、`EvalRun`、`Scorer`
   - baseline 对比 + CI 门禁

### 远期

8. Docker Compose 一键启动：server + postgres + worker + tempo/grafana
9. Helm chart + worker 水平扩展
10. worker ↔ server mTLS

---

## 6. 验收标准

- [x] `make ci` 中 Go 部分通过（proto-lint + vet + test-go）。
- [x] `go test ./...` 通过。
- [x] `internal/{domain,session}` 目录结构符合 DDD 约定。
- [x] `pkg/api` 和 `pkg/session` 不再直接引用旧 `internal/session` 大包，而是引用 `internal/session/broadcaster`。
- [ ] 后续 PR：handler 逻辑迁入 `internal/app`，`pkg/api` 只剩路由和协议转换。

---

## 7. 参考资料

- `docs/vision.md`
- `docs/server-design/service-architecture.md`
- `docs/project_management/2026.07.09/v1_2_roadmap.md`
- `MONOREPO.md`
- `hnsx-server/internal/domain/`
- `hnsx-server/internal/session/`
