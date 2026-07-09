# Python Worker Roadmap

> 周期：2026.07.09 起（独立 Python 侧跟踪）  
> 北极星：让 `hnsx-worker` 从当前骨架演进为**真实 SOTA Agent 接入、可治理、可观测、可评测**的 Python 能力执行平面。  
> 架构基线：`docs/server-design/service-architecture.md` §2 "Runtime Worker（Python）"  
> 协同基线：`docs/project_management/2026.07.09/v1_2_roadmap.md`（Go 控制面为主线）

> **本文只跟踪 Python 侧**。Go 控制面 / Console / Eval / Postgres / mTLS 等由对应文档负责。Python ↔ Go 的契约（proto / observation）若变化，先在 proto 仓库同步后再回到本文。

---

## 总览

| 里程碑 | 主题 | 预计周期 | 主要交付 |
|---|---|---|---|
| W0 | 骨架 + 契约 | ✅ done | proto + worker parent + subprocess + noop/echo |
| W1 | 真实 Agent 接入 | ✅ done | Anthropic / OpenAI / Ollama，streaming + tool use，多 turn 骨架 |
| W2 | proto_client 抽象 | ✅ done | runtime 与 wire format 完全解耦 |
| W3 | Tool Registry | 1 周 | `tools/registry.py` + `tools/{http,sql,python}.py` + Policy 拦截 |
| W4 | CLI Agent 接入 | 1 周 | Claude Code / Codex adapter，Policy + Sandbox 边界观测 |
| W5 | 多 Agent 编排 | 1-2 周 | `harness/{loader,runner,transition}.py`，supervisor mode |
| W6 | Policy + Sandbox + Store | 1-2 周 | `policy/engine.py` + `sandbox/backend.py` + `store/backend.py` + Secret |
| W7 | 性能与韧性 | 1 周 | 预 fork pool、优雅 drain、Domain invalidation 完整处理 |
| W8 | Eval 接入 | 1 周 | Python scorer：exact / contains / jmespath / structured_match / llm-judge |

---

## W0：骨架 + 契约（done）

### 交付

- `proto/hnsx/v1/worker.proto` + `observation.proto` + Go / Python 双端 stub
- Worker parent process：`worker_service.py`（Register / Heartbeat / PullSession / StreamChannel）
- Session runtime subprocess：`session_runtime.py`（stdin JSON / stdout JSONL / SIGTERM）
- Adapter registry：`adapters/{base,noop,echo}.py`
- `pyproject.toml` + venv + `make proto-py` / `make worker-test` / `make worker-install`

### 验收

- `hnsx-worker check-proto` 通过
- `make worker-test` 通过
- 端到端：Go server → Python worker → subprocess noop session → SSE observation

---

## W1：真实 Agent 接入（done）

### 交付

- `adapters/anthropic.py`：`invoke` + `invoke_stream` + 完整 tool use 解析 + cost 记录
- `adapters/openai.py`：`invoke` + `invoke_stream` + tool use 解析 + cost 记录
- `adapters/ollama.py`：`invoke`（无 streaming / 无 tool use）
- `adapters/base.py`：`StreamChunk` / `ToolCall` / `Cost` 类型
- `session_executor.py`：单 turn 流式输出（`agent_text_delta` / `agent_tool_call` / `agent_cost`）+ 多 turn loop（max_turns + stub tool result）
- `_build_messages(input)` 适配器内部约定：executor 通过 `input["_messages"]` 注入多 turn history
- 单元测试：`tests/test_real_adapters.py`（mock httpx）、`tests/test_openai_streaming.py`、`tests/test_executor_streaming.py`

### 验收

- Anthropic / OpenAI 单元测试全过（mock httpx）
- 多 turn 跑满 `max_turns` 后正确 `truncated` 终止
- 流式 chunks 全部 emit 为 observation（无双漏）

### 已知缺口（推到 W3 / W6）

- `_stub_tool_result` 是 placeholder；M3 接 `ToolRegistry` 时替换
- 多 turn 的 tool call 结果不实际执行工具，只是 echo
- 没有 budget / denied tools / human approval 拦截

