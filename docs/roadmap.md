# HnsX Roadmap

> 在 [`design/Tech/V1/Initial_Architectrue.md`](../design/Tech/V1/Initial_Architectrue.md)
> §10 的 4 阶段基础上展开。Phase 1-6 构成 v1.0；Phase 7-9 推到 v1.x / v2.0。

## TL;DR

| Phase | 时长 | 里程碑 | v1.0 |
|---|---|---|---|
| 0 | 1 d | 骨架（6 crate + 3 example + CLI） | ✅ shipped (`a8c75c0`) |
| **1** | 2-3 周 | 本地端到端（YAML → workflow → 流式） | ⏳ |
| **2** | 2-3 周 | Sandbox + Claude Code CLI（Linux） | ⏳ |
| **3** | 1-2 周 | 工具层（HTTP / Shell / SQL / Python） | ⏳ |
| **4** | 1-2 周 | 剩余 adapter + MemoryBackend 多后端 | ⏳ |
| **5** | 3-4 周 | 控制面 gRPC（Registry / Scheduler / Discovery） | ⏳ |
| **6** | 2-3 周 | 可观测性 + Web UI | ⏳ |
| 7 | 3-4 周 | Build / Deploy（Docker / K8s / Lambda） | v1.x |
| 8 | 2-3 周 | Python SDK（PyO3） | v1.x |
| 9 | 持续 | 生态（公开 Registry / TS SDK / 更多 adapter） | v2.0 |

v1.0 总投入约 **15-19 周**（4-5 个月）。每个 Phase 独立可合并、可演示。

---

## Phase 1 — 本地端到端运行时

**Goal**: `hnsx run --domain foo.yaml --trigger '{}'` 真正跑起来并流式输出 Chunk。

| # | Deliverable | 位置 |
|---|---|---|
| 1.1 | `DomainLoader`：YAML → `Arc<dyn Domain>`（不执行） | `hnsx-core/loader.rs` |
| 1.2 | Workflow 引擎：petgraph，顺序 / `condition` / `output` 绑定 | `hnsx-core/workflow.rs` |
| 1.3 | `InMemoryBackend`（默认） | `hnsx-core/memory.rs` |
| 1.4 | OpenAI adapter（HTTP + SSE streaming） | `hnsx-adapter/openai.rs` |
| 1.5 | Anthropic adapter（HTTP + SSE streaming） | `hnsx-adapter/anthropic.rs` |
| 1.6 | Noop adapter（CI 用） | `hnsx-adapter/noop.rs` |
| 1.7 | 最小 telemetry（in-memory + JSONL 写 `~/.hnsx/traces/`） | `hnsx-core/telemetry.rs` |
| 1.8 | `hnsx run` 真接 `Domain::invoke` 流式打印 | `hnsx-cli/commands/run.rs` |
| 1.9 | 单元 + 一个 e2e 集成测试（noop + 2-step） | `crates/*/tests/` |

**Acceptance**
- `cargo test --workspace` 全绿
- 配置 `OPENAI_API_KEY`（且本地 Ollama 运行 `llama3.1`）后 `hnsx run --domain domains/financial-analysis/domain.yaml --trigger '{"ticker":"AAPL"}'` 跑通
- `hnsx run --domain domains/customer-service/domain.yaml --adapter noop`（用 noop）走完两个 step

**依赖**: 无
**风险**: OpenAI / Anthropic 流式协议偶有变更；用 `eventsource-stream` 解码 SSE 隔离

---

## Phase 2 — 跨平台 Sandbox + Claude Code CLI

**Goal**: `hnsx run --domain domains/code-review/domain.yaml` 在 Linux、macOS 本地开发环境都能跑通，且沙箱写操作不污染 host；云上可切换到 Container / Micro-VM backend。

| # | Deliverable | 位置 |
|---|---|---|
| 2.1 | `SandboxFactory`：按 platform / deployment 选择 backend | `hnsx-sandbox/factory.rs` |
| 2.2 | `none` + `process` 通用 backend（所有平台兜底） | `hnsx-sandbox/src/backend/{none,process}.rs` |
| 2.3 | Linux namespace + landlock + seccomp + cgroups v2 | `hnsx-sandbox/src/backend/linux/` |
| 2.4 | macOS seatbelt / posix_spawn + rlimit 进程级沙箱 | `hnsx-sandbox/src/backend/macos/` |
| 2.5 | Windows job object + ACL 进程级沙箱（best-effort） | `hnsx-sandbox/src/backend/windows/` |
| 2.6 | OCI container backend（本地 Docker / 云上 containerd） | `hnsx-sandbox/src/backend/container.rs` |
| 2.7 | `SandboxSpec.runtime` 字段：`auto/linux-namespace/macos-seatbelt/.../container/micro-vm` | `hnsx-core/sandbox.rs` |
| 2.8 | `SandboxSpec` 补全字段：filesystem.mounts / commands.whitelist / network.domains / resources.quota / timeout.{total,idle} | `hnsx-core/sandbox.rs` |
| 2.9 | Claude Code CLI adapter（按 runtime 选择沙箱内 spawn + 流式 stdout） | `hnsx-adapter/claude_code.rs` |
| 2.10 | PII 行级检测（`Chunk::error`） | `hnsx-core/telemetry.rs` 钩子 |
| 2.11 | 沙箱 RAII 销毁（drop 时清理 namespace/cgroup/container） | `hnsx-sandbox/src/backend/*` |
| 2.12 | CI 矩阵（Linux + macOS + Windows best-effort） | `.github/workflows/ci.yml` |

