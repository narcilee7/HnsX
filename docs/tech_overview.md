# HnsX 技术总览（V1）

> 基于 [`vision.md`](./vision.md) 推导。HnsX 不是又一个 Agent，而是让企业安全、可控、可评估地驾驭最强 Agent 的 **Harness-as-a-Service**。

---

## 1. 我们解决什么问题

Claude Code、Codex 等 Coding Agent 的能力已经很强，但企业直接落地时遇到的真实问题是：

| 问题 | 表现 |
|---|---|
| **不可控** | Agent 能读代码、能调工具、能联网，但什么能做什么不能做，没有统一策略。 |
| **不可审计** | 一次会话里调了哪些工具、改了哪些文件、花了多少 token，事后说不清楚。 |
| **不可复用** | 每个业务线都把领域知识写成一次性 prompt，没有沉淀成可复用的 Skill / Rule。 |
| **不可评估** | 不知道 Agent 在真实业务场景下到底好不好，没法持续进化。 |
| **不可部署** | 本地跑得很开心，一到团队/生产环境就没有管控面、没有沙箱、没有成本预算。 |

**HnsX 的解法**：给企业一个声明式的 Harness 层，把领域知识、约束策略、执行沙箱、观测审计、评估体系整合起来，让最强的 Agent 在安全边界内为企业工作。

---

## 2. 核心定位

### 2.1 我们不做什么

- **不造 Agent 底座**：Claude Code、Codex、OpenAI、Anthropic、Ollama、未来的 Agent，都不是 HnsX 自己实现。
- **不做低代码 Workflow 编辑器**：那不是 Harness 的核心。
- **不做模型训练平台**：我们不微调模型。

### 2.2 我们做什么

- **Harness 层**：System Prompt、Knowledge、Skills、Rules、Tools、MCP、Sandbox、Policy、Memory、Eval 的声明式定义与运行时。
- **Agent 运行时编排**：细粒度到 Session / Turn / Observation 的编排，而不只是 DAG。
- **约束与治理**：Sandbox、Policy、Budget、Guardrails、Human-in-the-loop。
- **观测与审计**：每个 token、每次工具调用、每次成本消耗都可追踪。
- **评估体系**：评测集驱动 Harness 与 Agent 共同进化。
- **部署与控制面**：从本地开发到团队托管再到 SaaS 的渐进部署。

**Agent 是燃料，Harness 是引擎和方向盘。**

---

## 3. 总体架构

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                              用户与消费层                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │ hnsx CLI     │  │ React Console│  │ Python SDK   │  │ Node.js SDK  │     │
│  │   (Go)       │  │   (React/TS) │  │              │  │              │     │
│  │  for AI Agent│  │  管理/审批   │  │  Skill/Tool  │  │  前端/集成   │     │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘     │
└─────────┼────────────────┼────────────────┼────────────────┼───────────────┘
          │                │                │                │
          │  gRPC/HTTP     │  HTTP/SSE      │  HTTP/gRPC     │  HTTP/gRPC
          │                │                │                │
