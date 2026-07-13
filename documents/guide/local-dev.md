# 本地开发体验

> 目标：从源码拉下来一条命令起服务，另一条命令起前端，浏览器里点点点跑完整个产品。
> 这一份只覆盖"我要用 HnsX 看看是什么"，不覆盖"我要给 HnsX 写代码"——后者见 [`CONTRIBUTING.md`](../../CONTRIBUTING.md)。

---

## 路径 A：单二进制 daemon 模式（推荐先试这个）

HnsX v1.0 起，`hnsx-server` 可以脱离 CLI / docker-compose 单独跑起来：内嵌 SQLite、自动建库、自动生成 secret-key、监听 `:50051`。**Domain 注册 / 列表 / 详情 / 多版本** 这条核心路径在这条路径上完整可用。

### 1. 启动 server

```bash
# 仓库根目录
make build-server
./bin/hnsx-server server
```

你会看到：

```
{"level":"info","msg":"daemon mode: using embedded SQLite","path":"~/.local/share/hnsx/hnsx.db"}
{"level":"info","msg":"auto-generated HNSX_SECRET_KEY","path":"~/.local/share/hnsx/secret.key","action":"BACK THIS UP — losing this file means losing access to encrypted secrets"}
{"level":"info","msg":"sqlite schema applied (tenants + domains + domain_versions)"}
{"level":"info","msg":"hnsx-server listening","http":"127.0.0.1:50051"}
```

数据 + secret 默认落在 `~/.local/share/hnsx/`。覆盖路径：

```bash
HNSX_DAEMON_DATA_DIR=/tmp/my-hnsx-data ./bin/hnsx-server server
```

健康检查：

```bash
curl http://127.0.0.1:50051/healthz
# {"build":{...},"status":"ok"}
```

### 2. 启动前端

新开一个终端：

```bash
pnpm install --force
pnpm dev
```

Vite 默认监听 `http://localhost:5173`。前端默认指向 `http://localhost:50051/api/v1`，跟 server 默认端口对得上，**不用改配置就能连**。

### 3. 浏览器点点点

打开 `http://localhost:5173`，按这个顺序走：

| 步骤 | 路径 | 操作 |
|---|---|---|
| 1 | `/domains` | 点 **Register Domain** → 弹出一个 Monaco YAML 编辑器。粘贴： |

```yaml
id: customer-service
version: 1.0.0
description: First demo domain — customer support triage
harness:
  agents:
    main:
      id: main
      description: Helpful support agent
      provider: anthropic
      model: claude-haiku-4-5
  session:
    mode: single
```

点 **Commit**，域名出现在列表里。

| 步骤 | 路径 | 操作 |
|---|---|---|
| 2 | `/domains/customer-service` | 点 **Editor** 改 YAML，**Save** 后再点 **Versions** 看版本历史。**Debug** tab 列了这个 domain 的最近 sessions（SQLite 模式下表是空的，要先 trigger session 才有）。 |
| 3 | `/observability/debug` | 新加的 Live Debug 入口：跨 domain 列最近 sessions + traces，按 domain / agent 过滤。点任意 session 进详情看 SSE 流的 observation timeline。 |
| 4 | `/sessions` | 全局 session 列表，跑中的 session 多了 Pause / Cancel 按钮，已暂停的多 Resume。 |
| 5 | `/traces` | Trace 列表，含 duration / observation count / 费用摘要。 |

### 4. 用 Python 注册一个 domain（可选）

路径 A 完整支持 Python 端 gRPC 注册，无需经过 CLI 或 REST：

```python
from hnsx_worker.proto_client import ControlPlaneClient

c = ControlPlaneClient("127.0.0.1:50061", tenant_id="<your-tenant>")
ref = c.register_domain({
    "id": "from-python",
    "version": "1.0.0",
    "description": "registered via gRPC",
    "harness": {"agents": {"main": {"id": "main", "provider": "anthropic", "model": "claude-haiku-4-5"}}},
    "session": {"mode": "single"},
})
print(ref)  # DomainRef(id='from-python@1.0.0', version=1)
```

切回 `/domains` 看，刚注册的已经在列表里。

### 5. SQLite daemon 模式的边界

能跑：

- ✅ Register / List / Get / Update Domain
- ✅ Domain YAML 验证
- ✅ Versioning + rollback
- ✅ Python gRPC + REST + Console 三路注册互通

**不能跑**（v1.0 暂未 port，留 v1.1）：

