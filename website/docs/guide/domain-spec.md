# Domain 入门

Domain 是 HnsX 的核心概念：它描述了一个**业务场景下如何驾驭 Agent**。

## 最小 DomainSpec

```yaml
apiVersion: hnsx.io/v1
kind: Domain
metadata:
  id: customer-service
  version: "1.0.0"
spec:
  harness:
    agents:
      - id: assistant
        provider: openai
        model: gpt-4o-mini
        system_prompt: default
    prompts:
      - id: default
        template: |
          你是一个客服助手，帮助用户处理退换货、订单查询等问题。
    session_mode:
      type: single
      agent: assistant
    policies:
      - type: budget
        max_cost_usd: 1.0
```

## 核心字段

| 字段 | 说明 |
|---|---|
| `agents` | 可用的 Agent 列表，指定 provider、model、system_prompt |
| `prompts` | 可复用的 Prompt 模板 |
| `skills` | 业务 Skill，可被 Agent 调用 |
| `tools` | 外部工具（函数 / MCP / 脚本） |
| `policies` | Budget、Permission、Guardrail、Approval |
| `store` | 跨 Turn 记忆存储 |
| `evals` | 评测集定义 |

## 验证 Domain

```bash
hnsx validate --domain example-domains/customer-service/domain.yaml
```

## 运行 Domain

```bash
hnsx run --domain example-domains/customer-service/domain.yaml \
  --trigger '{"question":"我想退货"}'
```

完整字段定义见 [我们如何建模 Harness](/design/know-how/我们如何建模Harness)。