┌─────────┼────────────────┴────────────────┴────────────────┼───────────────┐
│         │              Control Plane（Go）                  │               │
│         │  ┌─────────────────────────────────────────────┐ │               │
│         │  │ Domain Registry / Versioning / Marketplace  │ │               │
│         │  │ Session Scheduler / Trigger Router          │ │               │
│         │  │ Eval / Benchmark Registry & Runner          │ │               │
│         │  │ Telemetry Aggregation / Audit Log / Cost    │ │               │
│         │  │ Secret / Policy / Sandbox Profile Store     │ │               │
│         │  └─────────────────────────────────────────────┘ │               │
│         └─────────────────────┬─────────────────────────────┘               │
│                               │                                             │
│         ┌─────────────────────┴─────────────────────────────┐               │
│         │              Harness Runtime（Go Worker）            │               │
│         │  ┌─────────────┐ ┌─────────────┐ ┌──────────────┐  │               │
│         │  │ DomainSpec  │ │  Harness    │ │   Session    │  │               │
│         │  │   Loader    │ │  Runner     │ │  / Turn /    │  │               │
│         │  │  Validator  │ │             │ │ Observation  │  │               │
│         │  └─────────────┘ └──────┬──────┘ └──────┬───────┘  │               │
│         │                         │               │           │               │
│         │  ┌──────────────────────┼───────────────┼────────┐  │               │
│         │  │      Agent Integration Layer                    │  │               │
│         │  │  Adapter（Claude Code / Codex / OpenAI / ...）  │  │               │
│         │  │  MCP Client（优先接入外部 Agent 能力）           │  │               │
│         │  └──────────────────────┴───────────────┴────────┘  │               │
│         │                         │                           │               │
│         │  ┌──────────────────────┼────────────────────────┐  │               │
│         │  │        Capability Layer                         │  │               │
│         │  │  Tools  │  Skills  │  MCP Servers  │  Prompts   │  │               │
│         │  └──────────────────────┴────────────────────────┘  │               │
│         │                         │                           │               │
│         │  ┌──────────────────────┼────────────────────────┐  │               │
│         │  │        Constraint Layer                         │  │               │
│         │  │  Sandbox  │  Policy  │  Guardrails  │  Budget  │  │               │
│         │  └──────────────────────┴────────────────────────┘  │               │
│         │                         │                           │               │
│         │  ┌──────────────────────┴────────────────────────┐  │               │
│         │  │        State & Observability Layer              │  │               │
│         │  │  Memory（Context） │  Telemetry（Trace/Metric） │  │               │
│         │  └─────────────────────────────────────────────────┘  │               │
│         └─────────────────────────────────────────────────────┘               │
└─────────────────────────────────────────────────────────────────────────────┘
```

关键分层：

- **Control Plane**：治理中心，所有 Domain、Session、Policy、Secret、Eval、Telemetry 的归集点。
- **Runtime Worker**：实际执行 Harness 的无状态工作节点，可以被 Scheduler 调度。
- **Agent Integration Layer**：Adapter 和 MCP Client，负责把外部 Agent 接入 Harness，但不属于核心价值层。

---

## 4. 核心抽象

| 概念 | 说明 |
|---|---|
| **Domain** | 一个业务领域配置包，包含 Harness 定义，是 HnsX 管理、版本化、评估的最小单元。 |
| **Harness** | 驾驭体系，由 Agent、Prompt、Skill、Tool、MCP、Sandbox、Policy、Memory、Session、Eval 组成。 |
| **Agent** | 被 Harness 接入的外部 Agent（Claude Code、Codex、OpenAI、Anthropic 等），HnsX 不实现 Agent。 |
| **Adapter** | 把统一运行时请求翻译为具体 Agent 调用形式的适配器；负责认证、流式、重试、成本采集。 |
| **Session** | 一次用户触发产生的运行会话，有生命周期和状态机。 |
| **Turn** | Session 内的一次交互轮次（user → agent → observation → response）。 |
| **Observation** | Agent 产生的可被审计的中间产物：文本、工具调用、错误、成本、延迟。 |
| **Skill** | 可复用的业务能力包，包含 Prompt、Tool、MCP、示例、评估用例。 |
| **Tool** | 内置或用户扩展的原子能力：http、shell、sql、python、file 等。 |
| **MCP** | Model Context Protocol Client，把外部 MCP Server 接入为 Skill/Tool 来源。 |
| **Sandbox** | 执行隔离策略，决定 Agent/Tool 能在什么环境里运行。 |
| **Policy** | 预算、权限、人工介入、输出校验等约束规则。 |
| **Memory** | 跨 Session 的上下文存储，支持短期工作记忆和长期知识。 |
| **Eval** | 评测集与评估运行器，量化 Harness 与 Agent 在业务场景下的表现。 |
| **Telemetry** | Trace、Metric、Cost、Audit 的采集与输出。 |

---

## 5. 技术栈：优先使用开源方案，少造轮子

### 5.1 CLI：Go 实现，为 AI Agent 设计

CLI 是独立的用户入口，**专门设计为可被 AI Agent 调用和理解**。

- **实现**：Go，单二进制，无外部依赖即可运行。
- **命令风格**：稳定、幂等、结构化输出优先，类 `gh`、`awscli`、`kubectl`。
- **AI Agent 友好原则**：
  - 所有输出默认提供 `--json` / `--yaml` 结构化模式，方便 Agent 解析。
  - 错误信息机器可读，包含稳定的错误码。
  - 命令名自解释，避免歧义缩写。
  - 支持 `--dry-run` 让 Agent 在真实执行前预览影响。
  - 支持 `--output-format` 统一输出形状。
  - 环境变量与配置文件一致，Agent 容易注入。
- **核心命令**（v1.0；完整词表见 [`docs/cli-roadmap.md`](cli-roadmap.md) §2）：
  - **Lifecycle**：`hnsx up|down|restart|status|doctor|logs|reset` —— 一键拉起 / 停 / 诊断本地栈
  - **Discovery**：`hnsx examples`、`hnsx try <name>` —— 一键跑示例 Domain
  - **Resource**：`hnsx domain|session|trace|eval {list,show,trigger,...}` —— 资源命令（资源导向命名）
  - **Governance**：`hnsx governance {policy,secret,approval,audit,auth} ...`
  - **Surface**：`hnsx console`（启 GUI）、`hnsx tui`、`hnsx update`
  - **Power**：`hnsx power {format,diff,replay,debug-bundle}`
  - **Local**：`hnsx validate`、`hnsx run`
- **配置三层**：`--flag` > `HNSX_*` env > `~/.config/hnsx/*.yaml`
- **Output 三态**：human（默认表格）/ json / quiet，CI 友好

CLI 可以是独立产物，也可以作为 Go SDK 的封装。

### 5.2 服务：Go 实现，拆分为 Control Plane + Runtime

服务是后端 Daemon，不和 CLI 耦合：

- **Control Plane**：Domain Registry、Scheduler、Secret/Policy Store、Telemetry Aggregation、Eval Runner、REST/gRPC API。
- **Runtime Worker**：执行 Harness 的无状态 Worker，由 Scheduler 触发，向 Control Plane 上报 Telemetry。
- **通信**：Control Plane 与 Runtime Worker 之间用 gRPC + Protobuf；外部用 HTTP/REST + JSON；实时 Observation 用 SSE。

服务可以：

- 内嵌在 CLI 中做本地开发（`hnsx server` 启动轻量版 Control Plane）。
- 单独部署为团队级服务。
- 水平扩展为 Worker Pool。

### 5.3 控制台：React + TypeScript，用成熟开源组件

- **框架**：React 19 + TypeScript + Vite
- **UI 组件库**：Shadcn/ui（基于 Radix UI + Tailwind CSS）
- **表格/复杂列表**：TanStack Table
- **表单**：React Hook Form + Zod
- **状态/缓存**：TanStack Query
- **路由**：React Router
- **编辑器**：
  - YAML/JSON/代码编辑：**Monaco Editor**（VS Code 同款）
  - 不自己实现编辑器内核
- **Trace/可观测性展示**：
  - 优先复用 OpenTelemetry 生态
  - Trace 可视化可用现有组件或嵌入 Grafana Tempo / Jaeger UI
  - 不自己实现复杂的 Trace 时序图
- **图表**：Recharts 或 Tremor

控制台不是低代码编辑器，而是：

- Domain / Harness 注册与版本管理
- Session 实时查看（SSE 接收流式 Observation）
- Trace / Cost / Audit 查询
- Eval 结果与对比
- Human-in-the-loop 审批面板

### 5.4 协议：Protobuf 作为唯一真相源

`proto/` 目录保存 HnsX 的 API 协议定义：

- `hnsx/v1/domain.proto`：DomainSpec、Harness 配置、版本管理
- `hnsx/v1/control_plane.proto`：Registry、Scheduler、Discovery、Telemetry、Eval
- `hnsx/v1/runtime.proto`：Trigger、Session 流式响应
- `hnsx/v1/observation.proto`：Observation、Trace、Metric 数据结构

生成目标：

- Go：运行时、CLI、控制面
- TypeScript：React Console、Node.js SDK
- Python：Python SDK

传输层：

- **CLI ↔ Control Plane**：gRPC（内部、高效）
- **Console / SDK ↔ Control Plane**：HTTP/REST + JSON（外部、通用）
- **实时 Observation**：SSE（Server-Sent Events）或 gRPC stream

### 5.5 可观测性：基于 OpenTelemetry

不自己造 Trace/Metric 协议和存储：

- **协议**：OpenTelemetry（OTLP）
- **存储**：
  - 本地：PostgreSQL 或 jsonl
  - 团队：PostgreSQL + Grafana Tempo / Jaeger
  - SaaS：OTLP Collector → 任意后端
- **展示**：
  - Grafana 做大盘和 Trace 查询
  - Console 里嵌入 Grafana 面板或调用 Tempo/Jaeger API
- **指标**：Prometheus exporter 或 OTLP metrics

### 5.6 Sandbox：优先复用现有隔离技术

不自己实现操作系统级隔离：

| 后端 | 技术 | 场景 |
|---|---|---|
| `none` | 当前进程 | 本地信任环境 |
| `process` | Linux namespace + seccomp / landlock | 轻量隔离 |
| `container` | Docker / Podman / containerd | 标准容器隔离 |
| `microvm` | Firecracker / gVisor | 强隔离、多租户 |
| `delegate` | 用户提供的远程 sandbox | 企业已有隔离设施 |

Runtime 通过统一接口调用不同后端，后端实现可以逐步增加。

### 5.7 SDK

SDK 是 Harness 的客户端和扩展入口：

- **Go SDK**：与 CLI/Runtime 共享代码，适合 CI 和运维集成。
- **Python SDK**：AI/ML 生态集成，Jupyter、自定义 Tool、Skill、Eval 开发。
- **Node.js SDK**：TypeScript 优先的前后端集成，适合与 Claude Code / Codex 扩展联动。

初期重点：

1. Go SDK（与运行时同步）
2. Python SDK（用户最可能写 Skill / Tool / Eval）
3. Node.js SDK（生态补齐）

### 5.8 基础设施：按阶段演进

不一步建成 SaaS，而是随阶段演进：

| 阶段 | 存储 | 缓存 | 消息 | 可观测性 | 部署 |
|---|---|---|---|---|---|
| **本地 / 单体** | PostgreSQL（轻量 Docker 版） | 内存 | 无 | jsonl / stdout 或 Tempo | 单二进制、Docker Compose |
| **小团队** | PostgreSQL | Redis（可选） | 无 / 内存队列 | OTLP → Tempo/Jaeger + Grafana | Docker Compose |
| **企业 / SaaS** | PostgreSQL | Redis | Kafka / NATS（可选） | OTLP Collector + 对象存储 | Kubernetes / Helm |

**当前阶段只承诺：PostgreSQL + 单二进制 + Docker Compose + OpenTelemetry。**

---

## 6. 部署形态

HnsX 从第一版就按服务设计，本地只是开发模式。

### 6.1 本地开发

```text
┌─────────┐     ┌────────────────────────────────┐
│ hnsx CLI │────▶│  Embedded Runtime + PostgreSQL │
└─────────┘     └────────────────────────────────┘
```

- CLI 内置轻量 Control Plane 和 Runtime，直接加载 Domain YAML。
- PostgreSQL 存 Trace / Metric / Audit。
- 用于开发、调试、CI。

### 6.2 团队托管

```text
┌─────────┐     ┌─────────────────┐     ┌───────────────┐
│ Console │────▶│  Control Plane  │────▶│ Runtime Worker│
│  / SDK  │     │  + PostgreSQL   │     │  (one or many)│
│  / CLI  │     │  + Tempo/Grafana│     │               │
└─────────┘     └─────────────────┘     └───────────────┘
```

- Control Plane 提供 Domain Registry、Scheduler、Telemetry Aggregation。
- 多个 Runtime Worker 可被调度执行不同 Domain。
- Trace 通过 OTLP 接入 Tempo，Grafana 做展示。
- 适合团队内部署。

### 6.3 企业 / SaaS

```text
┌─────────────┐     ┌──────────────────────┐     ┌─────────────────┐
│ Multi-tenant │────▶│   Control Plane      │────▶│ Runtime Pool    │
│   Console    │     │  + PostgreSQL + Redis│     │  (per-tenant    │
│   / SDK      │     │  + Secret / Policy   │     │   or shared)    │
│   / CLI      │     │  + OTLP Collector    │     │                 │
└─────────────┘     └──────────────────────┘     └─────────────────┘
```

- 多租户隔离、配额、计费、审计。
- 这是远期目标，当前架构预留接口但不提前实现。

---

## 7. 关键数据流

### 7.1 Trigger → Session → Observation

```text
用户 / SDK / Console / CLI
   │
   ▼
Control Plane（Registry 找到 Domain，Scheduler 选择 Runtime 实例）
   │
   ▼
Harness Runtime Worker
   │
   ├─► Loader 加载 DomainSpec
   ├─► Validator 校验 Harness
   ├─► HarnessRunner 创建 Session
   ├─► 按 Session.Mode 执行 Turn 序列
   │       │
   │       ▼
   │   Agent Integration Layer（Adapter / MCP）
   │       │
   │       ▼
   │   外部 Agent 调用（流式）
   │       │
   │       ▼
   │   输出经过 Sandbox / Policy 校验
   │       │
   │       ▼
   │   产生 Observation
   │
   ├─► Telemetry Sink 写入 Trace / Metric / Audit（OTLP / PostgreSQL）
   │
   └─► Console / CLI 实时展示 Observation
```

### 7.2 Agent Integration Layer 职责

这是 Harness 与外部 Agent 之间的边界：

- 把统一的 `TurnRequest` 翻译成 Agent 特定的调用格式。
- 处理流式返回，把 chunk 转成统一的 `ObservationChunk`。
- 处理认证、超时、重试、错误映射。
- 采集 token 用量、成本、延迟，写入 Telemetry。
- 对 Claude Code / Codex 这类本地 CLI Agent，Adapter 负责进程生命周期管理。

**重要**：Adapter 只是接入手段，HnsX 的核心价值在 Harness 层（约束、编排、评估、观测）。

### 7.3 Eval 数据流

```text
Domain + EvalSet
   │
   ▼
Eval Runner（Control Plane 或 CLI）
   │
   ├─► 对每个 Case 触发 Harness Runtime
   ├─► 收集 Observation 与输出
   ├─► 用评分器（规则 / LLM-as-judge / 人工）打分
   │
   ▼
Eval Report（对比版本、追踪回归）
```

Eval 是 Harness 进化的核心闭环。

---

## 8. 非功能性设计

### 8.1 可观测性

- 所有 Agent 调用必须产生 `Observation`。
- 每个 Session 必须可追踪到 Domain、Agent、Model、Cost、Latency。
- Telemetry 优先走 OpenTelemetry OTLP，本地 fallback 到 PostgreSQL / jsonl。
- Console 不自研复杂 Trace UI，嵌入 Grafana / Tempo / Jaeger。

### 8.2 安全与隔离

- Sandbox 是多后端设计：`none` / `process` / `container` / `microvm` / `delegate`。
- 后端优先复用 Docker、Firecracker、gVisor、Linux namespace 等现有技术。
- Policy 在编译期和运行时两层生效：预算超限、敏感操作、人工审批。
- Secret 不落在 DomainSpec 明文里，由控制面统一注入。

### 8.3 可扩展性

- Tool、Skill、MCP 都通过接口注册，不改动核心即可扩展。
- Adapter 接口统一，新增 Agent 只需新增 Adapter。
- Memory、Telemetry、Sandbox 后端可替换。

### 8.4 少造轮子的具体承诺

| 领域 | 不自研 | 优先复用 |
|---|---|---|
| 前端 UI 组件 | 基础组件库 | Shadcn/ui、Radix UI、Tailwind |
| 表格/复杂列表 | 高性能表格 | TanStack Table |
| 代码/YAML/JSON 编辑器 | 编辑器内核 | Monaco Editor |
| Trace 可视化 | 时序图 / Trace UI | Grafana Tempo / Jaeger |
| 可观测性协议 | 私有协议 | OpenTelemetry |
| 图表 | 图表库 | Recharts / Tremor |
| Sandbox 隔离 | OS 级隔离 | Docker、Firecracker、gVisor |
| 数据库 ORM | 复杂 ORM | 按需要选 sqlc / ent / raw SQL |
| 工作流引擎 | 复杂 DAG 引擎 | 先实现简单 Session/Turn 状态机 |

---

## 9. 当前阶段聚焦（V1 Phase 1-3）

1. **CLI for AI Agent**：Go 实现，`validate` / `run` / `eval` / `traces` 等命令，默认 `--json` 输出。
2. **Go Runtime Worker**：`DomainSpec` v2 模型、Loader/Validator、HarnessRunner、Session/Turn/Observation。
3. **Noop Adapter**：无网络环境下跑通完整 Harness 链路。
4. **Control Plane 骨架**：Domain Registry、Scheduler、Telemetry Aggregation，内嵌在 CLI 中可本地启动。
5. **Proto 协议**：完成 Domain / Runtime / Telemetry / Eval 协议定义并生成 Go/TS/Python 代码。
6. **React Console 骨架**：Domain 列表、Monaco 编辑器、Session 触发、Trace 查询。
7. **PostgreSQL Telemetry**：本地 Trace / Metric / Audit 存储，输出兼容 OTLP。
8. **Eval 骨架**：EvalSet 定义 + 基础评分器。

不在这个阶段做的事：

- Kafka / ELK / Helm
- 多租户 SaaS
- 复杂 Workflow DAG 可视化编辑器
- 自研模型或训练平台
- 自己实现 Agent 底座
- 自研复杂组件（编辑器、Trace UI、表格）

---

## 10. 口号

> **Don't build weaker agents. Harness stronger ones.**
