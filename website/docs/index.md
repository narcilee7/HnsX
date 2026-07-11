---
pageType: home

hero:
  name: HnsX
  text: Harness as a Service
  tagline: 不要造更弱的 Agent。给企业一套声明式 Harness，安全驾驭 Claude Code、Codex、OpenAI 等最强 Agent。
  actions:
    - theme: brand
      text: Why harness
      link: /blog/why-harness
    - theme: alt
      text: 阅读文档
      link: /design/vision
    - theme: alt
      text: GitHub
      link: https://github.com/narcilee7/HnsX

features:
  - title: Domain 定义
    details: 用 YAML/JSON 声明业务场景下的 Agent、Prompt、Skill、Tool、Policy。
    link: /design/vision
  - title: 细粒度编排
    details: Trigger → Session → Turn → Observation，全程可控。
    link: /design/know-how/我们如何编排Agent并集成Harness
  - title: 策略与治理
    details: Budget、Permission、Guardrail、Approval，守住企业边界。
    link: /design/tech_overview
  - title: 可观测性
    details: 每个 token、每次工具调用、每次成本消耗都进入 Trace / Metric / Audit。
    link: /design/know-how/我们如何观测Harness与Agent
  - title: 评估体系
    details: EvalSet + EvalRun，量化 Harness 与 Agent 在真实场景下的表现。
    link: /design/know-how/我们如何评测Harness与Agent
  - title: 多 Agent 接入
    details: 统一 Adapter 接入 Claude Code、Codex、OpenAI、Anthropic、Ollama。
    link: /design/tech_overview
---

## 快速开始

```bash
# 1. 安装 CLI
curl -sSL hnsx.dev/install.sh | sh

# 2. 初始化一个 Domain
hnsx init --template customer-service --output-dir my-domain

# 3. 运行一次 Session
hnsx run --domain my-domain/domain.yaml
```

更多安装方式见 [API 一览](/blog/api-at-a-glance) 与 [服务端 API 设计](/design/server-design/api-design)。