---

## W2：proto_client 抽象（done）

### 交付

- `proto_client/__init__.py` + `messages.py`（23 个 dataclass）+ `client.py`（`ControlPlaneClient`）
- `worker_service.py` / `config.py` 重构：完全去除 `worker_pb2` / `observation_pb2` import
- 新测试 `tests/test_proto_client.py`（8 个，直接测 `ControlPlaneClient`）

### 验收

- `grep -rn "_pb2" hnsx_worker/` 在 `proto_client/` 之外无命中（`__main__.py` 的 `check-proto` CLI 例外）
- 44 个测试全过

### 收益

- 未来加 mTLS、observation 字段变化、proto 字段调整，只改 `proto_client/` 一处
- Mock server 测试继续直接用 proto（那是 server-side mock，本来就在模拟 wire）

---

## W3：Tool Registry

### 目标

让 API Agent 通过 tool calling 调用外部能力，同时所有调用都过 Policy 拦截与审计。

### 任务

- [ ] **`tools/registry.py`**
  - [ ] 定义 `Tool` / `ToolRegistry` 抽象（按 name / capability 查找）
  - [ ] 调用前走 `policy.engine.check(tool_call, ctx)` 拦截
  - [ ] 调用后 emit `tool_call` / `tool_result` / `policy_check` / `policy_violation` observation

- [ ] **`tools/http.py`**
  - [ ] GET / POST / PUT / DELETE 实现（基于 httpx）
  - [ ] 支持 `{secret.XXX}` 占位符注入（header / query / body）
  - [ ] 支持 timeout / retry / status code 白名单

- [ ] **`tools/sql.py`**
  - [ ] 基于 SQLAlchemy 2.x 或 raw SQL（asyncpg）
  - [ ] 默认只读模式（`SELECT` only），写操作需 policy 显式 allow

- [ ] **`tools/python.py`**
  - [ ] 在 sandbox 内执行一段 Python 代码
  - [ ] stdout / stderr / return value 捕获为 observation

- [ ] **executor 改造**
  - [ ] `_run_multi_turn` 把 `_stub_tool_result` 换成 `_tool_registry.call(name, input)`
  - [ ] tool call 错误（policy violation / execution error）正确 emit 并终止 turn

### 验收

- 一个 example domain 跑通 `tool_call` → 真 HTTP 调用 → `tool_result` → 下一轮 LLM 推理
- Denied tool 被 Policy 拦截，session emit `policy_violation`
- Secret 注入不泄漏到 observation payload（仅记 `name`，不记 `value`）

---

## W4：CLI Agent 接入（Claude Code / Codex）

### 目标

把 Claude Code / Codex CLI 当 Adapter 接入；Tool 层对它们只做**约束与审计**，不复刻 shell / file / edit 能力。

### 任务

- [ ] **`adapters/claude_code.py`**
  - [ ] 用 `subprocess` 起 `claude` CLI（或 stdin/stdout JSON 协议）
  - [ ] 解析 stdout 的事件，转成 `StreamChunk`（text / tool_use）
  - [ ] `claude.api_key_env` / `claude.timeout_seconds` / `claude.workdir`

- [ ] **`adapters/codex.py`**
  - [ ] 同上，针对 `codex` CLI

- [ ] **`tools/` 的 CLI Agent 适配**
  - [ ] 对 shell / file_write / file_delete / edit 操作，**Policy 拦截**（不允许默认通过）
  - [ ] 任何被允许的操作 emit `tool_call` observation（name + input + 触发策略）
  - [ ] 与 W3 共享 Tool Registry，但走"只读声明"路径

- [ ] **`adapters/registry.py`**
  - [ ] 适配器 lazy import：`claudecode` / `codex` 需要本地 CLI 安装
  - [ ] 测试用 `pytest.fixture` 注册 fake CLI adapter

### 验收

- 一个 example domain 用 `claudecode` adapter 跑通（用 fake CLI 输出验证 observation 流）
- Policy denied 的操作正确被拦截、emit `policy_violation`
- CLI Agent 不在 `_stub_tool_result` 路径下走 Tool Registry（它们自带工具）

