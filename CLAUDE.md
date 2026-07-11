# CLAUDE.md

> 给 AI 编程助手（Claude Code 等）的 HnsX 工作指南 —— 先理解产品，再写代码。

---

## 1. 产品定位

**HnsX = Harness as a Service**

不要把它理解成"又一个 Agent 平台"。HnsX **不造 Agent 底座**，而是给企业一个**声明式 Harness 层**：

- 接入 Claude Code / Codex / OpenAI / Anthropic / Ollama 等最强 Agent
- 用 DomainSpec 定义业务场景下的 Agent、Prompt、Skill、Tool、Policy、Sandbox
- 让每次 Session 都可观测、可审计、可评估、可治理

**核心价值**：让企业把最好的 Agent 装进安全、可控、可评估的框架里工作。

完整愿景见 [`docs/vision.md`](docs/vision.md)，技术总览见 [`docs/tech_overview.md`](docs/tech_overview.md)，控制台设计见 [`docs/web-console-design/整体设计.md`](docs/web-console-design/整体设计.md)。

---

## 2. 仓库与模块职责

```text
HnsX/
├── docs/                  # 设计文档（vision、tech、api、know-how）
├── proto/                 # Protobuf — API 单一真相源
├── hnsx-server/           # Go 控制面：Domain / Session / Policy / Secret / Eval / Telemetry
├── hnsx-worker/           # Python 运行时：执行 Harness 的 Worker
├── hnsx-console/          # React 控制台：运维与审计中心
├── observability/         # 前端可观测组件库
├── sdk/                   # Go / Node / Python SDK
├── example-domains/       # 示例 DomainSpec YAML
├── deployments/local/     # Docker Compose 本地全栈环境
└── scripts/               # 构建、测试、smoke 脚本
```

关键分层：

- **Control Plane（Go）**：治理中心，所有 Domain、Session、Policy、Secret、Eval、Telemetry 的归集点。
- **Runtime Worker（Python）**：无状态执行节点，被 Scheduler 调度，上报 Observation。
- **Console（React）**：不是低代码编辑器，而是 Harness 的**运维与审计控制中心**。
- **SDK**：Harness 的客户端与扩展入口。

---

## 3. 核心抽象（写代码前先理解）

| 概念 | 含义 | 代码位置 |
|---|---|---|
| **Domain** | 业务领域配置包，包含 Harness 定义 | `proto/hnsx/v1/domain.proto`、`hnsx-server/pkg/spec` |
| **Harness** | 驾驭体系：Agent、Prompt、Skill、Tool、MCP、Sandbox、Policy、Memory、Eval | DomainSpec 的 `harness` 字段 |
| **Session** | 一次触发产生的运行会话 | `hnsx-server/internal/session`、`hnsx-worker/session_executor.py` |
| **Turn** | Session 内一次交互轮次 | Runtime 内部概念 |
| **Observation** | Agent 产生的可被审计的中间产物 | `hnsx-server/pkg/runtime/observation.go`、protobuf `observation.proto` |
| **Trace** | 一次 Session 的完整 Observation 聚合 | `hnsx-server/internal/trace` |
| **Eval** | 评测集与评估运行 | `hnsx-server/internal/evaluation` |
| **Policy** | Budget / Permission / Guardrail / Approval | `hnsx-server/pkg/policy`、`hnsx-server/internal/approval` |

---

## 4. 工作约定

### 4.1 协议优先

- `proto/hnsx/v1/*.proto` 是 API 唯一真相源。
- 改协议后先跑 `make proto` / `make proto-all`，再改 Go / Python / TS 代码。
- 不要让 REST handler、Console mapper、Worker 各自对字段做不同解释。

### 4.2 后端改动要同步的 checklist

改 `hnsx-server` 时：

1. `make build-server` 能过
2. `go test ./...` 能过
3. `./scripts/smoke.sh` 能过
4. `./scripts/smoke-cli.sh` 能过（CLI 表面）
5. 如果改 API，检查 `hnsx-console/src/api/` 和 `hnsx-console/src/api/mappers.ts`
6. 如果改 CLI 共享的 client 接口，同步更新 `hnsx-server/cmd/hnsx/cli/` 下的资源命令

### 4.2.1 CLI 命令约定（v1.0）

完整命令词表见 [`docs/cli-roadmap.md`](docs/cli-roadmap.md) §2。改动 CLI 时遵守：

- **资源导向命名**：`hnsx <verb> <resource>`，类似 `kubectl` / `gh`
- **Output 三态**：每条 list/show 默认 human 表格，加 `--output json|quiet` 切换
- **退出码语义化**：`0` 成功 / `1` 用户错 / `2` 资源错 / `3` 服务器错 / `4` 权限错 / `5` 网络错
- **配置三层**：`--flag` > `HNSX_*` env > `~/.config/hnsx/*.yaml`
- **破坏性命令必须 `--confirm`**：policy apply / delete、secret set / delete、eval set delete
- **共享 `internal/client/`**：CLI 不重写协议，加 endpoint 时同步在 `cmd/hnsx/cli/<resource>.go` 加新子命令
- **测试**：单元测试放 `<x>_test.go`（同包），端到端 smoke 加进 `scripts/smoke-cli.sh`
- **deprecated 命令**：只追加不删，旧 `hnsx remote <x>` 标记 hidden + 移除计划写进 v1.1

常见新增命令落位：

| 类型 | 文件 | 函数 |
|---|---|---|
| 资源 list/show | `cli/<resource>.go` | `newResource{List,Show,...}Cmd` |
| 资源 trigger/exec | `cli/<resource>.go` | 同上 |
| 治理（policy/secret/audit） | `cli/governance.go` | `newGovernanceCmd` 下挂 |
| 表面（console/tui/update） | `cli/console.go` / `update.go` | 独立 command |
| 高级（format/diff/replay） | `cli/power.go` | `newPowerCmd` 下挂 |

