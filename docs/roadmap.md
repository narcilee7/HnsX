# HnsX Roadmap

> 在 [`design/Tech/V1/Initial_Architectrue.md`](../design/Tech/V1/Initial_Architectrue.md)
> §10 的 4 阶段基础上展开。Phase 1-6 构成 v1.0；Phase 7-9 推到 v1.x / v2.0。

## TL;DR

| Phase | 时长 | 里程碑 | v1.0 |
|---|---|---|---|
| 0 | 1 d | 骨架（6 crate + 3 example + CLI） | ✅ shipped (`a8c75c0`) |
| **1** | 2-3 周 | 本地端到端（YAML → workflow → 流式） | ✅ |
| **2** | 2-3 周 | Sandbox + Claude Code CLI（跨平台） | ✅ |
| **3** | 1-2 周 | 工具层（HTTP / Shell / SQL / Python） | ✅ |
| **4** | 1-2 周 | 剩余 adapter + MemoryBackend 多后端 | ✅ |
| **5** | 3-4 周 | 控制面 gRPC（Registry / Scheduler / Discovery / Telemetry） | ✅ |
| **6** | 2-3 周 | 可观测性 + Web UI | ✅ |
| **7** | 3-4 周 | Build / Deploy（package + Docker） | ✅ |
| 8 | 2-3 周 | Python SDK（PyO3） | ⏸ 已规划 |
| 9 | 持续 | 生态（公开 Registry / TS SDK / 更多 adapter） | v2.0 |

v1.0 总投入约 **15-19 周**（4-5 个月）。每个 Phase 独立可合并、可演示。

当前状态：Phase 0-7 已完成并在 `feat/init` 分支持续迭代；Phase 8（Python SDK）已规划方案但暂缓实施，优先补齐文档与端到端验证。

---

## Phase 1 — 本地端到端运行时 ✅

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

## Phase 2 — 跨平台 Sandbox + Claude Code CLI ✅

**Goal**: `hnsx run --domain domains/code-review/domain.yaml` 在 Linux、macOS 本地开发环境都能跑通，且沙箱写操作不污染 host；云上可切换到 Container / Micro-VM backend。

| # | Deliverable | 状态 | 位置 |
|---|---|---|---|
| 2.1 | `SandboxFactory`：按 platform / deployment 选择 backend | ✅ | `hnsx-sandbox/factory.rs` |
| 2.2 | `none` + `process` 通用 backend（所有平台兜底） | ✅ | `hnsx-sandbox/src/backend/{none,process}.rs` |
| 2.3 | Linux namespace + chroot（mount namespace 骨架；landlock/seccomp/cgroup 留后续） | ✅ | `hnsx-sandbox/src/backend/linux.rs` |
| 2.4 | macOS seatbelt / posix_spawn + rlimit 进程级沙箱（`process` backend 覆盖） | ⏳ | `hnsx-sandbox/src/backend/macos/` |
| 2.5 | Windows job object + ACL 进程级沙箱（best-effort） | ⏳ | `hnsx-sandbox/src/backend/windows/` |
| 2.6 | OCI container backend（本地 Docker / 云上 containerd） | ⏳ | `hnsx-sandbox/src/backend/container.rs` |
| 2.7 | `SandboxSpec.runtime` 字段：`auto/linux-namespace/macos-seatbelt/.../container/micro-vm` | ✅ | `hnsx-core/sandbox.rs` |
| 2.8 | `SandboxSpec` 补全字段：filesystem.mounts / commands.whitelist / network.domains / resources.quota / timeout.{total,idle} | ⏳ | `hnsx-core/sandbox.rs` |
| 2.9 | Claude Code CLI adapter（按 runtime 选择沙箱内 spawn + 流式 stdout） | ✅ | `hnsx-adapter/claude_code.rs` |
| 2.10 | PII 行级检测（`Chunk::error`） | ✅ | `hnsx-core/pii.rs` + `workflow.rs` |
| 2.11 | 沙箱 RAII 销毁（`TempDir` + `Child` kill） | ✅ | `hnsx-sandbox/src/backend/*` |
| 2.12 | CI 矩阵（Linux + macOS） | ✅ | `.github/workflows/ci.yml` |

**Acceptance**
- ✅ macOS 上 `hnsx run --domain domains/code-review/domain.yaml --trigger '{"diff":"..."}'` 跑通（`process` backend 自动兜底）
- ✅ Linux namespace backend 代码 + Linux-only 测试（`chroot_isolates_writes_from_host`）
- ✅ `cargo test --workspace` 全绿
- ✅ CI 矩阵 `ubuntu-latest` + `macos-latest`
- ⏳ landlock / seccomp / cgroup / container / micro-vm backend 留到后续迭代