---

## W5：多 Agent 编排（Supervisor / Hierarchical / Autonomous）

### 目标

实现 `session.mode: supervisor` 的真编排：supervisor agent 产生 routing decision，Harness 做 transition。

### 任务

- [ ] **`harness/loader.py`**
  - [ ] 加载并引用校验 DomainSpec（agent / prompt / tool / skill 引用关系）
  - [ ] 提前报错：缺引用 / 循环引用 / 必填字段缺失

- [ ] **`harness/transition.py`**
  - [ ] 实现 JMESPath 表达式求值（用 `jmespath` 库）
  - [ ] 支持 `$.observations[-1].output.intent == 'billing'` 等 transition
  - [ ] 返回 `to: str` / `reason: str` / `fallback: bool`

- [ ] **`harness/runner.py`**
  - [ ] 实现 supervisor loop：supervisor agent 产生 `routing_decision` observation
  - [ ] 根据 transition 切换 specialist agent
  - [ ] specialist 完成后根据 exit 条件结束或返回 supervisor
  - [ ] emit `supervisor_decision` / `specialist_start` / `specialist_end` observation

- [ ] **`session_executor.py` 改造**
  - [ ] `supervisor` / `hierarchical` / `autonomous` mode 不再 `NotImplementedError`
  - [ ] 把控制权交给 `harness.runner.run(...)`

### 验收

- `claude-triage` example domain 跑通：用户问题 → triage agent 路由 → billing/technical specialist 回答
- `routing_decision` observation 包含 `to` / `reason` / `confidence` 字段
- 路由错误（所有 transition 不命中）时 session 标记 `failed` 并给出原因

---

## W6：Policy + Sandbox + Store + Secret

### 目标

让运行时约束真正生效，并统一存储抽象。

### 任务

- [ ] **`policy/engine.py`**
  - [ ] `budget` 检查：累计 cost / max_cost_usd
  - [ ] `allowed_tools` / `denied_tools` 检查
  - [ ] `human_approval` 触发：暂停 session，等待审批
  - [ ] 每次检查 emit `policy_check` / `policy_violation` observation

- [ ] **`sandbox/backend.py` + 子类**
  - [ ] 定义 `SandboxBackend` 抽象
  - [ ] `none` / `process`（Linux namespace + seccomp，可选）/ `container`（Docker / Podman）

- [ ] **`store/backend.py` + 子类**
  - [ ] 定义 `StoreBackend` 抽象
  - [ ] `in_memory`（默认）
  - [ ] `postgres`（跨 turn 长期 context）
  - [ ] `redis`（ephemeral，跨 turn 短期）

- [ ] **Secret 注入**
  - [ ] Control Plane 解析 `{secret.XXX}` 占位符，注入 `SessionRequest.secrets`
  - [ ] session_runtime.py 通过环境变量接收 secrets
  - [ ] audit log 记录 secret **访问**（不记录值）

- [ ] **`session_executor.py` 集成**
  - [ ] turn 开头调 `policy.check_budget(...)`
  - [ ] tool_call 调 `policy.check_tool(...)`
  - [ ] store 跨 turn 保持 messages / state

### 验收

- 超预算 session 被自动 pause / failed
- Denied tool 调用被 policy 拦截
- human_approval 触发后 session 进入 `paused`，人工审批后继续
- Container sandbox 能限制 tool 执行环境（用 `python:3.11-slim` 跑 `tools.python`）
- Store 跨 turn 保持 context（重启 subprocess 后可恢复）

---

## W7：性能与韧性

### 目标

解决 V1.1 已知瓶颈 + 完善 drain / invalidation 处理。

### 任务

- [ ] **预 fork pool**
  - [ ] 启动时预热 N 个 `session_runtime` 子进程
  - [ ] `_on_session` 从 pool 取，不用每次 fork（~100ms → ~5ms）

