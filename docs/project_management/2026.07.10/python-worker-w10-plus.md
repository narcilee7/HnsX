# Python Worker W10+ RoadMap：智能与 Harness 能力全面强化

> 目标：在 W9「生产就绪」基础上，让 Python Worker 从「稳定执行 Agent 代码」进化为「可自主学习、可深度集成 Harness、可对外提供标准化协议」的智能 Agent 底座。
>
> 时间窗口：2026.07.14 – 2026.08.31（7 周，W10–W16）
> 负责人：hnsx-worker squad
> 关联：hnsx-server Harness 核心、console UI、Node/Go SDK

---

## 1. 我们为什么要进入 W10+

W9 完成后，Python Worker 已经具备：

- 安全沙箱（nsjail / seccomp / cgroups）
- 可观测与成本治理
- Eval 平台化
- Worker 运维与韧性
- Policy 与人工审批

但这只是「把代码跑好」。接下来要回答的是：

1. **Worker 能不能让 Agent 更聪明？** 即长期记忆、自主规划、反思、学习。
2. **Worker 能不能让 Harness 更强？** 即通过标准化协议（MCP / A2A / ACP）接入外部 Agent 与工具。
3. **Python 生态能不能反哺平台？** 即 Python SDK、插件市场、社区贡献。
4. **生产安全能不能从被动转为主动？** 即动态策略、对抗测试、自动修复。

---

## 2. W10+ 总览

| 周 | Phase | 主题 | 核心产出 | 依赖 |
|---|---|---|---|---|
| W10 | Phase 1 | MCP Client 与外部工具生态 | `mcp_client` 工具；`mcp_servers` Domain 配置；SSE / stdio 双传输 | proto Tool 定义、server config |
| W11 | Phase 2 | 长期记忆与上下文管理 | Session Memory、Memory Store、自动摘要、向量检索 | 向量数据库选型 |
| W12 | Phase 3 | 高级 Agent 编排 | `agents/{react,plan_solve,multi_agent,reflection}` + `delegate_to` 工具 + `harness.orchestration.strategy` 路由；3 个示例域覆盖 react / plan_and_solve / multi_agent | Phase 2 memory、LLM 调用层 |
| W13 | Phase 4 | 智能 Eval 与 Harness 自进化 | `run_improvement_loop` + `RegressionStore` + `evolve_prompt` + `eval_self_check` 工具，全部接入 `execute_session` 的 EvalSet 分支并落端到端单测 | Eval 平台、Trace DB |
| W14 | Phase 5 | 人机协同与审批增强 | `approval/{bus,protocol}.py` + `request_human_approval` tool + `policy.approval` DSL；session_executor 已经在 tool dispatch 之前强制过 gate；customer-service 演示退款前审批 | server 审批 API、console |
| W15 | Phase 6 | Python SDK 与插件化 | `@hnsx/sdk-python` 发布、`hnsx_skill` 包规范、插件注册表 | worker 包结构 |
| W16 | Phase 7 | 主动安全与对抗治理 | 动态沙箱策略、对抗测试、red-team eval、自动修复建议 | W9 sandbox、policy |

---

## 3. Phase 1：MCP Client 与外部工具生态（W10）

### 3.1 目标
让 Python Worker 通过 [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) 调用外部工具，把 Harness 的 Tool 能力从「内置 Python 函数」扩展到「任意 MCP Server」。

### 3.2 关键任务

1. **MCP Client 工具**
   - 在 `hnsx_worker/tools/` 新增 `mcp_client.py`，实现 `call_mcp_server` 工具。
   - 支持 `stdio` 和 `SSE` 两种传输。
   - 支持一次会话内保持 MCP session，避免重复握手。

2. **Domain 配置扩展**
   - 在 `example-domains/` 新增 `mcp-demo/` Domain YAML：
     - 声明 `mcp_servers` 列表（name、transport、command/url、env）。
     - 通过 `policy` 控制允许调用的 MCP server/tool。

3. **工具发现与注册**
   - Worker 启动时读取 Domain 的 `mcp_servers`。
   - 对 stdio server 启动子进程并缓存；对 SSE server 建立持久连接。
   - 通过 `tools/list` 发现工具，动态注入当前 Session 的 ToolRegistry。

4. **安全与观测**
   - 所有 MCP 调用走现有 SandboxPolicy：超时、CPU/内存限制、网络白名单。
   - 生成 Observation：kind=`mcp_call`，记录 server、tool、参数摘要、latency、token/cost（若 MCP server 返回）。

