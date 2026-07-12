# HnsX 的愿景

> 这是 HnsX 项目愿景的公开版。完整版（含内部决策、商业策略、未发布方向）
> 在仓库的 [docs/vision.md](../docs/vision.md)，仅供核心团队阅读。

---

## 我们看到了什么

Agent 时代，越来越多的团队把 Agent 集成到自己的业务场景里。无论是低代码
平台、企业内部工具，还是各种 Agent 产品，落地时普遍遇到的真实问题是：

- **效果不达预期**：很多自研 / 通用 Agent 在真实业务里效果不达标。
- **能力差异巨大**：Claude Code、Codex、OpenAI 等 SOTA Coding Agent 的能力远超
  通用 Agent，但它们的设计目标是"通用助手"，不是"业务执行者"。
- **领域知识难沉淀**：每个团队把领域知识写成一次性 prompt，没有沉淀成可复用的资产。

那么问题来了：**为什么不直接用最强的 Agent 来跑业务**？一个 Agent 的能力
很强，但企业真正需要的是把领域知识、Skill、Rule 整合起来"驾驭"这个 Agent
的框架。这就是 Harness。

---

## 我们的答案：Harness as a Service

HnsX 不是要"做一个更好的 Agent 底座"。我们要做的是给企业提供一套
**Harness 约束与编排的能力**：

- **不造 Agent**——Claude Code、Codex、OpenAI、Anthropic、Ollama 等都是
  HnsX 的"被驾驭对象"，HnsX 自己不实现 Agent。
- **做 Harness 层**——把企业长期积累的领域知识、领域 Skill、领域 Rule 整合
  起来，驾驭最强的 Agent。
- **做编排与观测**——细粒度到 Session / Turn / Observation 的执行过程控制，
  而不只是 DAG。
- **做评估与进化**——用评测集驱动 Harness 与 Agent 共同进化。
- **做治理与合规**——Sandbox / Policy / Budget / Guardrails / Human-in-the-loop。

---

## 我们要做什么

### Harness 建设
支持用户改造 System Prompt、构建业务知识库、上传 Skills 和 Rules，
让用户可以定义自己的 Harness。

### Agent 运行时编排
自由编排 Agent 与 Harness——不只是一个 Workflow、一个 YAML/JSON，
而是细粒度到 Session / Turn 的编排。

### 评估
Agent 与 Harness 能力强不强？需要可量化的评估。好的评测集才能让
Harness 持续进化。

### 监控与观测
Agent 是否在进化，本质是 Harness 能力是否进化。我们要细粒度到
Agent / Model 层，把 Agent 每一个 token、每一次 function call 做的事情
都记录清楚。

### Sandbox
Sandbox 决定执行的强度和能做的范围。对企业来说，什么能做什么不能做
用户必须感知、可控、可审计。

### 部署
我们要支持从本地 → 团队 → SaaS 的渐进部署，让用户方便地部署和管理
Harness 与 Agent。

---

## 产品形态

HnsX 提供两种使用形态：

### SDK 形态（面向开发者）
用户可以用 SDK 在自己的产品里集成 Agent。SDK 提供：
- DomainSpec 编程式构造
- Session 触发与流式订阅
- Eval 一键跑

### 平台形态（面向团队）
用户可以在 HnsX 控制台管理自己的 Harness 与 Agent：
- Domain Registry
- Session 触发与监控
- Trace 查询与回放
- Eval 与回归对比
- Approvals / Audit / Metrics

---

## 一句话总结

**Agent 是燃料，Harness 是引擎和方向盘。**

HnsX 不造 Agent，而是把最好的 Agent 装进企业自己的 Harness 里。

---

## 了解更多

- [guide/quick-start.md](guide/quick-start.md) — 5 分钟跑通 HnsX
- [architecture.md](architecture.md) — 技术总览与架构
- [know-how/建模Harness.md](know-how/建模Harness.md) — DomainSpec 建模深度
- [blog/why-harness.md](blog/why-harness.md) — 为什么需要 Harness