**依赖**: Phase 1
**风险**
- 各平台沙箱 API 差异大，v1 先把 `process` 作为通用兜底
- Landlock 需要 kernel ≥ 5.13（探测 + 降级到 seccomp）
- Container / Micro-VM backend 需要本地运行时可用；CI 用 `process` 兜底
- seccomp 调试成本高（先 deny 危险 syscall）

---

## Phase 3 — 工具层 ✅

**Goal**: AgentSpec 能 opt-in 用 HTTP / Shell / SQL / Python 工具。

| # | Deliverable | 状态 | 位置 |
|---|---|---|---|
| 3.1 | `Tool` trait + `ToolSpec` | ✅ | `hnsx-core/tool.rs` |
| 3.2 | `hnsx-tool/http.rs` — GET/POST + Bearer/OAuth + 超时 | ✅ | `hnsx-tool/http.rs` |
| 3.3 | `hnsx-tool/shell.rs` — 跑在 sandbox 内，whitelist | ✅ | `hnsx-tool/shell.rs` |
| 3.4 | `hnsx-tool/sql.rs` — SQLite（`rusqlite`）+ Postgres 占位 | ✅/⏳ | `hnsx-tool/sql.rs` |
| 3.5 | `hnsx-tool/python.rs` — sandbox 内 Python 脚本 | ✅ | `hnsx-tool/python.rs` |
| 3.6 | 工具注册表 + AgentSpec 解析 | ✅ | `hnsx-adapter/tools.rs` |
| 3.7 | 重构 `financial-analysis` 让 `quoter` 用 `http` 工具 | ✅ | `domains/financial-analysis/` |
| 3.8 | `genai` adapter tool-use loop（HTTP/Shell/SQL/Python） | ✅ | `hnsx-adapter/genai.rs` |

**Acceptance**: 工具单测可独立跑（mock 外部依赖）；Python tool 沙箱受网络/超时限制。
**依赖**: Phase 2（Shell/Python 工具依赖 sandbox）
**风险**: SQL 连接池多 Agent 共享策略；Python sandbox 的 import 限制

---

## Phase 4 — 剩余 adapter + MemoryBackend 多后端 ✅

| # | Deliverable | 状态 | 位置 |
|---|---|---|---|
| 4.1 | Ollama adapter（HTTP + NDJSON 流式） | ✅ | `hnsx-adapter/ollama.rs` |
| 4.2 | Codex CLI adapter | ✅ | `hnsx-adapter/codex.rs` |
| 4.3 | Custom（OpenAI-compatible）adapter | ✅ | `hnsx-adapter/custom.rs` |
| 4.4 | `RedisBackend` / `PostgresBackend` / `SqliteBackend` | ✅/⏳ | `hnsx-core/memory.rs` |
| 4.5 | `MemoryConfig.backend` 字段 + 工厂 | ✅ | `hnsx-core/memory.rs` |

**Acceptance**: 6 个 Provider 全部至少 1 个 e2e 烟测；session 跨进程持久化（SQLite 或 Postgres）。
**依赖**: Phase 1
**风险**: Ollama 无官方 streaming 规范；Redis token window 截断策略

---

## Phase 5 — 控制面 gRPC ✅/⏳

| # | Deliverable | 状态 |
|---|---|---|
| 5.1 | `proto/hnsx/v1/*.proto` 完整接口 | ✅ |
| 5.2 | tonic server 启动 | ✅ |
| 5.3 | Registry（Domain 上下架 + 版本） | ✅ |
| 5.4 | Scheduler（AgentInstance 注册 / 心跳 / 负载均衡） | ✅ |
| 5.5 | Discovery（按 tag / region / capability 查询） | ✅ |
| 5.6 | Telemetry aggregation → SQLite | ✅ |
| 5.7 | mTLS（rustls；SPIFFE 留到后续） | ⏳ |
| 5.8 | Session store（跨域共享） | ⏳ |

**决策记录（2026-07-06）**：控制面状态不存内存，统一用 SQLite 持久化，便于本地单机/云单节点部署；后续按需要扩展 Postgres。

**Acceptance**: 两个本地 runtime 注册到同一 control plane；trace 聚合查询通；`grpcurl` 烟测所有 RPC。
**依赖**: Phase 1（runtime API 稳定）
**风险**: gRPC 双向流 + 重连复杂；scheduler 一致性 v1 先单 leader

---

## Phase 6 — 可观测性 + Web UI

| # | Deliverable | 状态 |
|---|---|---|
| 6.1 | OpenTelemetry SDK 接入（`tracing` instrument，可扩展 exporter） | ✅ |
| 6.2 | Prometheus exporter（`/metrics`） | ✅ |
| 6.3 | `hnsx traces / metrics / logs` 真接 control plane | ✅ |
| 6.4 | Web UI（React + Vite）：Domain 列表、Agent 状态、Trace 查看 | ✅ |
| 6.5 | 成本跟踪：每 invoke 算 $，domain 级汇总 | ✅ |

