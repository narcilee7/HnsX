# Why harness

> Agent 是燃料，Harness 是引擎和方向盘。

## 我们看到了什么

Agent 时代，越来越多的人选择自己实现 Agent，再集成到业务场景。Coze、Dify、LangGraph 等工具降低了入门门槛，但真到了生产环境，这些自研 Agent 往往面临三个问题：

1. **效果不稳定**：通用底座很难吃下特定业务的领域知识。
2. **边界不可控**：Agent 能调什么工具、花多少成本、出错谁负责，都是黑箱。
3. **难以进化**：没有评测、没有审计、没有反馈闭环，prompt 改来改去靠运气。

与此同时，Claude Code、Codex、Cursor 等 Coding Agent 的能力越来越强。它们不是通用 Agent 底座，而是**特定场景下的超级执行者**。为什么不让这些最强的 Agent 去跑业务任务，同时在外面套一层约束和编排？

这就是 Harness 的出发点。

## Harness 是什么

Harness 不是又一个 Agent 底座，而是**让企业把最强 Agent 装进安全、可控、可评估框架里的一层**。它回答四个问题：

- **做什么**：用 DomainSpec 描述业务场景、Agent、Prompt、Skill、Tool。
- **怎么做**：细粒度编排 Trigger → Session → Turn → Observation。
- **什么不能做**：Policy（预算、权限、护栏、审批）。
- **做得怎么样**：Eval（评测集）+ Trace（审计）+ Metric（观测）。

## HnsX 的答案

HnsX = Harness as a Service。

我们提供：

- **声明式 Domain**：YAML/JSON 定义业务 Harness。
- **多 Agent 接入**：统一 Adapter 接入 Claude Code、Codex、OpenAI、Anthropic、Ollama。
- **治理策略**：Budget、Permission、Guardrail、Approval、Sandbox。
- **可观测性**：每个 token、每次工具调用都进入 Trace / Metric / Audit。
- **评估体系**：EvalSet + EvalRun，让 Harness 和 Agent 一起进化。

下一篇：[API 一览](/blog/api-at-a-glance)。
