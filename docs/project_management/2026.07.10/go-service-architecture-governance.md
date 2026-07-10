# HnsX Go 服务架构治理 Roadmap

> 日期：2026.07.10
> 目标：完成 Go Control Plane 的 DDD 架构落地、CLI/Server 边界澄清、Postgres 持久化统一、gRPC/HTTP 服务治理。
> 北极星：`docs/vision.md`
> 架构设计：`docs/server-design/service-architecture.md`

---

## 一、核心原则

1. **Server 独占应用层**：`internal/app/commands` 与 `queries` 是 Server-side use cases，依赖 repository/service，不暴露给 CLI。
2. **CLI 只共享纯运行时**：`pkg/spec`、`pkg/runtime`、`pkg/adapter`、`pkg/local` 可以被 CLI 直接 import，严禁依赖 DB/HTTP/gRPC/OTel。
3. **CLI 远程命令走 HTTP client**：`hnsx domains`、`hnsx sessions`、`hnsx eval` 等通过 `internal/client` 访问 Server API。
4. **Postgres 是生产唯一真相源**：`domain`、`session` 已有 Postgres 实现；其余 `worker/audit/evaluation/trace/policy/secret` 补齐 Postgres，InMemory 仅用于测试/no-db 模式。
5. **handler 只做协议转换**：`pkg/api` 与 `pkg/controlplane` 只负责 decode/encode/route，业务逻辑下沉到 `internal/app`。

---

## 二、总体阶段

```text
Phase 0: 基础拆分（pkg/local + Application 组合器）
    │
    ▼
Phase 1: Server-side 应用层重构（internal/app/commands + queries）
    │
    ▼
Phase 2: Postgres 持久化补齐（worker/audit/eval/trace/policy/secret）
    │
    ▼
Phase 3: HTTP/gRPC handler 治理（pkg/api + pkg/controlplane）
    │
    ▼
Phase 4: CLI 远程命令（internal/client + cmd/hnsx）
    │
    ▼
Phase 5: 集成验收（go test ./... + make ci + 手工冒烟）
```

---

## 三、Phase 0：基础拆分

**目标**：把 CLI 真正能共享的本地命令拆出来；把 Server 的依赖组合逻辑从 `main.go` 抽离。

### 3.1 新建 `pkg/local`

从 `internal/app/commands` 拆出纯本地函数：

```text
hnsx-server/pkg/local/
  ├── local.go          # DomainSummary, ValidateDomain
  └── run.go            # RunLocalSession, PickAdapter
```

涉及函数：
- `ValidateDomain(r io.Reader, contentType string) (*DomainSummary, error)`
- `RunLocalSession(ctx, spec, trigger, adapter) (*runtime.Result, error)`
- `PickAdapter(kind string) (runtime.Adapter, error)`

**验收**：
- `pkg/local` 不 import `internal/app/*`、`pkg/db`、`pkg/api`、`pkg/controlplane`。
- `go test ./pkg/local/...` 通过。

### 3.2 新建 `internal/app/application.go`

组合 Server 所需的全部依赖：

```go
type Application struct {
    Config         *config.Config
    DB             *db.DB
    DomainService  *domain.Service
    SessionService *session.Service
    WorkerService  *worker.Service
    PolicyService  *policy.Service
    AuditService   *audit.Service
    TraceService   *trace.Service
    EvalService    *evaluation.Service
    SecretService  *secret.Service
    Executor       *pkgexecutor.Executor
    WorkerRegistry *worker.Registry
    SessionQueue   worker.SessionQueue
    BroadcasterMgr *broadcaster.Manager // 替代 AppState 的广播索引
}

func New(cfg *config.Config) (*Application, error)
```

**验收**：
- `cmd/hnsx-server/main.go` 的初始化代码从 200+ 行降到 ~50 行。
- `Application` 可被 `pkg/api.Server` 和 `pkg/controlplane.Server` 共用。

---

## 四、Phase 1：Server-side 应用层重构

**目标**：让 `internal/app/commands` 调用 `domain/session` service，而不是直接操作 `app.State`。

### 4.1 改造 `internal/app/commands/domain.go`

