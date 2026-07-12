# HnsX — Harness as a Service

<div align="center">

**Don't build weaker agents. Harness stronger ones.**

让企业把 Claude Code / Codex / OpenAI / Anthropic / Ollama 等最强 Agent
装进一个**声明式 Harness** 里 —— 可观测、可审计、可评估、可治理。

[官网](https://narcilee7.github.io/HnsX/) · [文档](https://narcilee7.github.io/HnsX/) · [博客](https://narcilee7.github.io/HnsX/blog) · [快速开始](#快速开始)

<!-- Demo GIF placeholder: 5 秒从 npm i -g hnsx 到 hnsx deploy 成功的全流程
     录制规格：1280×720，≤5s，≤3MB，丢进 docs/assets/demo.gif 后用下面这行引用 -->
<p align="center">
  <img src="docs/assets/demo.gif" alt="HnsX 5 分钟 deploy 演示" width="720">
</p>

```bash
npm i -g hnsx                     # 或 brew install narcilee7/hnsx/hnsx
hnsx try customer-service         # 30 秒跑通一个端到端 Session
```

</div>

---

<div align="center">

[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](hnsx-server/go.mod)
[![Python](https://img.shields.io/badge/Python-%E2%89%A53.11-3776AB?logo=python&logoColor=white)](hnsx-worker/pyproject.toml)
[![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)](hnsx-console/package.json)
[![CLI](https://img.shields.io/badge/CLI-v1.0.0-green)](CHANGELOG.md)
[![Proto](https://img.shields.io/badge/Proto-buf_1.47-00ADD8)](proto/buf.yaml)
[![Discord](https://img.shields.io/badge/Discord-加入-5865F2?logo=discord&logoColor=white)](https://discord.gg/hnsx)
[![GitHub Stars](https://img.shields.io/github/stars/narcilee7/HnsX?style=social)](https://github.com/narcilee7/HnsX)

</div>

---

## 一句话定位

**Agent 是燃料，Harness 是引擎和方向盘。**

HnsX 不自己造 Agent，而是把最好的 Agent 接入企业场景，用 YAML/JSON 声明：

- 这个业务场景需要哪些 Agent、Prompt、Skill、Tool
- 什么能做什么不能做（Policy / Sandbox / Guardrail）
- 谁在什么情况下必须人工审批（Human-in-the-loop）
- 每次调用花了多少 token、成本、延迟
- 如何在真实业务数据上持续评估和进化 Harness

---

## 为什么是 Harness

当企业把 Agent 从 demo 搬到生产，最先撞上的不是模型能力，是这五件事：

| 痛点 | 表现 |
|---|---|
| **不可控** | Agent 能读代码、调工具、联网，但什么能做不能做没有统一策略 |
| **不可审计** | 一次会话里调了哪些工具、改了哪些文件、花了多少 token，事后说不清 |
| **不可复用** | 每个业务线把领域知识写成一次性 prompt，没有沉淀为可复用 Skill/Rule |
| **不可评估** | 不知道 Agent 在真实业务场景下到底好不好，没法持续进化 |
| **不可部署** | 本地跑得开心，一到团队/生产环境就没管控面、沙箱、成本预算 |

HnsX 把这五件事做成产品的**一等公民**，而不是模型框架的边角能力。

---

## 核心能力

| 能力 | 说明 |
|---|---|
| **Domain 管理** | 用声明式 YAML 定义业务领域（Harness），支持注册、版本化、评估 |
| **Session 编排** | 细粒度执行会话：Trigger → Session → Turn → Observation |
| **策略与治理** | Budget（预算）、Permission（权限）、Guardrail（护栏）、Approval（人工审批） |
| **可观测性** | 每个 token、每次工具调用、每次成本消耗，都进入 Trace / Metric / Audit |
| **评估体系** | EvalSet + EvalRun，量化 Harness 与 Agent 在真实场景下的表现 |
| **多 Agent 接入** | 统一 Adapter 接入 Claude Code、Codex、OpenAI、Anthropic、Ollama 等 |
| **MCP 一等公民** | DomainSpec 直接声明 MCP server，无需胶水代码 |
| **Sandbox 多后端** | `none` / `process` / `container` / `microvm` / `delegate` 五级隔离 |
| **部署渐进** | 本地 CLI → Docker Compose → 团队托管 → 企业 SaaS |

---

## 适用场景

- **客服分诊**：把用户问题路由到正确的专家 Agent，自动处理常见问题，敏感操作进人工审批。
- **代码评审**：用 Harness 约束 Review Agent 的检查范围、输出格式、成本上限。
- **金融分析**：让 Agent 读取财报、调用工具、生成报告，同时审计每一步并控制预算。
- **内部运维**：把 SRE 知识沉淀为 Skill 和 Rule，让 Agent 在受限沙箱内执行诊断脚本。
- **跨境电商**：让 Agent 处理多语种 Listing、客服回复、合规检查，统一审计。

---

## 快速开始

> **30 秒跑通端到端**：`hnsx try customer-service`

### 选项 A：一键 install + try（推荐）

```bash
# 0. 装好 hnsx（任选其一）
curl -sSL hnsx.dev/install.sh | sh      # 自动下载并校验 checksum
brew install narcilee7/hnsx/hnsx          # macOS
make build-cli                           # 源码内构建

# 1. 起本地全栈（Postgres + Server + Worker，可选 Tempo + Grafana）
hnsx up                                  # 等 /healthz 通过后返回
hnsx up --with-telemetry                # 同上 + Tempo + Grafana
hnsx up --detach                        # 后台启动

# 2. 跑一个示例 Domain（自动 register + trigger + tail SSE）
hnsx examples                            # 列出 10 个内置示例
hnsx try customer-service               # 30 秒一键：up + register + trigger + tail
hnsx try code-review                    # 自动代码评审
hnsx try research-assistant             # 研报 / 文献综述

# 3. 看效果
hnsx session list                        # 表格列出 Session
hnsx session tail <id>                   # 实时 SSE 流（彩色）
hnsx trace list --limit 10               # 列出最近 Trace

# 4. 打开 GUI
hnsx console                             # 启 Vite + 自动开浏览器
```

本地默认使用 `noop` adapter，无需真实 LLM API Key 即可跑通完整链路。

### 选项 B：用 SDK 嵌入到你的产品

```python
# sdk/python（httpx + DomainSpecBuilder）
from hnsx import HnsXClient
from hnsx.builder import DomainSpecBuilder

spec = (DomainSpecBuilder("customer-service", "v1")
        .with_agent(name="router", provider="anthropic", model="claude-sonnet-4-5")
        .with_tool("http", allow=["api.example.com"])
        .with_policy(max_cost_usd=2.00, require_approval_above_usd=0.50)
        .build())

client = HnsXClient(base_url="http://127.0.0.1:50052")
domain = client.domains.register(spec)
session = client.sessions.trigger(domain_id=domain.id, question="...")
for event in client.sessions.stream_events(session.id):
    print(event)
```

```typescript
// sdk/node（SSE stream + @bufbuild/protobuf）
import { HnsXClient } from "@hnsx/sdk-node";

const client = new HnsXClient({ baseUrl: "http://127.0.0.1:50052" });
const session = await client.sessions.trigger({
  domainId: "customer-service",
  question: "用户问：怎么开通试用？",
});
for await (const event of client.sessions.streamEvents(session.id)) {
  console.log(event);
}
```

```go
// sdk/go（HTTP REST）
client := hnsx.NewClient("http://127.0.0.1:50052", hnsx.WithAPIKey(os.Getenv("HNSX_API_KEY")))
session, _ := client.Sessions.Trigger(ctx, &hnsx.TriggerRequest{
    DomainID: "customer-service",
    Question: "用户问：怎么开通试用？",
})
```

详细 SDK 文档见 [`sdk/`](sdk/) 与 [API 参考](documents/api-reference.md)。

---

## Operator CLI 命令速查

完整的命令词表见 [`documents/guide/cli-basics.md`](documents/guide/cli-basics.md)。常用片段：

| 想做什么 | 命令 |
|---|---|
| 起 / 停 / 看状态 | `hnsx up` / `down` / `status` / `doctor` |
| 列示例 Domain 并一键跑 | `hnsx examples` / `hnsx try <name>` |
| 触发 / 看 Session | `hnsx session trigger --domain <id>` / `hnsx session tail <id>` |
| 查 Trace | `hnsx trace list --since 1h` / `hnsx trace show <id>` |
| 跑 Eval / diff | `hnsx eval run start <set-id>` / `hnsx eval run diff <set> <a> <b>` |
| Policy / Secret / Approval | `hnsx governance policy apply --file ... --confirm` |
| Domain 高级动作 | `hnsx power format/diff/replay/debug-bundle` |
| 打开 GUI / TUI | `hnsx console` / `hnsx tui` |

通用 flag：每条 list 命令都支持 `--limit`、`--filter k=v`、`--since 5m|1h|2d`、`--output human|json|quiet`。

---

## 产品架构一览

```text
┌─────────────────────────────────────────────────────────────────┐
│                        用户与消费层                               │
│   CLI / Web Console / SDK → REST + SSE / gRPC                   │
└─────────────────────────────┬───────────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────────┐
│                     Control Plane（Go）                          │
│  Domain Registry · Session Scheduler · Secret/Policy Store      │
│  Eval Runner · Telemetry Aggregation · Audit Log                │
└─────────────────────────────┬───────────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────────┐
│                   Harness Runtime Worker                         │
│  Loader → Validator → Runner → Adapter → Agent                   │
│  Tool · Skill · MCP · Sandbox · Policy · Memory                  │
└─────────────────────────────────────────────────────────────────┘
```

- **Control Plane**：治理中心，所有 Domain、Session、Policy、Secret、Eval、Telemetry 的归集点。
- **Runtime Worker**：执行 Harness 的无状态工作节点，可被 Scheduler 调度。
- **Adapter**：把统一运行时请求翻译为具体 Agent 调用，负责认证、流式、重试、成本采集。
- **Observation**：Agent 产生的可被审计的中间产物，文本、工具调用、错误、成本、延迟一视同仁。

---

## 关键概念

| 概念 | 说明 |
|---|---|
| **Domain** | 一个业务领域配置包，包含 Harness 定义，是管理、版本化、评估的最小单元 |
| **Harness** | 驾驭体系：Agent、Prompt、Skill、Tool、MCP、Sandbox、Policy、Memory、Eval |
| **Session** | 一次用户触发产生的运行会话，有完整生命周期和状态机 |
| **Turn** | Session 内的一次交互轮次 |
| **Observation** | 可被审计的中间产物，统一进入 Trace / Metric / Audit |
| **Eval** | 评测集与评估运行器，驱动 Harness 持续进化 |

---

## 为什么不是又一个 Agent 平台

- ❌ 不造 Agent 底座
- ❌ 不做低代码 Workflow 编辑器
- ❌ 不做模型训练平台
- ✅ 造 Harness 约束层 + 控制面 + 运维控制台
- ✅ 让企业把最好的 Agent 装进安全、可控、可评估的框架

---

## 路线图

| Phase | 时间 | 目标 |
|---|---|---|
| Phase 1 — PLG 飞轮启动 | 2026-07 → 2026-10 | 1000 注册 / 500 deploy / 500 star |
| Phase 2 — PLG 加速 + B 端叠加 | 2026-10 → 2027-01 | 5000 注册 / 200 付费 |
| Phase 3 — B 端规模化 | 2027-01 → 2027-07 | 5-10 付费企业 / 信创 v1.0 |
| Phase 4 — 长出消费级（可选） | 2027-07 → 2028-07 | 100 付费 / hnsx.new |

---

## 贡献

欢迎贡献！我们接受 bug 修复、文档改进、有范围的 feature，以及 Example
Domain。请先读 [CONTRIBUTING.md](CONTRIBUTING.md) 和
[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)。

| 找什么 | 去哪里 |
|---|---|
| 报告 Bug | [GitHub Issues → bug report](../../issues/new?template=bug_report.yml) |
| 提 Feature | [GitHub Issues → feature request](../../issues/new?template=feature_request.yml) |
| 问使用问题 | [GitHub Discussions](../../discussions) |
| 披露安全漏洞 | [SECURITY.md](SECURITY.md) — **不要公开 Issue** |
| 翻译文档 | 在 Discussions 里认领语言 |
| 找 `good first issue` | [GitHub Issues](../../issues?q=is%3Aopen+is%3Aissue+label%3A%22good+first+issue%22) |

---

## 了解更多

| 文档 | 内容 |
|---|---|
| [官网](https://narcilee7.github.io/HnsX/) | Landing Page、指南、博客、设计文档索引 |
| [`documents/vision.md`](documents/vision.md) | 项目愿景与产品方向 |
| [`documents/architecture.md`](documents/architecture.md) | 技术总览与架构 |
| [`documents/api-reference.md`](documents/api-reference.md) | REST API 完整契约 |
| [`documents/console-design.md`](documents/console-design.md) | Web Console 设计定位与页面 |
| [`documents/know-how/`](documents/know-how/) | 建模、编排、观测、评测四篇 know-how |
| [`documents/blog/`](documents/blog/) | 公开博客（Why Harness / API 一览 / 愿景落地） |
| [`documents/guide/`](documents/guide/) | 用户上手指南（安装 / 快速开始 / CLI 速查 / Domain 入门） |

---

## License

本项目采用 [Apache License 2.0](LICENSE)。

```
Copyright 2026 HnsX Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
```

除非适用法律要求或书面同意，根据本许可分发的软件按"原样"分发，不附任何
明示或暗示的担保或条件。详见 [LICENSE](LICENSE) 中的完整条款。