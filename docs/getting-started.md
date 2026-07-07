# HnsX 快速上手指南

本文档介绍如何在本地启动 HnsX 控制面、注册 domain、运行 workflow，并通过 CLI / Web UI / REST API 查看结果。

---

## 前置条件

- Rust toolchain（1.85+）
- Node.js + pnpm（用于 Web UI）
- 可选：OpenAI / Anthropic API key（若使用真实 LLM adapter）

---

## 1. 启动控制面

控制面同时暴露：

- **gRPC**：默认 `127.0.0.1:50051`，供 CLI 注册/上报。
- **HTTP + Web UI**：默认 `127.0.0.1:50052`，供浏览器和 REST API 使用。

```bash
cargo run -p hnsx-cli -- control-plane \
  --addr 127.0.0.1:50051 \
  --db hnsx.db
```

启动成功后会打印：

```
[control-plane] serving gRPC on 127.0.0.1:50051 and HTTP on 127.0.0.1:50052
```

> 提示：`&#8203;``bash
> # 随机端口（适用于临时测试）
> cargo run -p hnsx-cli -- control-plane --addr 127.0.0.1:0 --db /tmp/hnsx.db
> ```

---

## 2. 注册 domain

把 `domains/` 下的某个 domain YAML 注册到控制面：

```bash
cargo run -p hnsx-cli -- register \
  --domain domains/customer-service/domain.yaml \
  --control-plane http://127.0.0.1:50051
```

成功后输出：

```
registered domain customer-service@0.1.0 at http://127.0.0.1:50051
```

---

## 3. 本地运行 domain

### 3.1 noop 模式（不依赖外部 API）

适合 CI 和快速验证：

```bash
cargo run -p hnsx-cli -- run \
  --domain domains/customer-service/domain.yaml \
  --adapter noop \
  --trigger '{"question":"hello"}'
```

### 3.2 真实 LLM 模式

```bash
export OPENAI_API_KEY=sk-...

cargo run -p hnsx-cli -- run \
  --domain domains/customer-service/domain.yaml \
  --adapter hnsx \
  --trigger '{"question":"how do I reset my password?"}'
```

> 也可以用 `--api-key sk-...` 临时覆盖，或写入 `.hnsx/secrets.yaml`。

### 3.3 运行并上报到控制面

```bash
cargo run -p hnsx-cli -- run \
  --domain domains/customer-service/domain.yaml \
  --adapter noop \
  --trigger '{"question":"hello"}' \
  --control-plane http://127.0.0.1:50051
```

运行产生的 step trace 和 invocation summary 会写入：

- 本地 `$HOME/.hnsx/traces/` 的 JSONL 文件
- 控制面 SQLite 数据库（`hnsx.db`）

---

## 4. 单 agent 测试

```bash
cargo run -p hnsx-cli -- test \
  --domain domains/customer-service/domain.yaml \
  --agent support \
  --adapter noop \
  --input '{"question":"hello"}'
```

上报控制面：

```bash
cargo run -p hnsx-cli -- test \
  --domain domains/customer-service/domain.yaml \
  --agent support \
  --adapter noop \
  --input '{"question":"hello"}' \
  --control-plane http://127.0.0.1:50051
```

---

## 5. 查看运行结果

### 5.1 CLI

```bash
# 查看指标
cargo run -p hnsx-cli -- metrics \
  --control-plane http://127.0.0.1:50051 \
  --domain-id customer-service

# 查看 traces
cargo run -p hnsx-cli -- traces \
  --control-plane http://127.0.0.1:50051 \
  --domain-id customer-service
```

### 5.2 REST API

```bash
# 列出已注册 domain
curl http://127.0.0.1:50052/api/v1/domains

# 列出某 domain 的 session
curl http://127.0.0.1:50052/api/v1/sessions/customer-service

# 列出某 domain 的全部 traces
curl http://127.0.0.1:50052/api/v1/traces/customer-service

# 按 session 过滤
curl "http://127.0.0.1:50052/api/v1/traces/customer-service?session_id=<session-id>"

# 查看指标
curl http://127.0.0.1:50052/api/v1/metrics/customer-service
```

### 5.3 Prometheus 指标

```bash
curl http://127.0.0.1:50052/metrics
```

---

## 6. 启动 Web UI

### 方式 A：静态文件 served by 控制面

先构建 UI：

```bash
cd web
pnpm install
pnpm run build
```

然后启动控制面时指定 `--static-dir`：

```bash
cargo run -p hnsx-cli -- control-plane \
  --addr 127.0.0.1:50051 \
  --db hnsx.db \
  --static-dir web/dist
```

浏览器访问 `http://127.0.0.1:50052`。

### 方式 B：Vite 开发服务器（推荐开发时用）

终端 1 启动控制面（可以不带 `--static-dir`）：

```bash
cargo run -p hnsx-cli -- control-plane --addr 127.0.0.1:50051 --db hnsx.db
```

终端 2 启动 Vite dev server：

```bash
cd web
pnpm dev
```

浏览器访问 `http://localhost:5173`。

> Vite 会把 `/api/*` 和 `/metrics` 代理到 `http://127.0.0.1:50052`。

---

## 7. 完整端到端流程示例

```bash
# 1. 启动控制面
cargo run -p hnsx-cli -- control-plane --addr 127.0.0.1:50051 --db hnsx.db

# 2. 在另一个终端注册 domain
cargo run -p hnsx-cli -- register \
  --domain domains/customer-service/domain.yaml \
  --control-plane http://127.0.0.1:50051

# 3. 运行 domain 并上报
cargo run -p hnsx-cli -- run \
  --domain domains/customer-service/domain.yaml \
  --adapter noop \
  --trigger '{"question":"hello"}' \
  --control-plane http://127.0.0.1:50051

# 4. 查看 trace
cargo run -p hnsx-cli -- traces \
  --control-plane http://127.0.0.1:50051 \
  --domain-id customer-service

# 5. 启动 Web UI（再开一个终端）
cd web && pnpm dev
```

---

## 8. 注销 domain

```bash
cargo run -p hnsx-cli -- unregister \
  --id customer-service \
  --version 0.1.0 \
  --control-plane http://127.0.0.1:50051
```

---

## 9. 常见问题

### 控制面 HTTP 端口显示为 `127.0.0.1:1`

确保使用最新代码。旧版本在 `--addr 127.0.0.1:0` 时会把 HTTP 端口算成 `0 + 1 = 1`，已修复为先绑定 gRPC listener 再取真实端口。

### Web UI 报 CORS 错误

控制面 HTTP API 已配置 `allow_origin(Any)`。如果出现 CORS 问题，请确认访问的是 HTTP 端口（默认 50052），而不是 gRPC 端口。

### `hnsx run` 没有上报 trace

只有显式加了 `--control-plane` 才会上报。否则只写本地 JSONL trace。

### 如何查看本地 JSONL trace

默认目录：

```bash
ls $HOME/.hnsx/traces
```

也可以用 `hnsx metrics --trace-dir /path/to/traces` 或 `hnsx traces --trace-dir /path/to/traces` 查看。