```go
type DomainCommands struct {
    domainSvc *domain.Service
}

func (c *DomainCommands) Register(ctx, tenantID, body) (*model.RegisteredDomain, error)
func (c *DomainCommands) Update(ctx, tenantID, id, body) (*model.RegisteredDomain, error)
func (c *DomainCommands) Delete(ctx, tenantID, id) error
```

### 4.2 改造 `internal/app/commands/session.go`

```go
type SessionCommands struct {
    sessionSvc *session.Service
    domainSvc  *domain.Service
    queue      worker.SessionQueue // optional
    executor   *pkgexecutor.Executor // optional
}

func (c *SessionCommands) Trigger(ctx, tenantID, domainID, trigger) (*model.Session, error)
func (c *SessionCommands) Cancel(ctx, tenantID, id) (*model.Session, error)
func (c *SessionCommands) Rerun(ctx, tenantID, id) (*model.Session, error)
func (c *SessionCommands) MarkRunning(...)
func (c *SessionCommands) MarkCompleted(...)
func (c *SessionCommands) MarkFailed(...)
```

### 4.3 改造 `internal/app/queries/queries.go`

Read models 调用 service：

```go
type Queries struct {
    domainSvc  *domain.Service
    sessionSvc *session.Service
    traceSvc   *trace.Service
}

func (q *Queries) ListDomains(ctx, tenantID) ([]DomainListItem, error)
func (q *Queries) GetDomain(ctx, tenantID, id) (*DomainListItem, *model.RegisteredDomain, error)
func (q *Queries) ListSessions(...) ([]SessionListItem, error)
func (q *Queries) GetSession(...) (*model.Session, error)
```

### 4.4 缩小 `internal/app/state.go` 职责

`app.State` 不再作为业务真相源，只保留：
- SSE broadcaster 索引（sessionID → *broadcaster.Broadcaster）
- 运行时缓存（可选，用于减少 DB 查询）

**验收**：
- `internal/app/commands` 不再 import `app.State`。
- `go test ./internal/app/...` 通过。

---

## 五、Phase 2：Postgres 持久化补齐

**目标**：所有 bounded context 都有 Postgres 实现，生产模式统一注入 Postgres。

### 5.1 新增 Postgres Repository

| 领域 | 文件 | 表 |
|---|---|---|
| worker | `internal/worker/repository/postgres.go` | `runtimes` |
| audit | `internal/audit/repository/postgres.go` | `audit_logs` |
| evaluation | `internal/evaluation/repository/postgres.go` | `eval_sets` / `eval_cases` / `eval_runs` / `eval_results` |
| trace | `internal/trace/repository/postgres.go` | `observations` |
| policy | `internal/policy/repository/postgres.go` | `policies` |
| secret | `internal/secret/repository/postgres.go` | `secrets` |

### 5.2 改造 `cmd/hnsx-server/main.go`

根据 `cfg.PostgresEnabled()` 选择实现：

```go
var domainRepo repository.Repository
if store.IsNoDB() {
    domainRepo = repository.NewInMemoryRepository()
} else {
    domainRepo = repository.NewPostgresRepository(store.Pool)
}
// 同理：session / worker / audit / eval / trace / policy / secret
```

### 5.3 模型与表的映射

需要确认/补齐以下 model 字段：
- `worker.Model.Worker` ↔ `runtimes`
- `audit.Model.Entry` ↔ `audit_logs`
- `eval.Model.EvalSet/EvalRun` ↔ `eval_sets` / `eval_runs`
- `trace.Model.Record` ↔ `observations`
- `policy.Model.Policy` ↔ `policies`
- `secret.Model.Secret` ↔ `secrets`

**验收**：
- 每个新增 Postgres repository 都有集成测试（参考 `domain/repository/postgres_test.go`）。
- `HNSX_DATABASE_URL` 配置下，server 启动后这些领域数据持久化。

---

## 六、Phase 3：HTTP/gRPC handler 治理

**目标**：`pkg/api` 和 `pkg/controlplane` 只调用 `internal/app/commands` 和 `queries`，不直接访问 `app.State`。

### 6.1 改造 `pkg/api/server.go`

```go
type Server struct {
    App       *app.Application
    BuildInfo BuildInfo
    // ... 不再直接持有 AppState
}
```

### 6.2 改造 `pkg/api/domains.go`

handler 调用 `App.Commands.Register/Update/Delete` 和 `App.Queries.ListDomains/GetDomain`。

