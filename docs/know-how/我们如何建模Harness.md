# 我们如何建模 Harness

> 本文定义 HnsX 的核心实体、实体间关系、生命周期和配置分层。是 `DomainSpec`、运行时和 Eval 的共同基础。

---

## 1. 设计目标

Harness 模型要满足：

1. **声明式**：用户用 YAML/JSON/TOML 描述，而不是写代码。
2. **可版本化**：Domain、Skill、Rule 都能独立版本和演进。
3. **可复用**：Skill、Tool、Prompt 能在多个 Domain 间共享。
4. **可评估**：Eval 能精确引用 Harness 的任意部分。
5. **不侵入 Agent**：Harness 只在边界生效，不替 Agent 思考。
6. **可编译**：DomainSpec 加载后应能静态校验引用完整性和策略一致性。

---

## 2. 核心实体

### 2.1 Domain

**Domain 是 HnsX 管理的最小业务单元**，包含一个 Harness 定义、可选的 Eval 配置、以及元数据。

```yaml
id: customer-service
version: 0.1.0
description: Routes customer questions.
harness:
  ...
eval:
  ...
```

- Domain 有全局唯一的 `id`。
- Domain 通过 `version` 演进，Registry 保留历史版本。
- Domain 是部署、调度、评估的最小单位。

### 2.2 Harness

**Harness 是套在 Agent 外面的约束与能力体系**，由以下要素组成：

| 要素 | 职责 | 是否必须 |
|---|---|---|
| **Agents** | 声明被接入的外部 Agent | 是 |
| **Prompts** | System prompt、role prompt、task prompt 模板 | 否 |
| **Skills** | 可复用的业务能力包 | 否 |
| **Tools** | 原子能力（http、shell、sql 等） | 否 |
| **MCPs** | 外部 MCP Server 配置 | 否 |
| **Sandbox** | 执行隔离策略 | 否（默认 none） |
| **Policy** | 预算、权限、guardrails、人工审批 | 否（默认放行） |
| **Store** | 跨 session 上下文存储策略 | 否（默认 in_memory） |
| **Session** | 编排策略和运行模式 | 是 |
| **Eval** | 评测集 | 否 |

Harness 本身不是运行时对象，而是**配置蓝图**。运行时根据 Harness 构建一个 `HarnessInstance`。

### 2.3 Agent

**Agent 是被 Harness 接入的外部 Agent**，HnsX 不实现 Agent。

```yaml
agents:
  - id: triage
    description: Classifies user intent.
    model:
      provider: anthropic
      model: claude-sonnet-4
    adapter:
      kind: anthropic
      timeout_seconds: 60
    prompt:
      id: triage-system
    skill_refs: [intent-extraction]
    tool_refs: [search]
```

Agent 配置的是**接入方式**，不是 Agent 内部能力。具体包括：

- `model`：provider + model name，用于成本估算和 trace 标注。
- `adapter`：如何调用（openai、anthropic、claude-code、codex、noop 等）。
- `prompt`：注入 system prompt。
- `skill_refs`：挂载的 Skill，扩展 Agent 可用能力。
- `tool_refs`：直接挂载的 Tool。

**关键**：Agent 可以不带 Skill/Tool，只作为对话伙伴存在。

### 2.4 Prompt

**Prompt 是可复用的文本模板**。

```yaml
prompts:
  - id: triage-system
    template: |
      You are a triage agent. Classify user intent into one of:
      {categories}
    variables:
      categories: "billing, technical, other"
```

- Prompt 通过 `id` 被 Agent 或 Skill 引用。
- 变量在加载时或运行时注入。
- 同一个 Prompt 可以被多个 Agent 复用。

### 2.5 Skill

**Skill 是可复用的业务能力包**，是 HnsX 沉淀领域知识的核心单元。

```yaml
skills:
  intent-extraction:
    description: Extracts user intent from messages.
    prompts:
      - id: intent-prompt
        template: "Extract intent from: {message}"
    tools:
      - name: search
        kind: http
        config: ...
    mcp_refs: [crm-mcp]
    examples:
      - input: "I was charged twice"
        output: '{"intent": "billing"}'
```

Skill 包含：

- Prompts
- Tools（使用 `kind` 指定工具类型）
- MCP refs
- 示例 / few-shot
- 评估用例（可选）

