# HnsX 标准操作流程（SOP）

> 面向开发者、运维和 Harness 设计者的本地开发到 E2E 验证操作手册。

---

## SOP-01 启动完整本地环境

### 目标

在本地一键拉起：Postgres + hnsx-server + hnsx-worker + Tempo + Grafana，用于开发和 E2E 验证。

### 前置条件

- Docker + Docker Compose 已安装
- 端口未被占用：`5433`（Postgres）、`50052`（HTTP）、`50062`（gRPC）、`3200`（Tempo query）、`3002`（Grafana）、`4317`/`4318`（OTLP）
- `hnsx` CLI 已构建（`make build-cli` 或 `brew install hnsx`）

### 操作步骤

**推荐：用 CLI（v1.0+）**

```bash
hnsx up                  # 起 postgres + server + worker，阻塞至 /healthz 通过
hnsx up --with-telemetry # 额外起 Tempo + Grafana
hnsx up --detach         # 后台启动
```

**等价原始方式**（CLI 不可用时）

```bash
cd deployments/local
docker compose up -d
```

### 验证

```bash
# 1. 一键诊断
hnsx doctor              # 检查 docker / compose file / repo root / server-health
hnsx status              # 表格列出每个 service 的状态
```

或用原始方式：

```bash
docker compose ps
curl -fsS http://127.0.0.1:50052/healthz
curl -fsS http://127.0.0.1:50052/api/v1/runtimes
```

### 预期结果

- `local-server-1` 状态为 `healthy`
- `local-worker-1` 已启动
- Grafana 可访问：`http://127.0.0.1:3002`（账号 `admin` / 密码 `hnsx`）

### 故障排查

| 现象 | 处理 |
|---|---|
| server 反复 unhealthy | `hnsx logs server` 或 `docker compose logs server` 看 DB 连接或迁移错误 |
| worker 连不上 server | 检查 `HNSX_SERVER` 是否为 `server:50061`，以及 server gRPC 是否 healthy |
| Tempo 起不来 | 检查 `deployments/local/tempo.yaml` 是否兼容当前镜像版本 |
| `hnsx doctor` 报缺 docker | 安装 Docker Desktop；macOS：`brew install --cask docker` |

---

## SOP-02 触发并观测一次 Session

### 目标

验证从触发到 Worker 执行、SSE 推送、Trace 落库、Grafana 可查的完整链路。

### 操作步骤

**推荐：用 CLI（v1.0+）**

```bash
# 0. 第一次尝试：直接一键试一个示例
hnsx try noop-smoke                       # 自动 up + register + trigger + tail

# 1. 触发一个 Session
SID=$(hnsx session trigger --domain customer-service \
  --trigger '{"question":"hello"}' --output quiet)
echo "session=$SID"

# 2. 实时观察 SSE 流（Ctrl-C 退出）
hnsx session tail "$SID"

# 3. 表格列出最近 Session
hnsx session list --limit 5 --state completed

# 4. 列 Trace 并查看详情
hnsx trace list --domain customer-service --limit 5
hnsx trace show <trace-id>
```

**等价原始方式**（CLI 不可用时）

```bash
SID=$(curl -fsS -X POST http://127.0.0.1:50052/api/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"domain_id":"customer-service","trigger":{"question":"hello"}}' | jq -r .id)
curl -N http://127.0.0.1:50052/api/v1/sessions/$SID/events
curl -fsS http://127.0.0.1:50052/api/v1/sessions/$SID
curl -fsS "http://127.0.0.1:50052/api/v1/traces?domain=customer-service"
```

### 预期结果

- Session 最终 `state` 为 `completed`
- SSE 流输出 `session_start`、`step_start`、`agent_invoke`、`step_end`、`session_end` 等事件
- Trace API 返回 ≥1 条 trace，包含 observations[]
- Tempo 中按 `session_id` 可查到 spans：`http://127.0.0.1:3200`

### 故障排查

| 现象 | 处理 |
|---|---|
| Session 状态为 `failed` | 查 DB：`SELECT kind, payload FROM observations WHERE session_id='...' AND kind='session_end';` |
| Worker 没执行 | `hnsx logs worker` 或 `docker compose logs worker` |
| SSE 没事件 | 确认请求后 session 还在运行；已完成 session 不会补发历史事件 |
| Tempo 搜不到 | 等 5-10s 让 live-store flush；或 `curl -s localhost:3200/metrics \| grep spans_received` |
| 想看更深的诊断 | `hnsx power debug-bundle -o bug.tar.gz` 一键打包版本/状态/最近 20 个 session+trace |

