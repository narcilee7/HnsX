# 本地开发体验

> 目标：从源码拉下来，一条命令起 Postgres，一条命令起 server，一条命令起 console，浏览器里点点点看产品。
> 重点是**让本地开发尽可能无痛**：DB 准备好、server 自己起、log 落地、Console 能连上。

---

## 全栈概览

```
                  ┌─────────────────────────────┐
                  │ hnsx-server (Go)              │  HTTP :50051  ← Console
   ┌────────┐     │ - REST API                    │  gRPC :50061  ← Worker (raw gRPC)
   │Postgres│◄────│ - Connect/gRPC mux (REST+ gRPC│
   │ :5432 │     │ - 自动迁移 + 自动 secret-key  │
   └────────┘     └─────────────────────────────┘
                          ▲             ▲
                          │             │
                  ┌───────┴────┐  ┌─────┴──────┐
                  │ hnsx-worker│  │ hnsx-console│
                  │ (Python)   │  │ (React/Vite)│
                  │ gRPC client│  │ :5173       │
                  └────────────┘  └────────────┘
```

**一个进程 = 一个 daemon**。`hnsx-server` 直接连本地 Postgres，无需 docker-compose 来包一层。

---

## 0. 前置条件

| 工具 | 用途 | 验证 |
|---|---|---|
| Go 1.22+ | 编译 server | `go version` |
| Node 20+ + pnpm | 跑 console | `node -v && pnpm -v` |
| Python 3.11+ + venv | 跑 worker（可选，路径 A 不需要） | `python3 --version` |
| Postgres（任何方式都行） | 数据持久化 | 见下一节 |

Postgres 的几种取法：

```bash
# A. Homebrew / apt — 最简单
brew install postgresql@16 && brew services start postgresql@16
createuser hnsx --pwprompt   # 设密码 hnsx
createdb -O hnsx hnsx

# B. Docker（一次性，最干净）
docker run --name hnsx-pg -d -p 5432:5432 \
  -e POSTGRES_USER=hnsx -e POSTGRES_PASSWORD=hnsx -e POSTGRES_DB=hnsx \
  postgres:16

# C. 系统包管理器
sudo apt install postgresql
sudo -u postgres createuser hnsx --pwprompt
sudo -u postgres createdb -O hnsx hnsx
```

**Postgres 不是 daemon 模式的一部分**。server 假定 Postgres 已经存在并可连。

---

## 1. 启动 server

```bash
make build-server
HNSX_DATABASE_URL='postgres://hnsx:hnsx@localhost:5432/hnsx?sslmode=disable' \
  ./bin/hnsx-server server
```

或者把 `HNSX_DATABASE_URL` 写到 `~/.zshrc` / `~/.bashrc` 里省去每次输入。

正常输出：

```
{"msg":"migrations applied","dir":"go/migrations"}
{"msg":"secret store: encryption enabled"}
{"msg":"hnsx-server listening","http":"127.0.0.1:50051","grpc":"127.0.0.1:50061"}
```

**Daemon-mode 提供的便利**（与 Postgres 解耦）：

| env | 默认 | 作用 |
|---|---|---|
| `HNSX_DAEMON_DATA_DIR` | `~/.local/share/hnsx` | secret-key 等 daemon 状态 |
| `HNSX_SECRET_KEY` | (auto-gen on first boot) | 加密 secret 用；首次启动后从 `secret.key` 读 |
| `HNSX_MIGRATIONS_DIR` | `go/migrations` | goose 迁移目录 |
| `HNSX_HTTP_ADDR` | `127.0.0.1:50051` | REST + Connect 监听 |
| `HNSX_GRPC_ADDR` | `127.0.0.1:50061` | 给 worker 的原始 gRPC（不是 Connect） |

健康检查：

```bash
curl http://127.0.0.1:50051/healthz
# {"build":{...},"status":"ok"}
```

把 server 推到后台跑（不想占终端）：

```bash
# 写到 launchd / systemd（生产路径）
make daemon-install
# 或手动后台
nohup ./bin/hnsx-server server > /tmp/hnsx.log 2>&1 &
tail -f /tmp/hnsx.log
```

---

## 2. 启动 console

新终端：

```bash
pnpm install --force
pnpm dev
```

Vite 默认 `http://localhost:5173`，CORS 全开，连得上 `:50051` 默认 config，**不用改配置**。

