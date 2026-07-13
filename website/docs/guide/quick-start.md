# 快速开始

> 目标：5 分钟内让 HnsX 跑起来（**默认走路径 A：daemon 模式，零依赖**），完成一次端到端 Domain CRUD + 前端体验。

## 前置条件

**路径 A（默认，daemon 模式）**：
- macOS / Linux / WSL
- Go 1.22+（编译 server）
- Node 20+ 和 pnpm（运行 console）

**路径 B（Postgres 全栈）**：再加上 Docker + Docker Compose。

## 1. 启动 server

```bash
make build-server
./bin/hnsx-server server
```

零环境变量。Server 自动：
- 建 `~/.local/share/hnsx/hnsx.db`（SQLite）
- 生成 `secret.key`
- 应用 schema
- 监听 `127.0.0.1:50051`

## 2. 启动 console

新终端：

```bash
pnpm install --force
pnpm dev
```

浏览器打开 `http://localhost:5173`。

## 3. 浏览器点点点

- **Register Domain** → 弹 Monaco YAML 编辑器 → 粘下面这段 → Commit：

  ```yaml
  id: customer-service
  version: 1.0.0
  description: First demo
  harness:
    agents:
      main:
        id: main
        provider: anthropic
        model: claude-haiku-4-5
  session:
    mode: single
  ```

- 进 `/observability/debug` 看 Live Debug 入口
- 进 `/sessions` / `/traces` 看列表（SQLite daemon 模式下默认空，要 trigger session 才会有）

## 4. 想跑完整 Session / Trace / Worker 流程？

切到 **路径 B**（Postgres 全栈），见 [`local-dev.md`](./local-dev.md) 第 2 节。

## 下一步

- 本地完整手册 → [`local-dev.md`](./local-dev.md)
- CLI 命令速查 → [`cli-basics.md`](./cli-basics.md)
- Domain YAML 字段 → [`domain-spec.md`](./domain-spec.md)
- [为什么需要 Harness](/blog/why-harness)