**Acceptance**
- Grafana / Prometheus 能拉到 HnsX 自定义指标
- Web UI 加载后能列出当前所有 domain 和最近 trace
- Token 成本在 CLI `metrics` 输出里能看到

**依赖**: Phase 5
**风险**: Web UI 新语言栈，CI 复杂度上升

---

## v1.0 Exit Criteria（Phase 1-6 全部完成）

- [x] 6 个 Provider 都有真实实现（OpenAI / Anthropic / Claude Code / Codex / Ollama / Custom）
- [x] 4 个 Tool 都有真实实现（HTTP / Shell / SQL / Python）
- [ ] Linux 上能跑通 `code-review` 域（沙箱 + Claude Code 端到端）
- [x] 多 runtime 实例可注册到控制面，trace 聚合查询可用
- [x] Web UI 能查看 trace、agent 状态、domain 列表
- [x] CI 矩阵：Linux（沙箱测试）+ macOS（开发）
- [x] `cargo clippy --workspace -- -D warnings` 0 警告
- [x] `cargo test --workspace` 全绿 + 至少 1 个跨域 e2e 测试
- [ ] 文档：README + `docs/roadmap.md` + `docs/architecture.md`（从 design/ 提升）

---

## Phase 7-9（v1.x / v2.0，不在 v1.0 范围内）

**Phase 7 — Build & Deploy**（v1.x, 3-4 周）
- ✅ 7.1 Domain 打包格式（gzip tarball + JSON manifest）
- ✅ 7.2 `hnsx build`
- ✅ 7.3 Docker 本地部署
- ⏳ 7.4 Kubernetes（CRD + Operator + Helm）
- ⏳ 7.5 AWS Lambda target
- ⏳ 7.6 `hnsx deploy` 通用调度（目前仅 docker）

**Phase 8 — Python SDK**（v1.x, 2-3 周）⏸ 已规划，暂不实施
- ⏸ 8.1 `hnsx-python`（PyO3 + maturin 打包 wheel）
- ⏸ 8.2 Domain / AgentSpec 构造器（流式 API）
- ⏸ 8.3 异步 invoke 客户端（asyncio + streaming）
- ⏸ 8.4 Python 端 Step / Hook 实现
- ⏸ 8.5 类型 stub + 文档 + 教程 notebook

**备注**：方案已设计完成（见 `hnsx-python` 计划）。优先级让位于补齐 v1.0 文档与验证，后续择机实施。

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

## 决策记录

- **沙箱处理**：Sandbox 是跨平台隔离契约。`hnsx-sandbox` 提供 `none`、`process`、平台特定（Linux namespace / macOS seatbelt / Windows job object）以及云 backend（container / micro-vm）。`auto` runtime 按平台/部署目标自动选择最佳 backend。
- **Provider 字段**：用 enum（`kebab-case` serde），编译期校验。如果后续需要任意 provider 再切 `String`。
- **Cargo.lock**：commit。Workspace 产出 binary (`hnsx`)，遵循 Rust 社区惯例。
- **共享类型**：全在 `hnsx-core`，不拆 `hnsx-types`（YAGNI）。
- **v1.0 范围**：Phase 1-6。Build/deploy 和 SDK 推到 v1.x。
- **Phase 1 顺序**：1.1 → 1.9 严格顺序推进，每个子步独立可合并。
- **Phase 6 可观测性**：
  - `hnsx-proto` 独立 crate，供 `hnsx-core`、`hnsx-control-plane`、`hnsx-cli` 共享 gRPC 类型，避免循环依赖。
  - `hnsx-core` 的 `Telemetry` 同时写本地 JSONL 和异步上报 control plane；gRPC reporter 失败只打 warning，不影响 workflow。
  - Web UI 为独立 React + Vite 应用，控制面 HTTP server 通过 `with_static_dir("web/dist")` 提供静态文件服务；本地开发用 Vite proxy 转发 `/api` 和 `/metrics` 到控制面 HTTP 端口（默认 50052）。
- **Phase 7 范围收缩**：先落地 Domain package 格式、`hnsx build`、`hnsx dev` 长运行模式以及 Docker 本地部署。K8s CRD/Operator/Helm 和 Lambda 放到 Phase 7 后续迭代。
- **Phase 7 运行时协议**：新增 `Runtime` gRPC service（`Trigger` 双向流），使 `hnsx dev` / 容器能接收外部触发。
- **Phase 8 暂缓**：Python SDK 方案已完成（PyO3 spec builder + grpcio async client），但暂不实施，优先补齐 v1.0 文档与端到端验证。
