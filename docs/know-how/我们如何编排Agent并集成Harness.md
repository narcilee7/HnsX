# 我们如何编排 Agent 并集成 Harness

> HnsX 的编排哲学：**Agent-Centric，不是 Workflow-Centric。**

---

## 1. 核心原则

### 1.1 不要切碎 Agent

最强的 Coding Agent（Claude Code、Codex 等）之所以强，是因为它们能在一个连续上下文中进行：

- 多步 reasoning
- self-reflection
- tool use 和纠错
- 长程规划

如果我们把 Agent 的输出切成一个个字段，下一步只拿到这个字段，就打断了这个 reasoning chain。**Workflow 应该定义上下文边界，而不是每次调用。**

### 1.2 Harness 只在边界介入

Harness 层的职责是：

- **选择 Agent**：当前 step 用哪个 Agent/Skill/Tool。
- **注入上下文**：把上一步的 observation、memory、policy 注入当前 Agent 的 prompt。
- **应用约束**：Sandbox、Policy、Budget、Guardrails。
- **记录审计**：每个 turn、每个 tool call、每个 token、每次成本。
- **决定转移**：根据 Agent 输出和规则，决定下一步去哪里。

Harness 不负责替 Agent 思考。

### 1.3 编排策略是可评估的配置

同一个 Domain，可以配置不同的 orchestration 策略：

- `single`：一个 Agent 跑到底。
- `supervisor`：supervisor Agent 动态 dispatch。
- `workflow`：预定义 step 图，但每个 step 内部 Agent 自主。
- `autonomous`：Agent 自己决定下一步。

这些策略都是 Harness 配置的一部分，可以被版本化、A/B 测试、用 Eval 评估。

---

## 2. 核心概念

| 概念 | 定义 |
|---|---|
| **Session** | 一次用户触发产生的完整运行实例，是 Agent 运行的容器。 |
| **Turn** | Agent 与环境的一次交互轮次（user/agent/tool/observation）。一个 Step 内可以有多个 Turn。 |
| **Step** | 编排层定义的**上下文边界**。指定用哪个 Agent/Skill/Tool、在什么约束下、什么条件下退出。 |
| **Observation** | Agent 产生的可被审计的事件：文本、tool call、tool result、错误、成本、transition decision。 |
| **Transition** | Step 之间的控制转移。可以由 Agent 输出、规则、人工、默认策略驱动。 |
| **Orchestration** | Harness 的编排策略：single / supervisor / workflow / autonomous。 |

关键公式：

```text
Step ≠ 一次 Agent call
Step = 一个 Agent session 的上下文定义
```

---

## 3. 编排模式

### 3.1 single：一个 Agent 跑到底

最简单的模式。用户问题交给一个 Agent，Harness 只负责约束和观测。

适用场景：

- 任务边界清晰
- 不需要多 Agent 协作
- 想要最大化 Agent 自主性

```yaml
harness:
  agents:
    - id: assistant
      model: { provider: anthropic, model: claude-sonnet-4 }
      adapter: { kind: anthropic }
  session:
    mode: single
    agent: assistant
    max_turns: 50
    exit: "$.observations[-1].role == 'assistant' && $.observations[-1].content != null"
```

### 3.2 supervisor：supervisor Agent 动态 dispatch

推荐优先模式。一个 supervisor Agent 负责理解用户意图，然后决定把控制权交给哪个 specialist Agent。

适用场景：

- 多个 specialist Agent
- 需要根据上下文路由
- 想保留 Agent 自主性，又需要结构化分工

```yaml
harness:
  agents:
    - id: triage
      model: { provider: anthropic, model: claude-sonnet-4 }
      adapter: { kind: anthropic }
    - id: billing
      model: { provider: openai, model: gpt-4o }
      adapter: { kind: openai }
    - id: technical
      model: { provider: openai, model: gpt-4o }
      adapter: { kind: openai }

  session:
    mode: supervisor
    supervisor:
      agent: triage
      # supervisor 每次给出 routing decision 后，Harness 根据 decision 做 transition
      transitions:
        - when: "$[?kind=='routing_decision' && output.intent == 'billing'].latest"
          to: billing
        - when: "$[?kind=='routing_decision' && output.intent == 'technical'].latest"
          to: technical
        - when: "$[?kind=='routing_decision' && output.requires_human == true].latest"
          to: human_approval
      fallback: triage
      max_turns: 20

    # specialist 完成后的默认行为
    specialists:
      billing:
        exit: "$[?agent_id=='billing' && kind=='text'].latest.content != null"
        transitions:
          - default: end
      technical:
        exit: "$[?agent_id=='technical' && kind=='text'].latest.content != null"
        transitions:
          - default: end
```

