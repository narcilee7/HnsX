# HnsX

> **Harness for Autonomous Agents**

HnsX 让企业直接消费 Claude Code、Codex 以及未来的 Claw、Hermes 等最强 Agent 的能力，而不是从头构建更弱的 Agent。

用户通过声明式配置（YAML/TOML/JSON）定义自己的 **Harness（驾驭体系）**：接入 Agent、配置能力层（Tools / Skills / MCP / Prompts）、设定沙箱与策略、接入记忆与观测。HnsX 运行时负责在约束下安全、可控、可审计地执行 Agent 会话。

---

## 一句话定位

**Don't build weaker agents. Harness stronger ones.**

---

## HnsX 不是什么

| 产品类型 | HnsX 的态度 |
|---|---|
| **MLflow / LangSmith / W&B** | 互补。它们是模型/调用观测平台，HnsX 是 Agent 执行 harness。 |
| **Coze / Dify / FastGPT** | 差异化。它们让用户构建弱 Agent；HnsX 让用户驾驭强 Agent。 |
| **n8n / Zapier** | 互补。它们是通用工作流自动化；HnsX 是 Agent 运行时基础设施。 |
| **LangGraph / CrewAI** | 互补。它们是开发者框架；HnsX 是声明式运行时。 |

更多见 [`design/ProjectManagement/V1/Positioning.md`](design/ProjectManagement/V1/Positioning.md)。

---

## Workspace layout

```
go/                    # Go monorepo: CLI / runtime / control plane
  cmd/hnsx/            # hnsx CLI
  pkg/core/            # domain, harness, session, turn, observation
  pkg/adapter/         # Claude Code / Codex / OpenAI / Anthropic / Ollama / Custom
  pkg/tool/            # HTTP / Shell / SQL / Python tools
  pkg/skill/           # Skill engine
  pkg/mcp/             # MCP client
  pkg/sandbox/         # Sandbox backends
  pkg/memory/          # Memory backends
  pkg/telemetry/       # Trace / metrics / audit
  pkg/controlplane/    # Registry / scheduler / API

python/                # Python runtime bridge + SDK
  hnsx/                # Python SDK
  bridges/             # Claude Code / Codex Python bridges
  tools/               # Python tool executor

console/               # Web Console (Vue + Vite)
  src/
  public/
  package.json
  vite.config.ts

domains/               # example domain YAMLs
legacy/                # archived Rust implementation (reference only)
docs/                  # public docs
design/                # architecture, spec, roadmap
```

---

## Tech stack

| 模块 | 技术 | 理由 |
|---|---|---|
| CLI / Runtime / Control Plane | Go | 部署简单、并发强、单二进制 |
| Agent Bridges / SDK | Python | AI/ML 生态、与 Agent CLI 集成 |
| Web Console | Vue + TypeScript | 现代前端、适合控制台类 UI |
| Protocol | gRPC + REST + YAML/TOML/JSON | 内部高效、外部通用、配置友好 |

---

## Build

### Go

```bash
cd go
go mod tidy
go build -o ../bin/hnsx ./cmd/hnsx
../bin/hnsx --help
```

### Web Console

```bash
cd console
pnpm install
pnpm dev
```

### Python SDK

```bash
cd python
pip install -e .
```

---

## Quick start

Validate an example domain:

```bash
./bin/hnsx validate --domain domains/customer-service/domain.yaml
```

Run a domain once locally with the noop adapter (no network required):

```bash
./bin/hnsx run \
  --domain domains/customer-service/domain.yaml \
  --adapter noop \
  --trigger '{"question":"What is the status of my order?"}'
```

Run with a real provider (set `OPENAI_API_KEY` or `ANTHROPIC_API_KEY`):

```bash
./bin/hnsx run \
  --domain domains/customer-service/domain.yaml \
  --trigger '{"question":"What is the status of my order?"}'
```

Start the control plane (gRPC on 50051, HTTP + Web UI on 50052):

```bash
./bin/hnsx control-plane --addr 127.0.0.1:50051 --static-dir console/dist
```

Register a domain with the control plane:

```bash
./bin/hnsx register \
  --domain domains/customer-service/domain.yaml \
  --control-plane http://127.0.0.1:50051
```

Run a domain and report telemetry to the control plane:

```bash
./bin/hnsx run \
  --domain domains/customer-service/domain.yaml \
  --adapter noop \
  --trigger '{"question":"hello"}' \
  --control-plane http://127.0.0.1:50051
```

Inspect telemetry:

```bash
./bin/hnsx traces --control-plane http://127.0.0.1:50051 --domain-id customer-service
./bin/hnsx metrics --control-plane http://127.0.0.1:50051 --domain-id customer-service
```

Browse the Web UI at `http://127.0.0.1:50052` when the control plane is running with `--static-dir console/dist`.

---

## Harness 五要素

```yaml
harness:
  agents:      # 接入 Claude Code / Codex / OpenAI / Anthropic / Ollama / Custom
  prompts:     # System / Structured Prompts
  skills:      # 可复用业务能力包
  tools:       # 内置 HTTP / Shell / SQL / Python tools
  mcp:         # Model Context Protocol servers
  sandbox:     # 执行隔离策略
  policy:      # 预算、权限、guardrails
  memory:      # 跨会话上下文
  session:     # 会话模式：single-task / multi-turn / hierarchical / autonomous / workflow
```

完整 schema 见 [`design/Tech/V1/DomainSpec-v2.md`](design/Tech/V1/DomainSpec-v2.md)。

---

## Status

项目正在进行 **V1 Harness 运行时** 的 pivot：

- 从旧的 Workflow-DAG 运行时代码模型，迁移到以 **Harness / Session / Turn / Observation** 为核心的新模型。
- 新 domain spec 以 `harness` 为顶层字段，旧 `workflow` 语法保持兼容。
- 技术栈从 Rust 迁移到 **Go + Python + Vue** 异构架构。
- 控制面、Web UI、CLI 将按新架构重新实现。

详细路线图见 [`design/ProjectManagement/V1/RoadMap.md`](design/ProjectManagement/V1/RoadMap.md)。

| Phase | Milestone | Status |
|---|---|---|
| 0 | Product & Architecture design | ✅ |
| 1 | Go runtime skeleton | 🚧 |
| 2 | Harness runtime core | 🚧 |
| 3 | Tools / Skills / MCP skeleton | 🚧 |
| 4 | Agent adapters | 🚧 |
| 5 | Sandbox & Policy | 🚧 |
| 6 | Memory & Telemetry | 🚧 |
| 7 | Control plane | 🚧 |
| 8 | Web Console (Vue) | 🚧 |
| 9 | Python SDK | 🚧 |
| 10 | Build / Deploy / Release | 🚧 |

---

## Test

### Go

```bash
cd go
go test ./...
go vet ./...
```

### Python

```bash
cd python
pytest
```

### Console

```bash
cd console
pnpm type-check
pnpm lint
```

---

## License

Apache-2.0