- [ ] **优雅 drain**
  - [ ] 收到 `drain` 命令后停止 pull 新 session
  - [ ] 等待当前 session 跑完（或到 deadline）
  - [ ] 退出 `pull_loop` 并 close gRPC channel

- [ ] **Domain invalidation**
  - [ ] 收到 `invalidate(domain_id, version)` 后从本地 cache 移除
  - [ ] 不影响正在跑的 session

- [ ] **Session-level timeout**
  - [ ] `SessionAssignment.session_timeout_seconds` 实际生效（subprocess 内 schedule kill）
  - [ ] 超时 emit `session_end{state: failed, reason: timeout}`

- [ ] **可重入错误处理**
  - [ ] gRPC channel transient errors 不重启 worker（backoff reconnect）
  - [ ] subprocess crash 后正确 emit `session_end{state: failed}` 而不是 worker crash

### 验收

- 100 sessions 并发跑（4 worker slot），平均启动延迟 < 20ms（pool 命中）
- Drain 后不接新 session；所有进行中 session 在 deadline 内完成
- Channel 断 5 秒后自动重连，期间 observation 在 `_obs_queue` 排队

---

## W8：Eval 接入

### 目标

支持 Eval 平台化的 Python 侧 scorer。

### 任务

- [ ] **`eval/scorers.py`**
  - [ ] `exact` / `contains` / `regex` / `jmespath`
  - [ ] `structured_match`（按 JSON schema 比对）
  - [ ] `llm_judge`（用低成本 model 调用 judge prompt）
  - [ ] 每种 scorer emit `eval_score{case_id, scorer, score, details}` observation

- [ ] **`session_executor.py` 集成**
  - [ ] 检测 `config.get("eval_case")` 存在时，session_end 后自动跑 scorer
  - [ ] 把结果通过 `SessionFinalResult.result["eval_scores"]` 写回

- [ ] **EvalSet 触发**
  - [ ] Go 端 `RunEval` 把 EvalCase 注入 `SessionRequest.eval_case`
  - [ ] Python 端读完自动跑并回写

### 验收

- 一个 example domain 配 EvalSet（10 个 case）跑通：每个 case 的 score 出现在 `EvalRun` 报告
- `llm_judge` 用 `claude-haiku-4-5` judge，能区分 "good" / "ok" / "bad" 三档
- Baseline 对比支持（Go 侧 `EvalRun.baseline_run_id`）

---

## 跨里程碑依赖

```text
W3 (Tool) ──▶ W4 (CLI Agent) ──▶ W5 (Orchestration) ──▶ W6 (Policy/Sandbox/Store)
  │                                    │                          │
  ▼                                    ▼                          ▼
W7 (Performance) ◀──────────────── W8 (Eval) ◀──────────────────┘
```

W3 → W5 必须串行（W5 依赖 W3 的 Tool Registry 走 supervisor 路由）。
W6 可以和 W4/W5 并行（独立子模块）。
W7 / W8 不依赖前面的具体实现，只依赖 `session_runtime` 的稳定。

---

## 本周（2026.07.09）已完成

1. ✅ **W1 收尾**：OpenAI streaming + tool use + executor 多 turn loop
2. ✅ **W2 完成**：proto_client 抽离，runtime 完全无 `*_pb2` import
3. ✅ **44 个测试全过**：`make worker-test`

## 下周（W3 启动）

1. 启动 `tools/registry.py` + `tools/http.py` 骨架
2. `_run_multi_turn` 把 `_stub_tool_result` 换成 `ToolRegistry.call(...)`
3. 一个 example domain（`tools-demo`）跑通单 tool 调用

---

## 不在这轮做的事

- 多租户隔离（auth token 透传在 W2 已经预留，租户隔离逻辑放到更后期）
- Python worker 自身的 dashboard / metrics endpoint（由 Go telemetry / OTLP 统一处理）
- Python worker 的分布式调度（保持单进程 + 多 subprocess 形态，水平扩展交给 Go scheduler）
- 把现有 `pkg/session/` / `pkg/runtime/` 的 Go 代码迁到 Python（架构明确说 Go 留在调度面）