### 3.3 验收标准
- `make worker-test` 全绿。
- 新增 `tests/tools/test_mcp_client.py`：stdio mock server 可通、SSE mock server 可通、超时失败被 policy 捕获。
- example-domain `mcp-demo` 可完成一次「调用 filesystem MCP 列出目录」的端到端 Trace。

### 3.4 文件路径（预期）
- `hnsx_worker/tools/mcp_client.py`
- `hnsx_worker/sandbox/mcp_transport.py`
- `hnsx_worker/config/mcp_server_config.py`
- `example-domains/mcp-demo/domain.yaml`
- `tests/tools/test_mcp_client.py`

---

## 4. Phase 2：长期记忆与上下文管理（W11）

### 4.1 目标
Worker 不再只依赖单次 Session 的上下文，而是能跨 Session 读取、写入、检索长期记忆。

### 4.2 关键任务

1. **Memory 抽象层**
   - 新增 `hnsx_worker/memory/` 模块：
     - `MemoryItem`：fact、preference、task_state、summary。
     - `MemoryStore` 接口：`add(session_id, item)`、`search(query, top_k)`、`get_recent(n)`。

2. **默认实现：本地 SQLite + 可选向量后端**
   - 默认用 SQLite 存储文本记忆（BM25/like 检索）。
   - 可选 `mem0` / `chroma` / `weaviate` 向量后端，通过配置切换。
   - 在 `pyproject.toml` 用 extras 安装：`pip install hnsx-worker[memory-chroma]`。

3. **自动摘要**
   - Session 结束时自动调用 LLM 生成摘要，写入 MemoryStore。
   - 支持配置摘要策略：每 N 条消息、每次工具调用、Session 结束。

4. **Memory 工具暴露给 Agent**
   - `memory_search(query)`
   - `memory_store(key, value, ttl)`
   - `memory_forget(key)`

5. **Harness 集成**
   - Trace 中新增 `memory_read` / `memory_write` Observation。
   - Domain YAML 新增 `memory` 字段：backend、embedding_model、max_items。

### 4.3 验收标准
- 跨 Session 的 Agent 能读取上一轮用户偏好。
- 新增 `tests/memory/test_memory_store.py`、`tests/memory/test_auto_summary.py`。
- 在 `example-domains/customer-service` 中演示「记住用户偏好的回复风格」。

---

## 5. Phase 3：高级 Agent 编排（W12）

### 5.1 目标
让 Python Worker 支持更复杂的 Agent 模式：规划、反思、多 Agent 协作、自主执行。

### 5.2 关键任务

1. **ReAct / Plan-and-Solve 框架**
   - 新增 `hnsx_worker/agents/`：
     - `ReActAgent`
     - `PlanAndSolveAgent`
   - 与现有 ToolRegistry 无缝集成。
   - 支持最大步数、循环检测、budget 上限。

2. **Reflection 模块**
   - 执行过程中可触发 `reflection_step`：
     - 检查当前进度 vs 目标。
     - 发现偏离时生成修正计划。
   - Trace 中新增 `reflection` Observation。

3. **Multi-Agent 协作**
   - 支持在一个 Session 内运行多个 sub-agent：
     - `delegate_to(agent_name, task)` 工具。
     - sub-agent 结果作为 Observation 返回给 parent。
   - 每个 sub-agent 可独立 budget、policy、memory。

4. **编排配置 DSL**
   - Domain YAML 新增 `orchestration.strategy`：
     - `direct`（默认，当前模式）
     - `react`
     - `plan_and_solve`
     - `multi_agent`

### 5.3 验收标准
- [x] `hnsx_worker/agents/{base,react_agent,plan_solve_agent,multi_agent,reflection}.py` + `tools/delegate.py` 实现，20 个 unit 测试（`tests/agents/test_{multi_agent,plan_solve_agent,react_agent,reflection}.py`）全过。
- [x] `session_executor.execute_session` 读 `harness.orchestration.strategy`，按 `direct / react / plan_and_solve / multi_agent` 路由到 `_run_strategy_agent(...)`。
- [x] `example-domains/react-demo` 用 `strategy: react`、`example-domains/financial-analysis` 用 `strategy: plan_and_solve`、`example-domains/code-review` 用 `strategy: multi_agent`（3 套策略各有一个 demo）。

---

## 6. Phase 4：智能 Eval 与 Harness 自进化（W13）

### 6.1 目标
让 Eval 不仅用于验收，还能驱动 Worker / Agent / Prompt 自动改进。

