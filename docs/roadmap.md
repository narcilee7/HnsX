# HnsX RoadMap

> Public-facing high-level roadmap. For the detailed implementation plan,
> gap analysis, and exit criteria, see
> [`design/ProjectManagement/V1/RoadMap.md`](../design/ProjectManagement/V1/RoadMap.md).

---

## V1 Goal

让 HnsX 成为**企业级自主 Agent 的 Harness 运行时**：

- 通过声明式配置（YAML/TOML/JSON）接入 Claude Code、Codex、OpenAI、Anthropic、Ollama 等强 Agent。
- 提供约束体系：sandbox、policy、guardrails、budget。
- 提供能力层：tools、skills、MCP、prompts、memory。
- 提供观测与审计：session / turn / observation / cost / trace。
- 支持本地、容器、控制面、SaaS 多种部署形态。

V1 不是：低代码 AI 应用构建器、模型训练平台、通用 RPA、纯开发者编程框架。

更多见 [`design/ProjectManagement/V1/Positioning.md`](../design/ProjectManagement/V1/Positioning.md)。

---

## V1 Phases

### Phase 0 — 产品与架构定型（已完成）

| 子任务 | 交付物 | 状态 |
|---|---|---|
| 产品定位 | [`Positioning.md`](../design/ProjectManagement/V1/Positioning.md) | ✅ |
| 架构蓝图 | [`Architecture.md`](../design/Tech/V1/Architecture.md) | ✅ |
| 领域模型与 Spec | [`DomainSpec-v2.md`](../design/Tech/V1/DomainSpec-v2.md) | ✅ |
| 技术栈决策 | Go + Python + Vue | ✅ |

### Phase 1 — Go Runtime 骨架（进行中）

- Go module 与目录结构
- `DomainSpec` v2 模型
- YAML/TOML/JSON loader
- `hnsx validate` 命令

### Phase 2 — Harness 运行时核心

- `HarnessRunner` + `Session/Turn/Observation`
- `noop` adapter
- `single-task` / `workflow` session 模式

### Phase 3 — 能力层：Tools / Skills / MCP

- 内置 Tools：`http`、`shell`、`sql`、`python`
- `Skill` 引擎
- MCP client
- Prompt / SP 模块

### Phase 4 — Agent Adapters

- `openai` / `anthropic` / `ollama` / `custom`
- `claude-code` CLI adapter
- `codex` CLI adapter

### Phase 5 — Sandbox & Policy

- Sandbox backends：`none` / `process` / `container` / `delegate`
- Budget / Permission / Human-in-the-loop guardrails
- Output schema validation

### Phase 6 — Memory & Telemetry

- Memory backends：`in_memory` / `sqlite`
- Telemetry sinks：`jsonl` / stdout
- Trace 查看 CLI

### Phase 7 — Control Plane

- gRPC/REST API
- Domain Registry / Session Scheduler
- Telemetry Aggregation / Secret Management

### Phase 8 — Web Console（Vue）

- Domain / Session / Trace / Metrics 页面
- 与控制面 API 对接

### Phase 9 — Python SDK

- Python runtime bridge
- `hnsx` Python package

### Phase 10 — Build / Deploy / Release

- `hnsx build` / `hnsx deploy --target docker`
- Release 流程与安装脚本
- 完整文档站点

---

## V1 Exit Criteria

1. `hnsx validate` 能解析 DomainSpec v2 和旧 v1 workflow 格式。
2. `hnsx run --adapter noop` 在无网络环境下跑通完整 harness。
3. `hnsx run` 在至少一个真实 provider 下成功执行。
4. `HarnessRunner` 支持 `single-task` 和 `workflow` 两种 session 模式。
5. 内置 tools 至少实现 `http` 和 `shell`。
6. 控制面能注册 domain、接收 trigger、聚合 telemetry。
7. Web Console 能查看 domain、session、trace。
8. Go / Python / Console 测试全绿。
9. README、RoadMap、Architecture、DomainSpec 文档与代码一致。

---

## 口号

> **Don't build weaker agents. Harness stronger ones.**