**重要**：supervisor 是一个真正的 Agent，它可以多 turn 思考、调用 tool、反问用户，最后才给出 routing decision。Harness 只在收到 routing decision 这个 observation 时才做 transition。

### 3.3 workflow：预定义 step 图，step 内 Agent 自主

适合需要显式控制流程的场景。每个 step 指定 Agent 和退出条件，Agent 在 step 内部自主运行。

```yaml
harness:
  session:
    mode: workflow
    workflow:
      entry: gather_context
      steps:
        - id: gather_context
          agent: triage
          # Agent 自主运行，直到产生非空 intent
          exit: "$[?kind=='text' && agent_id=='triage'].latest.output.intent != null"
          transitions:
            - when: "$[?step_id=='gather_context'].latest.output.intent == 'billing'"
              to: billing
            - when: "$[?step_id=='gather_context'].latest.output.intent == 'technical'"
              to: technical
            - default: gather_context  # 没拿到 intent，继续

        - id: billing
          agent: billing
          exit: "$[?step_id=='billing'].latest.kind == 'text'"
          transitions:
            - default: end

        - id: technical
          agent: technical
          exit: "$[?step_id=='technical'].latest.kind == 'text'"
          transitions:
            - default: end
```

### 3.4 autonomous：Agent 自己决定下一步

Agent 拥有最大自由度，自己决定调用 tool、skill 或其他 agent。Harness 只负责：

- 审计每个 decision
- 应用 Sandbox/Policy veto
- 在预算/权限边界处暂停

```yaml
harness:
  agents:
    - id: primary
      model: { provider: anthropic, model: claude-sonnet-4 }
      adapter: { kind: anthropic }
  session:
    mode: autonomous
    agent: primary
    allowed_tools: [search, shell, file_read]
    allowed_agents: [billing, technical]
    max_turns: 50
    exit: "$[?kind=='text' && content contains 'DONE'].exists"
```

---

## 4. Transition 表达式

Transition 表达式基于 Observation 序列。我们建议用 JMESPath 语法：

| 示例 | 含义 |
|---|---|
| `$.observations[-1].output.intent == 'billing'` | 最新 observation 的 intent 是 billing |
| `$[?agent_id=='triage' && kind=='routing_decision'].latest.output.intent` | triage 的最新 routing decision |
| `$[?kind=='text' && agent_id=='billing'].latest.content != null` | billing 已经产生文本输出 |
| `$.policy.requires_human_approval == true` | policy 触发人工审批 |
| `$.budget.remaining_usd < 0.01` | 预算即将耗尽 |

表达式求值结果必须是 boolean。

---

## 5. 与 Harness 集成

### 5.1 Agent 选择

每个 Step 的 `agent` 字段指向 Harness 中定义的 Agent。Agent 配置包括：

- model（provider + model name）
- adapter（如何调用）
- prompt（system prompt 模板）
- skill_refs（可复用的 Skill）
- tool_refs（可用 Tools）

### 5.2 上下文注入

Harness Runner 会自动把以下上下文注入 Agent：

- 用户原始 trigger
- 当前 step 之前的 observations
- memory 中的相关信息
- policy 约束
- available tools / skills / agents

注入方式由 Adapter 决定。对于 Claude Code / Codex，可能是通过 stdin 或 MCP；对于 OpenAI / Anthropic API，是构造 messages。

### 5.3 Sandbox 与 Policy

每个 Step 执行前，Runner 会：

1. 检查当前 Agent/Tool 是否在 allowed list。
2. 检查预算是否足够。
3. 根据 Sandbox backend 准备执行环境。
4. 如果 policy 要求 human approval，暂停并等待。

这些约束不侵入 Agent 的 reasoning，只在边界生效。

### 5.4 Observation 与 Telemetry

每个 Step 内部发生的所有事件都会产生 Observation：