**Skill 可以独立发布和版本化**，一个 Skill Registry 未来可以让团队共享 Skill。

### 2.6 Tool

**Tool 是原子能力**，是 Agent 与外部世界交互的通道。

```yaml
tools:
  search:
    description: Search internal knowledge base.
    kind: http
    config:
      method: GET
      url: https://kb.example.com/search
      headers:
        Authorization: "Bearer {secret.kb_token}"
```

内置 Tool 类型：

| 类型 | 说明 |
|---|---|
| `http` | HTTP 请求 |
| `shell` | 执行 shell 命令（受 Sandbox 约束） |
| `sql` | 执行 SQL 查询 |
| `python` | 执行 Python 代码 |
| `file` | 文件读写 |
| `custom` | 用户扩展 |

Tool 的 `config` 可以引用 secret：`{secret.SECRET_NAME}`，由控制面注入。

### 2.7 MCP

**MCP 是外部 Model Context Protocol Server 的配置**。

```yaml
harness:
  mcp:
    servers:
      - name: crm-mcp
        transport: stdio
        command: npx
        args: ["-y", "@example/crm-mcp"]
        headers:
          CRM_API_KEY: "{secret.crm_api_key}"
```

MCP 让 Agent 获得动态、外部化的能力，而不需要在 Domain 里定义所有 Tool。

### 2.8 Sandbox

**Sandbox 定义执行隔离策略**。

```yaml
sandbox:
  backend: container
  config:
    image: hnsx/runtime:latest
    network: none
    volumes: []
```

后端：`none`、`process`、`container`、`microvm`、`delegate`。

### 2.9 Policy

**Policy 定义约束规则**。

```yaml
policy:
  budget:
    max_cost_usd: 10.0
    max_turns: 20
  permissions:
    allow_network: true
    allow_file_write: false
    allow_shell: false
  guardrails:
    - id: prevent_data_exfiltration
      type: tool_deny
      on: tool_call
      action: block
      config:
        tools: [http]
      message: HTTP tool is not allowed unless destination is allowlisted
  approval:
    default_timeout_seconds: 600
    required_for:
      tools: [issue_refund, export_customer_data]
      resources: ["billing:write", "customer:*"]
      cost_threshold_usd: 0.25
```

Policy 在以下时机生效：

1. **编译期**：检查 Tool/Agent 是否在 allowed/denied 列表。
2. **运行时 Tool call 前**：否决不允许的操作。
3. **运行时预算检查**：超预算时暂停或失败。
4. **运行时敏感操作前**：触发 human-in-the-loop。

### 2.10 Store（原 Memory）

**Store 定义跨 session 上下文存储策略**，按用途分为 context / knowledge / ephemeral 三个命名空间。

```yaml
store:
  context:
    backend: in_memory
  knowledge:
    backend: postgres
    config:
      connection_string: "{secret.pg_conn}"
      retention: 30d
  ephemeral:
    backend: in_memory
```

类型：

- `in_memory`：当前 session 内有效。
- `postgres` / `redis`：跨 session 长期记忆。

Store 内容对 Agent 可见的方式由 Adapter 决定。

### 2.11 Session

**Session 定义编排策略**，是 Harness 与运行时的桥梁。

```yaml
session:
  mode: supervisor
  supervisor:
    agent: triage
    transitions: ...
```

详见《我们如何编排 Agent 并集成 Harness.md》。

### 2.12 Eval

**Eval 定义评测集**，用于量化 Harness 在真实业务场景下的表现。

```yaml
eval:
  sets:
    - id: routing-accuracy
      cases:
        - id: billing-question
          input: { question: "Why was I charged twice?" }
          expect:
            observations:
              - kind: routing_decision
                output: { intent: billing }
          scorer: structured_match
```

Eval 可以引用 Harness 的 Agents、Steps、Observations 进行断言。

---

## 3. 实体关系图

```text
Domain
│
├─ Harness
│   │
│   ├─ Agents ─────┬── Prompts
│   │              ├── Skills ──── Prompts / Tools / MCPs / Examples
│   │              ├── Tools ────── Config
│   │              └── MCPs ─────── Config
│   │
│   ├─ Sandbox
│   ├─ Policy
│   ├─ Memory
│   └─ Session ───── Orchestration
│
└─ Eval
    └─ EvalSets ──── EvalCases ──── Expect / Scorer
```