**Acceptance**
- Linux 上 `hnsx run --domain domains/code-review/domain.yaml` 走完一个真实 PR review，沙箱内写入不污染 host
- macOS 上以 `process` 或 `macos-seatbelt` backend 跑通同一 domain（强度降级但行为一致）
- `cargo test -p hnsx-sandbox` 在所有支持平台绿
- 不支持的 backend 请求给出清晰错误，不 panic

**依赖**: Phase 1
**风险**
- 各平台沙箱 API 差异大，v1 先把 `process` 作为通用兜底
- Landlock 需要 kernel ≥ 5.13（探测 + 降级到 seccomp）
- Container / Micro-VM backend 需要本地运行时可用；CI 用 `none`/`process` 兜底
- seccomp 调试成本高（先 deny 危险 syscall）

---

## Phase 3 — 工具层

**Goal**: AgentSpec 能 opt-in 用 HTTP / Shell / SQL / Python 工具。

| # | Deliverable | 位置 |
|---|---|---|
| 3.1 | `Tool` trait + `ToolSpec` | `hnsx-core/tool.rs` |
| 3.2 | `hnsx-tool/http.rs` — GET/POST + Bearer/OAuth + 超时 | `hnsx-tool/http.rs` |
| 3.3 | `hnsx-tool/shell.rs` — 跑在 sandbox 内，whitelist | `hnsx-tool/shell.rs` |
| 3.4 | `hnsx-tool/sql.rs` — Postgres（`sqlx`）+ SQLite（`rusqlite`） | `hnsx-tool/sql.rs` |
| 3.5 | `hnsx-tool/python.rs` — sandbox 内 Python 脚本 | `hnsx-tool/python.rs` |
| 3.6 | 工具注册表 + AgentSpec 解析 | `hnsx-core/agent_factory.rs` |
| 3.7 | 重构 `financial-analysis` 让 `quoter` 用 `http` 工具 | `domains/financial-analysis/` |

**Acceptance**: 工具单测可独立跑（mock 外部依赖）；Python tool 沙箱受网络/超时限制。
**依赖**: Phase 2（Shell/Python 工具依赖 sandbox）
**风险**: SQL 连接池多 Agent 共享策略；Python sandbox 的 import 限制

---

## Phase 4 — 剩余 adapter + MemoryBackend 多后端

| # | Deliverable |
|---|---|
| 4.1 | Ollama adapter（HTTP + NDJSON 流式） |
| 4.2 | Codex CLI adapter |
| 4.3 | Custom（OpenAI-compatible）adapter |
| 4.4 | `RedisBackend` / `PostgresBackend` / `SqliteBackend` |
| 4.5 | `MemoryConfig.backend` 字段 + 工厂 |

**Acceptance**: 6 个 Provider 全部至少 1 个 e2e 烟测；session 跨进程持久化（SQLite 或 Postgres）。
**依赖**: Phase 1
**风险**: Ollama 无官方 streaming 规范；Redis token window 截断策略

---

## Phase 5 — 控制面 gRPC

| # | Deliverable |
|---|---|
| 5.1 | `proto/hnsx/v1/*.proto` 完整接口 |
| 5.2 | tonic server 启动 |
| 5.3 | Registry（Domain 上下架 + 版本） |
| 5.4 | Scheduler（AgentInstance 注册 / 心跳 / 负载均衡） |
| 5.5 | Discovery（按 tag / region / capability 查询） |
| 5.6 | Telemetry aggregation → Postgres + Prometheus exporter |
| 5.7 | mTLS（rustls；SPIFFE 留到后续） |
| 5.8 | Session store（跨域共享） |

**Acceptance**: 两个本地 runtime 注册到同一 control plane；trace 聚合查询通；`grpcurl` 烟测所有 RPC。
**依赖**: Phase 1（runtime API 稳定）
**风险**: gRPC 双向流 + 重连复杂；scheduler 一致性 v1 先单 leader

---

## Phase 6 — 可观测性 + Web UI

