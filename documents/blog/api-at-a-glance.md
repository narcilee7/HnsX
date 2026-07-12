# API 一览

HnsX 采用 **Protobuf 作为 API 单一真相源**，对外同时暴露 **gRPC (Connect-RPC)** 与 **REST** 两套协议，方便不同场景接入。

## 核心资源

| 资源 | 说明 | 典型操作 |
|---|---|---|
| Domain | Harness 定义 | Register / Get / List / Validate |
| Session | 一次运行会话 | Trigger / Get / List / Stop |
| Trace | 会话完整轨迹 | Get / List / Search |
| Approval | 人工审批 | List / Approve / Reject |
| Policy | 策略与治理 | Get / Apply |
| Secret | 敏感配置 | Set / Get / Delete |
| Eval | 评测 | CreateSet / Run / GetResult |

## REST 端点

所有 REST 端点前缀为 `/api/v1/`。例如：

```bash
curl -fsS http://127.0.0.1:50052/api/v1/domains
curl -fsS http://127.0.0.1:50052/api/v1/sessions
```

完整设计见：[服务端 API 设计](/design/server-design/api-design)。

## 认证

当前支持：

- `Authorization: Bearer <token>` JWT / API Key
- `X-Tenant-ID` 多租户隔离

## SDK

- [Node/TypeScript SDK](/design/tech_overview)
- Python SDK（开发中）
- Go SDK（开发中）