---

## SOP-03 新增一个 Example Domain

### 目标

在 `example-domains/` 下新增一个可运行、可验证的 DomainSpec。

### 操作步骤

1. 复制模板目录：

```bash
cp -r example-domains/noop-smoke example-domains/my-domain
cd example-domains/my-domain
```

2. 编辑 `domain.yaml`，确保包含：

- `id`：全局唯一 domain id
- `harness.agents`：至少一个 agent，指定 `adapter.kind`（本地推荐 `noop` 或 `echo`）
- `harness.session`：mode（`single` / `multi-turn` / `workflow`）
- `harness.prompts`：agent 引用的 prompt

3. 验证 YAML 结构：

```bash
cd /Users/Zhuanz/business_project/HnsX
hnsx validate --domain example-domains/my-domain/domain.yaml --json
```

4. 本地单测运行（不启动 server）：

```bash
hnsx run \
  --domain example-domains/my-domain/domain.yaml \
  --adapter noop \
  --trigger '{"question":"test"}' \
  --json
```

5. 一键跑一遍（CLI 会自动 up + register + trigger + tail）：

```bash
hnsx try my-domain
```

或手动重启 server 让新 domain 从 seed 加载，然后触发：

```bash
hnsx restart                                              # down + up
hnsx domain register example-domains/my-domain/domain.yaml
hnsx session trigger --domain my-domain --trigger '{"question":"test"}'
```

### 预期结果

- `hnsx validate` 输出 `valid: true`
- `hnsx run` 输出最终文本且 rc=0
- API 触发后 session 状态为 `completed`

### 注意事项

- 不要直接在 `example-domains/` 下新增文件，必须是一个子目录 + `domain.yaml`
- 如果 agent 使用 `anthropic` / `openai` adapter，需配置对应 API Key，否则 Worker 会失败
- 本地 docker compose 默认通过 `HNSX_FORCE_ADAPTER_KIND=noop` 覆盖真实 adapter

---

## SOP-04 运行 Smoke 测试

### 目标

在修改代码后快速验证 server 核心链路是否仍通。

### 操作步骤

**Server 链路 smoke**

```bash
cd /Users/Zhuanz/business_project/HnsX
make build-server
./scripts/smoke.sh
```

**CLI surface smoke（v0.3-v1.0）**

```bash
./scripts/smoke-cli.sh
```

后者在 docker compose 栈启动后一键验证 `hnsx doctor` / `hnsx status` / `hnsx examples` / `hnsx try` / `hnsx session {list,trigger}` / `hnsx trace list` / `hnsx eval set list` / `hnsx governance {policy,approval,audit,secret,auth}` / `hnsx power {format,diff,replay,debug-bundle}`。

### 预期结果

输出 `ALL GOOD ✓`，包含：

- example domains 验证通过
- server 启动成功
- REST 契约正常
- Session 触发并完成
- Trace API 返回 trace 详情
- Secrets / Policies CRUD 正常

### 故障排查

| 现象 | 处理 |
|---|---|
| 端口 51002 被占用 | 杀掉占用进程或改 `smoke.sh` 第一个参数：`./scripts/smoke.sh 127.0.0.1:51003` |
| trace list 返回空 | 检查 `internal/telemetry/db.go` metadata 是否正常写入；或增加重试 |
| Postgres 连接失败 | 确认 docker compose postgres 在跑，且端口 `5433` 映射正确 |

---

## SOP-05 调试一次失败的 Session

### 目标

定位 Session 失败原因。

### 操作步骤

1. **一键打包**：把所有诊断资料打包后贴 issue

```bash
hnsx power debug-bundle -o bug.tar.gz
```

2. **获取 session 详情**：

```bash
hnsx session show <sid>
hnsx trace show <trace-id>
```

或用 curl：

```bash
curl -fsS http://127.0.0.1:50052/api/v1/sessions/$SID
curl -fsS "http://127.0.0.1:50052/api/v1/traces?domain=customer-service" | jq
curl -fsS http://127.0.0.1:50052/api/v1/traces/$TRACE_ID
```

3. **直接查 DB**（如果 API 无数据）：

```bash
docker compose -f deployments/local/docker-compose.yaml exec postgres psql -U hnsx -d hnsx -c "
SELECT kind, agent_id, payload FROM observations
WHERE session_id='$SID' ORDER BY created_at;
"
```

4. **查看日志**：

```bash
hnsx logs worker --tail 50   # 或 docker compose logs worker --tail=50
hnsx logs server --tail 50   # 或 docker compose logs server --tail=50
```