| # | Deliverable |
|---|---|
| 6.1 | OpenTelemetry SDK 接入（`opentelemetry` + `tracing-opentelemetry`） |
| 6.2 | Prometheus exporter（`/metrics`） |
| 6.3 | `hnsx traces / metrics / logs` 真接 control plane |
| 6.4 | Web UI（React + Vite）：Trace 时间线、Agent 状态、Domain 列表 |
| 6.5 | 成本跟踪：每 invoke 算 $，domain 级汇总 |

**Acceptance**
- Grafana / Prometheus 能拉到 HnsX 自定义指标
- Web UI 加载后能列出当前所有 domain 和最近 trace
- Token 成本在 CLI `metrics` 输出里能看到

**依赖**: Phase 5
**风险**: Web UI 新语言栈，CI 复杂度上升

---

## v1.0 Exit Criteria（Phase 1-6 全部完成）

- [ ] 6 个 Provider 都有真实实现（OpenAI / Anthropic / Claude Code / Codex / Ollama / Custom）
- [ ] 4 个 Tool 都有真实实现（HTTP / Shell / SQL / Python）
- [ ] Linux 上能跑通 `code-review` 域（沙箱 + Claude Code 端到端）
- [ ] 多 runtime 实例可注册到控制面，trace 聚合查询可用
- [ ] Web UI 能查看 trace、agent 状态、domain 列表
- [ ] CI 矩阵：Linux（沙箱测试）+ macOS（开发）
- [ ] `cargo clippy --workspace -- -D warnings` 0 警告
- [ ] `cargo test --workspace` 全绿 + 至少 1 个跨域 e2e 测试
- [ ] 文档：README + `docs/roadmap.md` + `docs/architecture.md`（从 design/ 提升）

---

## Phase 7-9（v1.x / v2.0，不在 v1.0 范围内）

**Phase 7 — Build & Deploy**（v1.x, 3-4 周）
- 7.1 Domain 打包格式（OCI 风格 tarball：yaml + 资源 + 签名）
- 7.2 `hnsx build`
- 7.3 Docker / Kubernetes（CRD + Operator + Helm）/ Lambda targets
- 7.4 `hnsx deploy` 通用调度

**Phase 8 — Python SDK**（v1.x, 2-3 周）
- 8.1 `hnsx-python`（PyO3 + maturin 打包 wheel）
- 8.2 Domain / AgentSpec 构造器（流式 API）
- 8.3 异步 invoke 客户端（asyncio + streaming）
- 8.4 Python 端 Step / Hook 实现
- 8.5 类型 stub + 文档 + 教程 notebook

**Phase 9 — 生态**（v2.0, 持续）
- 9.1 公开 Registry + CLI `publish` / `install`
- 9.2 TypeScript SDK（N-API 优先，WASM 备选）
- 9.3 更多 adapter（Pi、本地 GGUF/MLX）
- 9.4 模板 domain 库（PR review / triage / doc Q&A）
- 9.5 社区运营

---

## 横切关注点（不要等末期再补）

| 关注点 | 引入时机 | 备注 |
|---|---|---|
| **Secrets 管理** | Phase 1.4 第一个 HTTP adapter | 从 env 起步，Phase 5 接 Vault |
| **AuthN / AuthZ** | Phase 5 控制面 | OAuth2 + RBAC，Phase 1-4 本地不强制 |
| **成本跟踪** | Phase 1.7 trace 一起 | 早算 $ 比回头补历史数据省事 |
| **CI 矩阵** | Phase 2 起 | Linux（沙箱）+ macOS（开发） + Windows best-effort |
| **版本化 / 兼容** | Phase 1 末 | `DomainSpec` 语义版本 + 迁移工具 |
| **日志脱敏** | Phase 1.7 一起 | PII 检测要早做，事后清洗代价高 |
| **Examples 累积** | 持续 | 每个 Phase 至少新加 1 个 domain example |

---

## 决策记录（2026-07-06）

- **沙箱处理**：Sandbox 是跨平台隔离契约。`hnsx-sandbox` 提供 `none`、`process`、平台特定（Linux namespace / macOS seatbelt / Windows job object）以及云 backend（container / micro-vm）。`auto` runtime 按平台/部署目标自动选择最佳 backend。
- **Provider 字段**：用 enum（`kebab-case` serde），编译期校验。如果后续需要任意 provider 再切 `String`。
- **Cargo.lock**：commit。Workspace 产出 binary (`hnsx`)，遵循 Rust 社区惯例。
- **共享类型**：全在 `hnsx-core`，不拆 `hnsx-types`（YAGNI）。
- **v1.0 范围**：Phase 1-6。Build/deploy 和 SDK 推到 v1.x。
- **Phase 1 顺序**：1.1 → 1.9 严格顺序推进，每个子步独立可合并。