- Agent 输出的文本
- Tool call 和 tool result
- Routing decision
- Human approval/rejection
- Cost 和 latency
- Transition 触发

Telemetry 通过 OpenTelemetry OTLP 输出，可以被 Tempo / Grafana 消费。

---

## 6. 与 Eval 集成

Eval 不再只测最终输出，而是测整个 Session 的执行过程。

### 6.1 Eval 可以评估编排策略

同一个 Domain，可以用不同 orchestration 跑同一个 eval set：

```bash
hnsx eval --domain customer-service --orchestration supervisor
hnsx eval --domain customer-service --orchestration workflow
```

### 6.2 Eval 可以评估 observation 序列

```yaml
eval:
  sets:
    - id: routing-accuracy
      description: 检查 supervisor 是否正确路由用户问题
      cases:
        - id: billing-question
          name: Billing routing
          input:
            question: "Why was I charged twice?"
          expect:
            observations:
              - kind: routing_decision
                agent_id: triage
                output:
                  intent: billing
              - kind: text
                agent_id: billing
          scorer: structured_match

        - id: technical-question
          name: Technical routing
          input:
            question: "How do I reset my API key?"
          expect:
            observations:
              - kind: routing_decision
                output:
                  intent: technical
              - kind: text
                agent_id: technical
          scorer: structured_match
```

### 6.3 Eval 评分器

- `exact`：完全匹配
- `contains`：包含指定字段/文本
- `structured_match`：基于 observation 序列的结构化匹配
- `llm-judge`：用 LLM 评估质量
- `script`：自定义脚本

---

## 7. 设计 checklist

在设计一个 Domain 的编排时，问自己：

- [ ] 这个任务真的需要多个 Agent 吗？如果不需要，用 `single`。
- [ ] 多个 Agent 之间是自然分工吗？如果是，用 `supervisor`。
- [ ] 流程需要严格保证顺序和条件吗？如果是，用 `workflow`。
- [ ] 是否希望 Agent 自己探索？如果是，用 `autonomous`。
- [ ] 每个 Step 的退出条件是否明确？
- [ ] Transition 是否会被 Eval 覆盖到？
- [ ] Policy / Sandbox 是否会在关键边界生效？

---

## 8. 反模式

### ❌ 把 Agent 切成函数

```yaml
# 坏的例子
steps:
  - id: classify
    agent: triage
    output: classification   # 只拿到一个字符串
  - id: respond
    agent: billing
    input:
      classification: "${classification}"   # 丢失了大量上下文
```

这样 triage 只能输出一个分类字符串，不能反问用户、不能调用 tool、不能解释原因。

### ✅ Agent-Centric 的做法

```yaml
steps:
  - id: triage
    agent: triage
    exit: "$[?kind=='routing_decision'].exists"
    transitions:
      - when: "$[?kind=='routing_decision'].latest.output.intent == 'billing'"
        to: billing
```

triage 是一个完整的 Agent session，它可以思考、反问、调用 tool，最后产生 routing_decision。

### ❌ 在编排层替 Agent 做决策

不要在 Transition 里写复杂的业务逻辑。Transition 应该基于 Agent 的输出，而不是绕过 Agent。

### ✅ Harness 只做边界判断

```yaml
transitions:
  - when: "$[?kind=='routing_decision'].latest.output.intent == 'billing'"
    to: billing
```

意图判断由 Agent 做，Harness 只负责路由。

---

## 9. 实现路线图

1. **Phase 1**：实现 `single` 和 `supervisor` 两种模式。
2. **Phase 2**：实现 `workflow` 模式，支持 step 内多 turn。
3. **Phase 3**：实现 `autonomous` 模式，Agent 自己决定下一步。
4. **Phase 4**：支持 hierarchical / sub-session，一个 Domain 可以调用另一个 Domain。
5. **Phase 5**：Eval 框架支持 observation 序列评估。

---

## 10. 总结

HnsX 的编排不是把 Agent 变成 DAG 节点，而是：

> **在 Agent 自主运行的前提下，定义 Harness 何时介入、何时转移、如何约束、如何审计。**

这样既能发挥最强 Agent 的能力，又能让企业安全、可控、可评估地使用它们。

**Agent 是主角，Harness 是导演和监制。**
