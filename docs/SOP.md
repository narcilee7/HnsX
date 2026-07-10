# HnsX 标准操作流程（SOP）

> 面向开发者、运维和 Harness 设计者的本地开发到 E2E 验证操作手册。

---

## SOP-01 启动完整本地环境

### 目标

在本地一键拉起：Postgres + hnsx-server + hnsx-worker + Tempo + Grafana，用于开发和 E2E 验证。

### 前置条件

- Docker + Docker Compose 已安装
- 端口未被占用：`5433`（Postgres）、`50052`（HTTP）、`50062`（gRPC）、`3200`（Tempo query）、`3002`（Grafana）、`4317`/`4318`（OTLP）

### 操作步骤

```bash
cd deployments/local
docker compose up -d
```

### 验证

```bash
# 1. 所有服务 healthy
docker compose ps

# 2. Server healthz
curl -fsS http://127.0.0.1:50052/healthz

# 3. Worker 已注册
curl -fsS http://127.0.0.1:50052/api/v1/runtimes
```

### 预期结果

- `local-server-1` 状态为 `healthy`
- `local-worker-1` 已启动
- Grafana 可访问：`http://127.0.0.1:3002`（账号 `admin` / 密码 `hnsx`）

### 故障排查

| 现象 | 处理 |
|---|---|
| server 反复 unhealthy | `docker compose logs server` 看 DB 连接或迁移错误 |
| worker 连不上 server | 检查 `HNSX_SERVER` 是否为 `server:50061`，以及 server gRPC 是否 healthy |
| Tempo 起不来 | 检查 `deployments/local/tempo.yaml` 是否兼容当前镜像版本 |

---

## SOP-02 触发并观测一次 Session

### 目标

验证从 API 触发到 Worker 执行、SSE 推送、Trace 落库、Grafana 可查的完整链路。

### 操作步骤

```bash
# 1. 创建 Session
SID=$(curl -fsS -X POST http://127.0.0.1:50052/api/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"domain_id":"customer-service","trigger":{"question":"hello"}}' | jq -r .id)
echo "session=$SID"

# 2. 实时观看 Observation 流
curl -N http://127.0.0.1:50052/api/v1/sessions/$SID/events

# 3. 查询 Session 状态
curl -fsS http://127.0.0.1:50052/api/v1/sessions/$SID

# 4. 查询 Trace 列表
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
| Worker 没执行 | 检查 worker 日志：`docker compose logs worker` |
| SSE 没事件 | 确认请求后 session 还在运行；已完成 session 不会补发历史事件 |
| Tempo 搜不到 | 等 5-10s 让 live-store flush；或查 `curl -s localhost:3200/metrics \| grep spans_received` |

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
./bin/hnsx validate --domain example-domains/my-domain/domain.yaml --json
```

4. 本地单测运行（不启动 server）：

```bash
./bin/hnsx run \
  --domain example-domains/my-domain/domain.yaml \
  --adapter noop \
  --trigger '{"question":"test"}' \
  --json
```

5. 启动或重启 docker compose，让 server 从新 seed：

```bash
cd deployments/local
docker compose restart server
```

6. 通过 API 触发：

```bash
SID=$(curl -fsS -X POST http://127.0.0.1:50052/api/v1/sessions \
  -H 'Content-Type: application/json' \
  -d '{"domain_id":"my-domain","trigger":{"question":"test"}}' | jq -r .id)
curl -fsS http://127.0.0.1:50052/api/v1/sessions/$SID
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

```bash
cd /Users/Zhuanz/business_project/HnsX
make build-server
./scripts/smoke.sh
```

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

1. 获取 session_id 和失败状态：

```bash
curl -fsS http://127.0.0.1:50052/api/v1/sessions/$SID
```

2. 查询该 session 的 observations：

```bash
curl -fsS "http://127.0.0.1:50052/api/v1/traces?domain=customer-service" | jq
# 找到对应 trace_id，再查详情
curl -fsS http://127.0.0.1:50052/api/v1/traces/$TRACE_ID
```

3. 直接查 DB（如果 API 无数据）：

```bash
docker compose -f deployments/local/docker-compose.yaml exec postgres psql -U hnsx -d hnsx -c "
SELECT kind, agent_id, payload FROM observations
WHERE session_id='$SID' ORDER BY created_at;
"
```

4. 查看 Worker 日志：

```bash
docker compose logs worker --tail=50
```

5. 查看 Server 日志：

```bash
docker compose logs server --tail=50
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

*本 SOP 与 `README.md`、`CLAUDE.md`、roadmap 一起维护，产品能力变化时同步更新。*