### 6.2 关键任务

1. **Eval-driven Improvement Loop**
   - 在 `hnsx_worker/eval/` 新增 `improvement_loop.py`：
     - 跑 eval → 识别失败模式 → 生成 prompt / rule 修正建议 → 人工确认后应用。
   - 输出 `improvement_report.json`。

2. **Regression Radar**
   - 每次 eval 与历史基线对比。
   - 自动检测 regressions，触发 CI 告警或 Slack 通知。

3. **Auto Prompt Evolution**
   - 基于成功/失败 Trace，自动尝试 prompt 变体。
   - 使用 LLM 生成 3-5 个候选 prompt，A/B eval 后推荐最优。
   - 人工审批后才能替换生产 prompt。

4. **Harness 元能力增强**
   - Worker 在执行 Agent 任务时，能主动调用 Harness 的 Eval 接口：
     - `eval_self_check(task, candidate_output)` 工具。
   - 让 Agent 自己判断输出质量。

### 6.3 验收标准
- [x] 新增 `tests/eval/test_improvement_loop.py`、`tests/eval/test_regression_radar.py`、`tests/eval/test_prompt_evolution.py`、`tests/tools/test_self_check.py`、`tests/eval/test_session_w13_pipeline.py`（端到端）—— 全部通过。
- [x] `execute_session` 在 `spec.improvement` 存在时自动串起 `run_improvement_loop` → `evolve_prompt` → `check_regressions` + 把当前 report 写入 `RegressionStore`，结果落到 `result["improvement_report"]` / `result["evolution_report"]` / `result["regression_check"]`。
- [x] `RegressionStore` 持久路径可控：构造参数 `path=` > 环境变量 `HNSX_REGRESSION_STORE` > 默认 `.hnsx/regression-radar.json`，取代了原先 `tempfile` 默认行为（重启即丢 baseline）。
- [x] `eval_self_check` tool 已在 `tools/factory._BUILTIN_TOOLS` 注册，`example-domains/claude-triage/domain.yaml` 已在 billing agent 上挂载。
- [ ] CI 中 regression radar 识别人为注入退化用例——下一步接入控制面时验证（本轮仅 worker 侧落地）。

---

## 7. Phase 5：人机协同与审批增强（W14）

### 7.1 目标
让高风险操作能优雅地挂起等待人类，且恢复后上下文不丢失。

### 7.2 关键任务

1. **可恢复的人类审批**
   - Agent 调用 `request_human_approval(reason, options)` 工具后：
     - Worker 将任务状态置为 `awaiting_human`。
     - 通过 server API 推送审批请求到 console / Slack / Email。
     - 人类回复后，Worker 从挂起点恢复。

2. **异步审批协议**
   - 定义 `ApprovalRequest` / `ApprovalResponse` 消息格式。
   - Worker 支持通过长轮询或 SSE 等待审批结果。

3. **协作编辑**
   - 人类可在审批过程中修改 Agent 生成的草稿。
   - 修改结果作为新 Observation 进入 Trace。

4. **Domain 配置**
   - YAML 中 `policy.approval.required_for` 支持规则：
     - 特定 tool
     - 涉及敏感资源
     - cost 超过阈值

### 7.3 验收标准
- [x] 新增 `tests/approval/test_human_in_the_loop.py` + `tests/approval/test_policy_approval.py`（覆盖：lifecycle、in-memory bus submit/respond/cancel/observation、`request_approval` blocking + stop-event + timeout + explicit deny、`HumanApprovalTool` approve/deny/edit/timeout/validation）—— 全部通过。
- [x] `approval/{bus,protocol}.py` 提供可插拔传输（in-memory 默认 + 抽象 `ApprovalBus`），36 个 unit + integration 测试覆盖全部状态机分支。
- [x] `policy.approval` DSL 解析 `tools` + glob `resources` + `cost_threshold_usd` 三个独立 trigger，`ApprovalPolicy.requires_approval(tool, input, *, projected_cost_usd)` 返回 `(needs, rule_label)`。
- [x] `example-domains/customer-service` 已经挂 `request_human_approval` tool + `policy.approval.required_for`（gate `issue_refund` / `export_customer_data` / `customer:*` 资源），billing-prompt 提示 agent 在退款前调用。
- [x] `session_executor._run_multi_turn` / `AgentLoopContext` 在 tool dispatch 之前先过 `_maybe_gate_for_approval`，审批被拒则返回 `ToolResult(error=…)` 不污染 observation 主路径。
- [ ] 控制面 server API（推送审批到 console / Slack / Email） + console 异步审批 UI — 属于前后端工作，本轮仅完成 worker 侧契约与可插拔总线。