### 常见失败原因

| 原因 | 特征 | 处理 |
|---|---|---|
| 缺 API Key | `error: API key env var 'ANTHORPIC_API_KEY' is not set` | docker compose 已默认 `HNSX_FORCE_ADAPTER_KIND=noop`，如未生效检查 worker env |
| Domain 不存在 | `DOMAIN_NOT_FOUND` | 确认 domain id、server 是否 seed 成功 |
| Worker 未注册 | Session 一直 pending | 检查 worker 是否 healthy、grpc 是否通 |
| 策略拦截 | `policy:` 开头错误 | 调整 DomainSpec 的 policy.budget / policy.permissions |

---

## SOP-06 构建并推送

### 目标

完成改动后按规范提交代码。

### 操作步骤

1. 本地验证：

```bash
make build-server
./scripts/smoke.sh
```

2. 检查改动范围：

```bash
git status --short
git diff --stat
```

3. 按 Conventional Commits 提交：

```bash
git add -A
git commit -m "feat(server): 一句话描述

- 改动点 1
- 改动点 2

验证：./scripts/smoke.sh ALL GOOD"
```

4. 推送到远端：

```bash
git push origin feat/connection_e2e
```

### 注意事项

- 不要直接 push 到 `main`
- commit body 里写明验证命令和结果
- 改 docs 的 commit 用 `docs(...)`，改 bug 用 `fix(...)`，新功能用 `feat(...)`
- 多个无关改动建议分 commit

---

## SOP-07 更新项目文档

### 目标

当产品能力或接口变化时，同步更新对外文档。

### 需要同步的场景

| 改动 | 必须更新的文档 |
|---|---|
| 新增 API endpoint | `docs/server-design/api-design.md`、README 里的 API 表（如还在维护） |
| 改 DomainSpec 模型 | `docs/know-how/我们如何建模Harness.md`、example-domains、protobuf |
| 改 Observation / Trace | `docs/know-how/我们如何观测Harness与Agent.md`、SOP-05 |
| 改 Console 页面或路由 | `docs/web-console-design/整体设计.md` |
| 改部署方式 | `README.md` quick start、`deployments/local/docker-compose.yaml`、本 SOP |
| 改 Eval 机制 | `docs/know-how/我们如何评测Harness与Agent.md` |

### 操作步骤

1. 改代码前先确认相关设计文档是否需要更新
2. 代码 commit 与 docs commit 一起提交（同一主题）
3. 提交后抽查 README / CLAUDE.md / SOP 是否仍一致

---

## CLI 速查

> 完整命令词表见 [`docs/cli-roadmap.md`](cli-roadmap.md) §2。本节只列最常用。

| 想做什么 | 命令 |
|---|---|
| 装好 CLI | `make build-cli` / `brew install hnsx` / `curl -sSL hnsx.dev/install.sh \| sh` |
| 起 / 停栈 | `hnsx up` / `down` / `status` / `doctor` / `logs [-f] [-s server\|worker]` / `reset --yes` |
| 列示例 / 一键试 | `hnsx examples` / `hnsx try <name>` |
| 列 / 注册 / 删 Domain | `hnsx domain list/show/register/delete/export` |
| 触发 / 看 Session | `hnsx session trigger\|tail\|list\|show\|cancel\|rerun\|approve\|reject` |
| 看 Trace | `hnsx trace list/show/export` |
| 跑 Eval | `hnsx eval set list/show/create/delete` / `hnsx eval run start\|list\|show\|diff` |
| Policy / Secret | `hnsx governance policy apply\|delete --confirm` |
| Approval | `hnsx governance approval list\|approve\|reject\|watch` |
| Audit | `hnsx governance audit list --actor ... --csv file.csv` |
| Auth | `hnsx governance auth login\|status\|logout` |
| Domain 高级 | `hnsx power format\|diff\|replay\|debug-bundle` |
| 打开 GUI | `hnsx console` |
| 自更新 | `hnsx update --check` |

通用 flag（每条 list 命令都支持）：

- `--limit N` — 最大行数
- `--filter k=v` — 过滤，可重复
- `--since 5m|1h|2d` — 时间窗
- `--output human|json|quiet` — 默认 human，CI 用 json/quiet

config 三层：`--flag` > `HNSX_*` env > `~/.config/hnsx/*.yaml`。

---

*本 SOP 与 `README.md`、`CLAUDE.md`、roadmap 一起维护，产品能力变化时同步更新。*