---

## 3. 浏览器点点点

打开 `http://localhost:5173`：

| 步骤 | 路径 | 操作 |
|---|---|---|
| 1 | `/domains` | **Register Domain** → 弹 Monaco YAML 编辑器 → 粘贴： |

```yaml
id: customer-service
version: 1.0.0
description: Customer support triage
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
| 2 | `/domains/customer-service` | **Editor** 改 YAML → **Save** → **Versions** 看历史 → **Debug** 看这个 domain 的最近 sessions |
| 3 | `/observability/debug` | 跨 domain 调试入口：左侧 sessions、右侧 traces，按 domain / agent 过滤。点 session 进详情看实时 SSE 流 |
| 4 | `/sessions` | 列表，跑中的 session 有 Pause / Cancel；暂停的有 Resume |
| 5 | `/traces` | 列表带 duration / observation count / 费用 |

---

## 4. 跑 worker（让 session 真能跑起来）

需要 worker 是因为你 trigger session 时要有人执行；不然状态会卡在 pending。

```bash
make worker-install                                # 一次：建 venv + 装依赖
HNSX_SERVER=127.0.0.1:50061 \
  .venv/bin/python -m hnsx_worker.worker_service
```

正常输出：

```
{"msg":"worker registered","worker_id":"w-...","heartbeat_interval":5}
{"msg":"heartbeat sent"}
{"msg":"session pulled","session_id":"s-..."}
```

回到 console 点 `/domains/customer-service/run`，trigger 一个 session，timeline 会实时滚 observation。

---

## 5. 用 Python 注册 domain

API 完整，路径 A、B 都行——只是换 transport：

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

切回 `/domains` 看，刚刚 Python 注册的已经在列表里——三路注册（gRPC / REST / Console）共用同一个 Postgres 表。

---

## 6. 想跑 CLI 命令？

CLI 不是必须的，但装了能快速验证：

```bash
brew install narcilee7/hnsx/hnsx
# 或源码：
make build-cli
./bin/hnsx version
./bin/hnsx domain list
./bin/hnsx session list --state running
./bin/hnsx trace list
./bin/hnsx trace show <id>
```

---

## 三条安装路径对照

| | curl | Homebrew | 源码 |
|---|---|---|---|
| CLI | ✅ | ✅ | `make build-cli` |
| Server | (通过 homebrew plist launchd) | (同左) | `make build-server && hnsx-server server` |
| Worker | ❌（自己 `pip install hnsx-worker`） | ❌（同左） | `make worker-install && python -m hnsx_worker.worker_service` |
| Console | ❌（自己 `pnpm dev`） | ❌（同左） | ❌（同左） |
| 一句话 | 全自动，launchd 拉起 server | 同左 | 最灵活，能改东西 |

详见 [`install.md`](./install.md)。

---

## 排错速查

| 现象 | 排查 |
|---|---|
| `curl: (52) Empty reply from server` for `hnsx.dev` | 这个域名还没接通，**不要用**。从源码跑就用上面的 1+2+3。 |
| server 起不来 + `postgres is required` | 没设 `HNSX_DATABASE_URL`。Postgres 是 server 必需的——单独建一个本地实例。 |
| `migrations failed: relation already exists` | 上次跑了一半的迁移。`psql ... -c "DROP TABLE goose_db_version"` 然后重试（数据不会丢因为 goose 只跟踪 schema）。 |
| Console 连不上 server | 检查 `HNSX_HTTP_ADDR` 跟 console 跑的端口对不对。CORS 已开。 |
| Worker 报 `no such table: sessions` | migrations 没跑完。`make db-up` 跑全套，或直接重启 server 让它跑 goose。 |
| Python `register_domain` 失败 | v1.0 之前有过 gen_random_uuid 兼容 bug，确保用最新代码（Phase 1 PR #33 修了）。 |
| `secret store: encryption enabled` 失败 + `set HNSX_SECRET_KEY` 错误 | 没找到 `~/.local/share/hnsx/secret.key` 也没设 env。第一次会自动生成，删目录重跑就好。 |

---

## 下一步

- [`cli-basics.md`](./cli-basics.md) — CLI 命令表
- [`domain-spec.md`](./domain-spec.md) — Domain YAML 字段参考
- [Architecture](../../architecture.md) — 内部组件图