---

## 8. Phase 6：Python SDK 与插件化（W15）

### 8.1 目标
让外部开发者能用 Python 为 HnsX 写 Skill / Tool / Agent，并发布为插件。

### 8.2 关键任务

1. **发布 `@hnsx/sdk-python`**
   - 当前 `hnsx_worker` 是运行时；把公共接口拆出为 `hnsx-sdk` 包：
     - `Skill` 装饰器
     - `Tool` 注册
     - `Observation` 创建
     - `Agent` 基类
   - 发布到内部 PyPI / 公网 PyPI（视项目策略）。

2. **Skill 包规范**
   - 定义 `hnsx_skill.toml`：
     - name、version、entrypoint、dependencies、permissions。
   - 支持 `hnsx skill install ./my-skill`。

3. **插件注册表**
   - 在 `hnsx-worker/` 新增 `skills/` 目录，存放官方 skill 示例。
   - 支持 Domain YAML 引用 skill：`skills: [hnsx-web-search, hnsx-memory]`。

4. **CLI 增强**
   - `hnsx skill init`：生成 skill 模板。
   - `hnsx skill test`：本地运行 skill 测试。
   - `hnsx skill publish`：打包上传。

### 8.3 验收标准
- `hnsx-sdk` 独立安装并通过测试。
- 新增 `tests/sdk/test_skill_decorator.py`。
- 至少 3 个官方 skill 示例：web-search、memory、slack-notification。

---

## 9. Phase 7：主动安全与对抗治理（W16）

### 9.1 目标
从「跑代码时隔离」升级为「主动识别、测试、修复安全风险」。

### 9.2 关键任务

1. **动态沙箱策略**
   - 基于 Agent 行为实时调整沙箱：
     - 检测到异常系统调用 → 收紧权限。
     - 检测到高频网络请求 → 限速/熔断。
   - 策略调整记录到 Observation。

2. **Red-Team Eval Suite**
   - 新增 `tests/redteam/`：
     - prompt injection
     - data exfiltration
     - tool misuse
     - jailbreak
   - 每个用例评估 Worker 的防御能力。

3. **自动修复建议**
   - 当 red-team eval 失败时，自动生成修复建议：
     - 增加 policy rule
     - 调整 prompt 安全前缀
     - 限制 tool 参数
   - 输出 `security_remediation_report.md`。

4. **合规审计**
   - Worker 定期输出安全审计日志：
     - 权限变更
     - 敏感数据访问
     - 异常行为统计

### 9.3 验收标准
- red-team eval 套件全跑通，基线报告记录初始分数。
- 动态沙箱策略在模拟攻击测试中生效。
- 安全修复建议文档化，至少修复 3 个已知风险模式。

---

## 10. 跨阶段支撑工作

| 主题 | 说明 |
|---|---|
| 文档 | 每个 Phase 同步更新 `hnsx-worker/README.md` 和 `docs/know-how/` 中相关文章 |
| 测试 | 每个 Phase 新增测试，保持 `make worker-test` 全绿，coverage 不下降 |
| 示例 | 每个 Phase 至少新增/更新一个 `example-domains/` 示例 |
| 提交 | 每个 Phase 单独 commit，遵循 Conventional Commits |
| Metrics | 记录 latency、p95、eval score、red-team score、cost per trace |

---

## 11. 风险与依赖

| 风险 | 缓解 |
|---|---|
| MCP 协议快速迭代 | 只实现 core spec 1.0，通过 transport 抽象隔离 |
| 向量后端引入运维负担 | 默认 SQLite，向量后端作为可选 extras |
| 多 Agent 调试困难 | 强化 Trace / Observation，提供可视化 sub-agent 树 |
| Prompt evolution 成本高 | 限制变体数量，人工审批后才能上线 |
| 人类审批延迟影响体验 | 支持异步通知 + 可配置超时 fallback |
| SDK 拆分引入 breaking change | 保持 `hnsx_worker` 向后兼容，SDK 只是新接口 |

---

## 12. 本周（W10）立即开始

1. 创建 feature 分支 `feat/worker-w10-mcp`。
2. 实现 `mcp_client` 工具与 stdio transport。
3. 新增 `example-domains/mcp-demo/`。
4. 跑 `make worker-test` 并提交。

---

*文档生成时间：2026-07-10*
*Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>*
