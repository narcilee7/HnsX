# 快速开始

> 目标：5 分钟内让 HnsX 在本地跑起来——Postgres + server + console 三条命令完事。

## 前置条件

- macOS / Linux / WSL
- Go 1.22+（编译 server）
- Node 20+ 和 pnpm（运行 console）
- Postgres（任何方式——Homebrew / Docker / 系统包都行）

## 1. 启动 Postgres

任选一种：

```bash
# Homebrew（macOS）
brew install postgresql@16 && brew services start postgresql@16
createuser hnsx --pwprompt   # 密码 hnsx
createdb -O hnsx hnsx

# Docker（最干净）
docker run --name hnsx-pg -d -p 5432:5432 \
  -e POSTGRES_USER=hnsx -e POSTGRES_PASSWORD=hnsx -e POSTGRES_DB=hnsx \
  postgres:16
```

DSN：`postgres://hnsx:hnsx@localhost:5432/hnsx?sslmode=disable`

## 2. 启动 server

```bash
make build-server
HNSX_DATABASE_URL='postgres://hnsx:hnsx@localhost:5432/hnsx?sslmode=disable' \
  ./bin/hnsx-server server
```

Server 自动：
- 跑 goose migrations（`migrations applied`）
- 首次启动生成 `~/.local/share/hnsx/secret.key`
- 监听 `:50051`（REST + Connect/gRPC）

## 3. 启动 console

新终端：

```bash
pnpm install --force
pnpm dev
```

浏览器打开 `http://localhost:5173`。

## 4. 浏览器点点点

- **Register Domain** → Monaco YAML 编辑器 → 粘下面这段 → Commit：

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
- 进 `/sessions` / `/traces` 看列表

## 5. 让 session 真跑起来（可选）

需要 worker 在跑，否则 session 卡 pending：

```bash
make worker-install
HNSX_SERVER=127.0.0.1:50061 \
  .venv/bin/python -m hnsx_worker.worker_service
```

## 下一步

- 本地完整手册（包含 worker / launchd / Python gRPC 详解）→ [`local-dev.md`](./local-dev.md)
- CLI 命令速查 → [`cli-basics.md`](./cli-basics.md)
- Domain YAML 字段 → [`domain-spec.md`](./domain-spec.md)
- [为什么需要 Harness](/blog/why-harness)