### 6.3 改造 `pkg/api/sessions.go`

handler 调用 `App.Commands.Trigger/Cancel/Rerun` 和 `App.Queries.ListSessions/GetSession`。

### 6.4 改造 `pkg/controlplane/scheduler_service.go`

Worker 上报 observation / session status 时，通过 `App.SessionService.UpdateState` 写入 Postgres，再通过 `App.BroadcasterMgr` fan-out SSE。

### 6.5 清理 `pkg/controlplane`

- 确认旧服务（DomainRegistryService 等）是否还有引用；如无，移除相关代码或标记 deprecated。
- gRPC 与 HTTP 共享同一个 `Application` 实例。

**验收**：
- `pkg/api` handler 平均行数 < 30。
- `pkg/api` 不再直接读写 `app.State` 的 domain/session map。
- `go test ./pkg/api/...` 通过。

---

## 七、Phase 4：CLI 远程命令

**目标**：`cmd/hnsx` 支持本地命令 + 远程 Server 命令。

### 7.1 新建 `internal/client`

```text
hnsx-server/internal/client/
  ├── client.go       # Client 构造 + BaseURL + API Key
  ├── domains.go      # ListDomains / GetDomain / RegisterDomain / UpdateDomain
  ├── sessions.go     # ListSessions / GetSession / TriggerSession / CancelSession / RerunSession
  └── events.go       # SSE client for /sessions/:id/events
```

### 7.2 改造 `cmd/hnsx/main.go`

```go
switch os.Args[1] {
case "validate":
    // pkg/local
 case "run":
    // pkg/local
 case "version":
    // version
 case "login":
    // 写入 ~/.hnsx/config.yaml
 case "domains":
    // internal/client
 case "sessions":
    // internal/client
 case "eval":
    // internal/client
}
```

### 7.3 CLI 配置

`~/.hnsx/config.yaml`：

```yaml
server_url: http://127.0.0.1:50051
api_key: hnsx-api-key-xxx
```

**验收**：
- `hnsx validate --domain domain.yaml` 本地运行。
- `hnsx domains list` 调用 Server API 返回结果。
- `hnsx sessions trigger --domain customer-service --trigger '{"question":"hello"}'` 成功创建 session。

---

## 八、Phase 5：集成验收

### 8.1 自动化测试

```bash
cd hnsx-server
go test ./...
make ci          # proto-lint + go vet/test + TS lint/type-check/test + worker pytest
```

### 8.2 手工冒烟

| 场景 | 命令 |
|---|---|
| 本地校验 | `hnsx validate --domain example-domains/customer-service/domain.yaml` |
| 本地运行 | `hnsx run --domain ... --adapter noop --trigger '{"question":"hello"}'` |
| Server 启动 | `HNSX_DATABASE_URL=postgresql://... hnsx-server server` |
| 注册 Domain | `hnsx domains register --file ...` |
| 触发 Session | `hnsx sessions trigger --domain customer-service --trigger '{"question":"hello"}'` |
| 查询 Session | `hnsx sessions get <id>` |
| SSE 观测 | `curl http://127.0.0.1:50051/api/v1/sessions/<id>/events` |
| 重启 Server | 确认 domain/session/worker 数据仍在 |

---

## 九、风险与应对

| 风险 | 应对 |
|---|---|
| `app.State` 与 service 双写不一致 | 用 service 作为唯一写入口，`AppState` 只读/缓存 |
| Postgres repository 接口与 model 不匹配 | 参考 `internal/domain/repository/postgres.go` 模式；先补测试再补实现 |
| CLI 远程命令与 Server API 不同步 | CLI client 必须复用 Server 的 route/path/错误码定义 |
| gRPC 状态更新绕过 service | scheduler_service 统一走 `App.SessionService.UpdateState` |
| 改动面大导致测试雪崩 | 分 Phase 合并，每 Phase 单独 PR，确保 `go test ./...` 通过 |

---

## 十、下一步动作

1. 进入 Plan Mode，把 Phase 0 的 `pkg/local` + `internal/app/application.go` 写成详细实施计划。
2. 按 Phase 0 → Phase 5 顺序逐步推进。
3. 每个 Phase 一个独立 PR，commit message 用 Conventional Commits。