- ❌ Trigger Session / 跑 Harness / 收 Observation（sessions / traces 表还没 SQLite schema）
- ❌ Audit / Approval / Secret / Eval
- ❌ Worker 注册 / Heartbeat（worker 还是直连 Postgres）

如果想试这些就走 **路径 B**。

---

## 路径 B：Postgres 全栈（功能完整）

适合想看完整 Session / Trace / Eval / Worker 流程的。需要在本地跑 Postgres + HnsX server + Python worker。

### 前置条件

- macOS / Linux
- Docker + Docker Compose
- Go 1.22+（编译 server / CLI）
- Node 20+ + pnpm（编译 console）
- Python 3.11+ + venv（worker）

### 1. 拉起 Postgres

```bash
make db-up        # docker compose 起一个 Postgres
```

### 2. 编译 + 起 server（连 Postgres）

```bash
make build
HNSX_DATABASE_URL=postgres://hnsx:hnsx@localhost:5432/hnsx?sslmode=disable \
  ./bin/hnsx-server server
```

日志里 `migrations applied` 表示 schema 已经建好。

### 3. 起 worker

```bash
make worker-install      # 一次：建 venv + 装依赖
HNSX_SERVER=127.0.0.1:50061 \
  .venv/bin/python -m hnsx_worker.worker_service
```

worker 会自动 register + heartbeat + 从 server 拉 session。

### 4. 起 console

```bash
pnpm install --force
pnpm dev
```

### 5. 体验全流程

| 步骤 | 路径 | 看到什么 |
|---|---|---|
| 1 | `/domains` | Register 一个 domain（同上） |
| 2 | `/domains/customer-service/run` | Trigger Session：填 trigger payload、点 Run |
| 3 | `/sessions` | Session 出现，state=running |
| 4 | 点进 session | Timeline 实时滚动（tool_call / tool_result / agent_text_delta） |
| 5 | 点 Pause | State → paused；worker 停拉下一个 turn（当前 turn 跑完才停） |
| 6 | 点 Resume | State → running，继续 |
| 7 | `/traces` | Trace 列表带 duration / token / cost |
| 8 | `/observability/debug` | 跨 domain 调试入口 |
| 9 | `/audit` | Audit log（Domain 注册 / Session 取消等操作记录） |

### 6. CLI 联动

CLI 不是必须，但装了可以快速验证：

```bash
hnsx version
hnsx domain list
hnsx session list --state running
hnsx trace list
hnsx trace show <id>
hnsx update --check
```

---

## 三条安装路径快速对照

| | curl | Homebrew | 源码 |
|---|---|---|---|
| CLI | ✅ | ✅ | ✅ |
| Server | (通过 homebrew plist 自动 launchd) | (同左) | `make build-server && ./bin/hnsx-server server` |
| Worker | ❌（自己 `pip install hnsx-worker`） | ❌（同左） | `make worker-install && python -m hnsx_worker.worker_service` |
| Console | ❌（自己 `pnpm dev`） | ❌（同左） | ❌（同左） |
| 一句话 | 全自动要 launchd | 同左 | 最灵活，能改东西 |

详见 [`install.md`](./install.md)。

---

## 排错速查

| 现象 | 排查 |
|---|---|
| `curl: (52) Empty reply from server` for `hnsx.dev` | 这个域名还没接通，**不要用**。从源码跑用路径 A / B。 |
| console 连不上 server | 检查 `HNSX_HTTP_ADDR` 是不是 `:50051`，Vite 是不是 `:5173`。CORS 已经全开（`Access-Control-Allow-Origin: *`）。 |
| SQLite daemon 起不来，提示 `sqlite schema applied` 后崩溃 | 检查 `~/.local/share/hnsx/` 是不是只读，权限够不够。 |
| Postgres 模式下 `migrations skipped` | SQLite 模式才出这条；Postgres 下 `migrations applied` 才是正常的。 |
| Worker 报 `no such table: sessions` | SQLite 模式预期行为。要跑 worker 就走路径 B（Postgres）。 |
| Python `register_domain` 报 `gen_random_uuid` 错 | 这条是 v1.0 已修复的；如果你在用 v1.0 之前的版本，先升级。 |

---

## 下一步

- [`cli-basics.md`](./cli-basics.md) — CLI 命令表
- [`domain-spec.md`](./domain-spec.md) — Domain YAML 字段参考
- [Architecture](../../architecture.md) — 内部组件图