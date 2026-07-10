# HnsX — Harness as a Service

> **Don't build weaker agents. Harness stronger ones.**

HnsX 让企业安全、可控、可评估地驾驭 Claude Code、Codex、OpenAI、Anthropic、Ollama 等最强 Agent。它不是又一个 Agent 底座，而是一层**声明式 Harness**：把领域知识、约束策略、执行沙箱、观测审计、评估体系整合在一起，让 Agent 在明确边界内为企业工作。

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

## 核心能力

| 能力 | 说明 |
|---|---|
| **Domain 管理** | 用声明式 YAML 定义业务领域（Harness），支持注册、版本化、评估 |
| **Session 编排** | 细粒度执行会话：Trigger → Session → Turn → Observation |
| **策略与治理** | Budget（预算）、Permission（权限）、Guardrail（护栏）、Approval（人工审批） |
| **可观测性** | 每个 token、每次工具调用、每次成本消耗，都进入 Trace / Metric / Audit |
| **评估体系** | EvalSet + EvalRun，量化 Harness 与 Agent 在真实场景下的表现 |
| **多 Agent 接入** | 统一 Adapter 接入 Claude Code、Codex、OpenAI、Anthropic、Ollama 等 |
| **部署渐进** | 本地 CLI → Docker Compose → 团队托管 → 企业 SaaS |

---

## 适用场景

- **客服分诊**：把用户问题路由到正确的专家 Agent，自动处理常见问题，敏感操作进人工审批。
- **代码评审**：用 Harness 约束 Review Agent 的检查范围、输出格式、成本上限。
- **金融分析**：让 Agent 读取财报、调用工具、生成报告，同时审计每一步并控制预算。
- **内部运维**：把 SRE 知识沉淀为 Skill 和 Rule，让 Agent 在受限沙箱内执行诊断脚本。

---

## 快速开始

```bash
# 1. 启动完整本地环境（Postgres + Server + Worker + Tempo + Grafana）
docker compose -f deployments/local/docker-compose.yaml up -d

# 2. 触发一个 customer-service 会话
SID=$(curl -fsS -X POST http://127.0.0.1:50052/api/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"domain_id":"customer-service","trigger":{"question":"hello"}}' | jq -r .id)

# 3. 实时观看 Observation 流
curl -N http://127.0.0.1:50052/api/v1/sessions/$SID/events

# 4. 打开 Grafana 看 Trace 大盘
open http://127.0.0.1:3002
```

本地默认使用 `noop` adapter，无需真实 LLM API Key 即可跑通完整链路。

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

## 了解更多

| 文档 | 内容 |
|---|---|
| [`docs/vision.md`](docs/vision.md) | 项目愿景与产品方向 |
| [`docs/tech_overview.md`](docs/tech_overview.md) | 技术总览、架构与阶段规划 |
| [`docs/server-design/api-design.md`](docs/server-design/api-design.md) | REST API 完整契约 |
| [`docs/web-console-design/整体设计.md`](docs/web-console-design/整体设计.md) | Web Console 设计定位与页面 |
| [`docs/know-how/`](docs/know-how/) | 建模、编排、观测、评测四篇 know-how |

---

## License

See source headers. Phase 1 work-in-progress.