引用规则：

- Agent 引用 Prompt、Skill、Tool。
- Skill 引用 Prompt、Tool、MCP。
- Session 引用 Agent。
- Eval 引用 Agent、Step、Observation。
- 所有引用通过 `id` 解析，加载时校验。

---

## 4. 配置分层

HnsX 有三层配置：

### 4.1 DomainSpec（用户写的配置）

用户输入，YAML/JSON/TOML。关注可读性和表达力。

### 4.2 Harness Model（内存模型）

加载 DomainSpec 后的结构化对象，包含所有引用解析后的结果。

### 4.3 Harness Instance（运行时实例）

一次 Session 运行时构建的实例，包含：

- 解析后的 Domain
- 注入的 secret
- 激活的 Sandbox backend
- 激活的 Memory backend
- Telemetry sink
- 当前 Session 状态

```text
DomainSpec --(Load)--> Harness Model --(Instantiate)--> Harness Instance
```

---

## 5. 生命周期

### 5.1 Domain 生命周期

```text
author --> write DomainSpec --> validate --> register --> version --> schedule/run --> telemetry --> eval --> evolve
```

### 5.2 Session 生命周期

```text
trigger --> load domain --> build Harness Instance --> create Session --> execute turns --> observations --> transitions --> completed/failed/paused
```

### 5.3 Skill 生命周期

```text
author --> write Skill --> test --> publish --> version --> referenced by Domain --> evaluated in Domain context
```

---

## 6. 引用解析与校验

加载 Domain 时，必须完成以下校验：

1. **ID 唯一性**：Domain、Agent、Prompt、Skill、Tool、MCP 的 ID 不能重复。
2. **引用存在性**：Agent 引用的 Prompt/Skill/Tool 必须存在；Session 引用的 Agent 必须存在。
3. **循环依赖**：Skill 之间不能循环引用。
4. **Policy 一致性**：allowed/denied 列表不能冲突；secret 占位符必须可解析。
5. **Session 有效性**：orchestration 模式必须合法，entry step 必须存在。

未解析的引用必须报错，不能静默忽略。

---

## 7. 与编排的关系

Harness 模型提供**素材**，编排策略决定**如何使用这些素材**。

```text
Harness: 有什么 Agent、Skill、Tool、约束
Session: 怎么把它们组织成一次运行
```

例如：

- 同一个 Harness（triage + billing + technical）可以用 `supervisor` 跑，也可以用 `workflow` 跑。
- `single` 模式只用一个 Agent，忽略其他 Agent。
- `autonomous` 模式让 Agent 自己决定调用哪些 Tool/Skill。

所以 **Harness 是声明性的，编排策略是 Harness 的一部分但独立演化**。

---

## 8. 与 Eval 的关系

Eval 是 Domain 的一部分，但可以独立引用 Harness 元素。

```yaml
eval:
  sets:
    - id: routing-accuracy
      cases:
        - id: billing-question
          input: { question: "Why was I charged twice?" }
          expect:
            observations:
              - kind: routing_decision
                agent_id: triage
                output: { intent: billing }
```

Eval 可以验证：

- Agent 是否被正确调用
- Observation 序列是否符合预期
- Transition 是否正确触发
- 最终输出是否满足业务要求
- 成本和延迟是否在预算内

---

## 9. 设计原则 checklist

在设计 Harness 模型时检查：

- [ ] 每个实体是否有清晰的职责边界？
- [ ] 引用关系是否无歧义、可静态校验？
- [ ] 是否方便 Eval 引用任意部分？
- [ ] Skill 是否能独立复用？
- [ ] Secret 是否不落在明文配置中？
- [ ] Policy 是否在编译期和运行时都能生效？
- [ ] 编排策略是否与 Harness 素材解耦？
- [ ] 是否保留了 Agent 的自主性？

---

## 10. 演进方向

1. **Skill Registry**：Skill 独立发布，Domain 通过坐标引用。
2. **Policy as Code**：支持更复杂的 guardrail 规则。
3. **Memory Schema**：定义记忆类型（episodic、semantic、procedural）。
4. **Multi-Domain**：一个 Domain 可以 import 另一个 Domain 的 Skill/Prompt。
5. **Schema 约束**：DomainSpec 增加 JSON Schema 校验。