每次新增 CLI 子命令至少满足：
1. `--help` 写清楚用法 + 至少 1 个例子
2. `human | json | quiet` 三种 output 都合理
3. 至少 1 条单元测试（参数解析、边界、helper）
4. smoke-cli.sh 至少 1 行覆盖

改 `hnsx-worker` 时：

1. 在 `hnsx-worker/.venv` 环境下跑 `python -m pytest`
2. 关键路径跑 docker compose 验证一次真实 Session

### 4.3 前端改动要同步的 checklist

改 `observability/` 时：

1. `pnpm type-check`
2. 到 `hnsx-console` 跑 `pnpm install --force` 再 `pnpm type-check && pnpm build`

改 `hnsx-console/` 时：

1. `pnpm install --force`
2. `pnpm type-check`
3. `pnpm build`

### 4.4 颜色与主题

- 所有颜色走 CSS 变量：`var(--chart-1..5)`、`var(--success)`、`var(--warning)`、`var(--danger)`、`var(--info)`
- 不要硬编码 hex 或 Tailwind 默认色
- 主题文件：`observability/src/tokens/morandi.css`、`hnsx-console/src/index.css`

### 4.5 不自研原则

| 需要 | 用现成的 | 不要 |
|---|---|---|
| sparkline | `react-sparklines` | 自己画 SVG |
| calendar heatmap | `react-calendar-heatmap` | 自己写 grid |
| 完整图表 | recharts / Tremor | 手画 SVG |
| schema 校验 | Zod | 手写 if/else |
| 表单 | React Hook Form | 自己管 onChange |
| icon | lucide-react | 自己画 SVG |
| Toast | sonner | 自己实现 |
| 代码编辑器 | Monaco | 自研编辑器 |
| Trace UI | Grafana Tempo / Jaeger | 自研复杂 Trace 时序图 |

只在**壳层 / 配色 / 主题 / 业务逻辑**上自研。

---

## 5. 常见任务速查

### 加一个新页面到 Console

1. `hnsx-console/src/pages/FooPage.tsx`
2. 路由加进 `hnsx-console/src/App.tsx`
3. 侧边栏加进 `hnsx-console/src/components/layout/Sidebar.tsx`
4. 数据走 `src/hooks/useFoo.ts` + `src/api/mappers.ts`

### 改 API 协议

1. 改 `proto/hnsx/v1/*.proto`
2. `make proto` 或 `make proto-all`
3. 改 `hnsx-server/pkg/api/` handler
4. 改 `hnsx-console/src/api/mappers.ts`
5. 跑 `go test ./...` + `pnpm type-check`

### 改 Domain 模型

1. 先读 `docs/know-how/我们如何建模Harness.md`
2. 同步改 `proto/`、`hnsx-server/pkg/spec/`、`hnsx_worker/` 的加载器
3. 更新 `example-domains/` 中的示例

### 改 Observation / Trace

1. 先读 `docs/know-how/我们如何观测Harness与Agent.md`
2. 同步改 `hnsx-server/pkg/runtime/observation.go`、protobuf、DB repo、TracerSink
3. Worker 侧的 Observation 序列化也要同步

### 本地验证端到端

```bash
cd deployments/local
docker compose up -d
# 触发 Session
curl -fsS -X POST http://127.0.0.1:50052/api/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"domain_id":"customer-service","trigger":{"question":"hello"}}'
# 看 Grafana
open http://127.0.0.1:3002
```

---

## 6. 不要做的事

- ❌ 把 HnsX 做成 Agent 底座或低代码 Workflow 编辑器
- ❌ 在 Console 里"造 Agent"——那是 Claude Code / Codex 的活
- ❌ 重新发明 sparkline / heatmap / sankey / 代码编辑器 / Trace UI
- ❌ 硬编码颜色——全部走 CSS var
- ❌ 跳过 `go test ./...` 或 `pnpm type-check`
- ❌ 改完 `observability` 不跑 `pnpm install --force`
- ❌ 协议字段做两套解释——Protobuf 是唯一真相源
- ❌ 直接 push 到 main，走 feat/* → PR → review 流程

---

## 7. 知识库（先读再做）

| 文件 | 何时读 |
|---|---|
| `docs/vision.md` | 拿不准产品方向时 |
| `docs/tech_overview.md` | 想了解架构与阶段规划时 |
| `docs/server-design/api-design.md` | 改 API 或加 endpoint 前 |
| `docs/web-console-design/整体设计.md` | 加新页面 / 改交互前 |
| `docs/know-how/我们如何建模Harness.md` | 改 Domain 模型时 |
| `docs/know-how/我们如何编排Agent并集成Harness.md` | 改 Runtime / Workflow 时 |
| `docs/know-how/我们如何观测Harness与Agent.md` | 改 Observation / Trace 时 |
| `docs/know-how/我们如何评测Harness与Agent.md` | 改 Eval 系统时 |
| `observability/README.md` | 加 / 改 observability 组件前 |

---

## 8. 提交规范

- Commit message 用 Conventional Commits：`feat(server): ...`、`fix(console): ...`、`docs(readme): ...`
- 多文件改动但同一主题时尽量一个 commit
- PR 末尾可标注 `🤖 Generated with [Claude Code](https://claude.com/claude-code)`

---

*记住：你不是在造一个更好的 Agent，你是在造让企业安全驾驭最强 Agent 的 Harness。*
