# HnsX 端到端落地 Roadmap

> 日期：2026-07-10
> 周期：迭代至 P0 全部清零；P1/P2 待 P0 e2e 串通后再排
> 北极星：[`docs/vision.md`](../../vision.md) — Harness as a Service
> 当前分支：`feat/connection_e2e`
> 协议真相源：[`proto/hnsx/v1/`](../../../proto/hnsx/v1/)

---

## 0. 工作方式

- 每个 T 票 = 一个独立分支 + 一个常规 commit。commit message 用 Conventional Commits，并在 commit body 引用本文件锚点（如 `Refs: docs/project_management/2026.07.10/end-to-end-product-roadmap.md#t1`）。
- 每完成一个 T：在本文件**勾掉对应 `- [ ]`**（勾成 `- [x]`），该勾选与代码 commit **一起**入库，让"勾选即落库"成为强约束。
- 全栈纵切：每个 T 都同时改 server + console（如适用）+ 协议，**不留 server 改完再改 console 的中间态**。
- 验收：每个 T 自带 `## 验收` 段，PR 模板强制对照勾选。

---

## 1. 家底盘点（这一轮开始时的实情）

| 层 | 当前 | 评级 |
|---|---|---|
| **文档** | vision / tech_overview / tech_v1_1 / api-design / tablebase / know-how × 4 / service-architecture / go-refactor-plan / 4 份 PM（w9 / w10+ / go-service-arch-governance / v1_2_roadmap）齐 | ⭐⭐⭐ 完整 |
| **proto** | `domain.proto` / `control_plane.proto` / `runtime.proto`（deprecated）/ `observation.proto` / `worker.proto` 全有；Go + TS 已生成；`v1connect/` 已生成但零引用 | ⭐⭐⭐ 完整但漂移 |
| **Go Control Plane** | DDD 骨架齐：11 个 `internal/` bounded context + Postgres repo + service；sinks + executor + WorkerRegistry + SessionQueue 已实装 | ⭐⭐ 骨架完整 |
| **Python Worker** | adapters / tools / policy / approval / sandbox / skills / memory / harness / runner 全齐；W0–W16 全部 ✅；是全栈完成度最高的一块 | ⭐⭐⭐ 完整 |
| **Console** | 17 个 page + hooks × 7 + api client × 8 + mappers 全在 | ⭐⭐ 大部分齐 |
| **observability** | 7 chart + integration lib + tokens + playground 全在 | ⭐⭐⭐ 完整 |
| **SDK** | node（完整）/ go（go.mod+README 无码）/ python（空目录）| ⭐ 严重不平衡 |
| **部署** | `deployments/local/docker-compose.yml` 是否默认起 tempo+grafana+worker 待验 | ⭐ 待验 |
| **e2e 串联** | customer-service / claude-triage / financial-analysis / skills-demo / mcp-demo 等 9 个 example domain 存在，但**没人跑过完整 trigger → server → worker → SSE → console** 链路 | 0 — 这一轮要打通 |

---

## 2. Gap 清单（按兑现 vision 与联调阻塞度排序）

### 🔴 P0 — 联调阻塞，没法 demo 任何真实域

| # | 位置 | 现状 |
|---|---|---|
| **G1** | `pkg/api/auxiliary.go:24-152` | `ListTraces/GetTrace` 用 `trace_id == session_id` 把 session 列表伪装成 trace；`internal/trace/service` 真聚合已有，但被绕开 |
| **G2** | `pkg/api/auxiliary.go:574-604` | `ListSecrets` 回空数组；`CreateSecret/UpdateSecret/DeleteSecret` 全部 `ADAPTER_NOT_IMPLEMENTED`；message 写 "target: Phase 2" |
| **G3** | `pkg/api/auxiliary.go:558-572` | `ListRuntimes` 写死 fake `local-control-plane/phase1`；worker 已能 Register + Heartbeat 但 Settings → Runtime 看不到 |
| **G4** | `pkg/api/auxiliary.go:606-612` + `router.go` | `ListPolicies` 返空；`POST /policies` / `POST /domains/:id/policies` 路由表根本没绑 |
| **G5** | `pkg/api/auxiliary.go:156-180` | `ListApprovals/ApproveApproval/RejectApproval` 全空数组 + stub；approval 表不存在 |
| **G6** | `src/api/` | 缺 `evals.ts`（hooks `useEvals.ts` 在但 client 缺失）|
| **G7** | `mappers.ts:187/197/224` | 三处 traceId 拼装依赖旧 `trace_id == session_id` 语义 |

### 🟡 P1 — 契约对齐（不影响 demo，但跨模块会漂）

| # | 位置 | 现状 |
|---|---|---|
| **G8** | `router.go` vs `api-design.md §3.4 / §6.1 / §12.3` | 缺 `PUT /domains/:id` / `GET /domains/:id/evals` / `POST /domains/:id/policies` 等路由 |
| **G9** | `proto/Observation.{payload,metadata}` | proto 是 string，Python 塞 dict；缺统一序列化约定 |
| **G10** | `proto/MCPConfig`（Go）↔ Python `mcp_servers{name/transport/url/command}` | 字段名不一致；两端各写各的 |
| **G11** | `WorkflowSession.step.prompt_ref` | Python runner 当前不支持 workflow mode，stub 起来不是真跑 |
| **G12** | proto | 大量 message 标 deprecated（`RuntimeService`/`ScheduleSession` 等已迁移到 `worker.proto`，但 proto 文件仍保留 + client 误引用） |

### 🟢 P2 — 兑现承诺 / 技术债

| # | 位置 | 现状 |
|---|---|---|
| **G13** | `v1connect/` | 已生成、零引用；6 个 service 是 `Unimplemented`；CLI 实际走 HTTP 而非 gRPC |
| **G14** | `cmd/hnsx` | 远程命令（`domains/sessions/eval`）尚未实现 |
| **G15** | `sdk/go` / `sdk/python` | 仅 README + 空目录，仅 node 完整 |
| **G16** | `main.go` | `HNSX_OTEL_EXPORTER` 默认空 → Otel 默认不接 Tempo |
| **G17** | 多租户 | `X-Tenant-ID` 中间件在，`auth_token → tenant` 映射未做，所有表已带 `tenant_id` 默认 `default` |

---

## 3. Tickets（P0 = 立刻做；P1 = P0 跑完 e2e 后排；P2 = 之后单独迭代）

### P0 — 全栈纵切，8 张票

每张票 = 1 分支 + 1 commit + 1 勾选。

#### T1 — Trace API 归位（server + console）

- [ ] **server** `pkg/api/auxiliary.go:24-152` 改调 `internal/trace/service.ListTraces/GetTrace`，删除 `trace_id == session_id` 假象
- [ ] **server** `internal/trace/service` 暴露 `ListTraces(tenant, filter)` + `GetTrace(tenant, traceID)`；如缺失则补
- [ ] **server** handler 测试：trace_id 不存在返 404 `TRACE_NOT_FOUND`；列出分页正确
- [ ] **migration** 视情况加 `observations(domain_id, created_at)` 复合索引
- [ ] **console** `src/api/mappers.ts:187/197/224` 与 `useTraces/SessionDetailPage/TraceDetailPage` 链路同步
- [ ] **验收**：`GET /api/v1/traces?domain=x&from=&to=&limit=` 返回真实聚合；`GET /api/v1/traces/:id` 返回 observations[]；console TracesPage 端到端走通

Refs：`docs/server-design/go-refactor-plan.md §2 Track A`

#### T2 — Secret CRUD 闭 loop（server）

- [ ] **server** `auxiliary.go:574-604` 全部接 `secret.Service`；`List/Get` 只返 `id+created_at+masked_preview`，绝不回显 value
- [ ] **server** value 落库前 AES-GCM 加密，key 来自 `HNSX_SECRET_KEY` env
- [ ] **server** `Create` 返 201 不含 value
- [ ] **console** `src/api/settings.ts` + `SettingsPage` Secret tab 端到端
- [ ] **验收**：CLI / Console 创建后能 list 但看不到 value；任意 domain 用 `{secret.XXX}` 能注入（不在本票范围，做到接口齐全即可）

Refs：`docs/server-design/go-refactor-plan.md §4 D1`

#### T3 — Runtime 列表读真 worker.Registry（server）

- [ ] **server** `auxiliary.go:558-572` 删硬编码，改读 `worker.Registry/worker repo`
- [ ] **server** `WorkerInfo → REST` 映射对齐 `api-design §10` 与 `proto/RuntimeInfo`
- [ ] **console** `SettingsPage` Runtime tab 与 `useSettings` 同步展示多 worker
- [ ] **验收**：启动 2 个 worker 后 Settings 看到 2 行；刷新有 last_heartbeat_at 滚动

Refs：`docs/server-design/go-refactor-plan.md §4 D3`

#### T4 — Policy CRUD + Domain 绑定（server）

- [ ] **server** `ListPolicies` 接 repo；新增 `CreatePolicy` / `POST /policies`
- [ ] **server** 新增 `POST /domains/:id/policies` 绑定
- [ ] **migration** `000006_domain_policies` 关联表
- [ ] **router** 加对应路由
- [ ] **console** `SettingsPage` Policy tab；Domain 详情加 "绑定 Policy" 操作
- [ ] **验收**：建一条 policy + 绑到 domain，Executor 执行能看到 `pkg/policy.Engine` 生效

Refs：`docs/server-design/go-refactor-plan.md §4 D2`

#### T5 — Approval 一次到位（server + worker）

- [ ] **server** `internal/approval/{model,repository,service}` 新增
- [ ] **migration** `000007_approvals`
- [ ] **server** `ListApprovals/Approve/Reject` 接 service + 路由
- [ ] **server** Executor 命中 `policy.require_human_approval` 挂起 session 并通过 SSE 推 `event: approval_required`
- [ ] **server** approve/reject 后通过 gRPC `StreamChannel` 推 `ResumeSession/CancelSession` 到 worker；W14 Python `approval/bus.py` 配套
- [ ] **console** `ApprovalsPage` + `SessionDetailPage` 的 Approve/Reject 操作端到端
- [ ] **验收**：example domain `customer-service` 配 `require_human_approval: true` 触发退款后，control plane 收到 approval_required，console 审批后 session 续跑 / 终止

Refs：`docs/project_management/2026.07.10/python-worker-w10-plus.md §7`

#### T6 — Console 补 evals.ts api client（console）

- [ ] **console** 新建 `src/api/evals.ts` 对齐 `EvalSet/EvalCase/EvalRunResult` proto；`listEvalSets/getEvalSet/createEvalSet/runEval/getEvalRun`
- [ ] **console** `useEvals.ts` + `EvalsPage` + `EvalSetPage` + `EvalRunPage` 端到端
- [ ] **验收**：从 console 创建一个 EvalSet + 触发 Run，能看到真实分数与 cases

Refs：`docs/server-design/api-design.md §6`

#### T7 — Console traceId 语义同步（console，与 T1 联动）

> 阻塞 T1。

- [ ] **console** `mappers.ts:187/197/224` 与新 trace_id 语义对齐
- [ ] **console** `SessionDetailPage` `/traces/:id` Link、`TracesPage/TraceDetailPage` 渲染
- [ ] **验收**：T1 改完后 console 不出现 stale `traceId`，全部走真 trace_id

#### T8 — e2e：docker compose 跑通 customer-service 真实域（infra + 验证）

> 阻塞 T1–T7。

- [ ] **deploy** `deployments/local/docker-compose.yml` 默认起 server + postgres + worker + tempo + grafana，OTel exporter 默认 OTLP
- [ ] **e2e 脚本** `scripts/e2e/customer-service.sh`：validate → POST domain → POST session → SSE consume → 落 trace → console 反查
- [ ] **验收**：执行后输出 trace_id + obs 序列 + 一张 console 截图占位（mvp 不要图）

---

### P1 — 契约对齐（P0 e2e 跑通后启动）

- [ ] **T9** 补齐 `api-design.md` 缺失路由（`PUT /domains/:id`、`/evals` 嵌套路径等）
- [ ] **T10** `Observation.{payload,metadata}` 序列化约定写进 proto / Python loader / TS mappers
- [ ] **T11** `MCPConfig` ↔ Worker `mcp_servers` 字段映射 + 双向转换函数
- [ ] **T12** `WorkflowSession.mode` Python runner 至少跑通一个 example（`workflow-demo`）

### P2 — 兑现承诺（独立迭代排期）

- [ ] **T13** Connect-RPC 收敛（v1connect 上线 + 5 个 control_plane service 实装 + CLI 改走 Connect）
- [ ] **T14** `cmd/hnsx` 远程命令（`domains/sessions/eval` 通过 `internal/client`）
- [ ] **T15** `sdk/go` / `sdk/python` 真出包（Go 用 `internal/client` 封装；Python 用 `hnsx_worker/proto_client` 拆 SDK）
- [ ] **T16** OTLP 默认开启 + Tempo + 5 张 Grafana dashboard JSON
- [ ] **T17** 多租户 `auth_token → tenant` + 行级 RLS 预留

---

## 4. 跨 Track 跟随项（不在单独 T 票里但要做到）

- [ ] 每个 T 必跑 `cd hnsx-server && go build ./... && go test ./...`
- [ ] 动 proto 的跑 `make proto` 后再动
- [ ] 动 Console 的到 `hnsx-console` 跑 `pnpm install --force && pnpm type-check`
- [ ] 动 worker 的跑 `make worker-test`
- [ ] 每个 T 的 PR 末尾带：「🤖 Generated with [Claude Code]」+ 引用本文件锚点

---

## 5. 风险与守则

| 风险 | 守则 |
|---|---|
| Server 改完忘同步 console，T8 e2e 必现 stale | 全栈纵切强约束：单 T 不允许 server 一侧先 merge |
| Trace 索引 / CTE 写复杂，首页 P95 退步 | 复用 `traceSvc.Aggregate` + (session_id, created_at) 已有索引；新加索引必须 EXPALIN 过 |
| Approval 一次到位风险高（gRPC 下行 + 会话挂起状态机） | 先在 `customer-service` 跑通单域，再扩到通用 |
| Secret 加密 key 走 env，本地开发易忘 | `internal/config` 启动 fail-fast：未设 `HNSX_SECRET_KEY` → 拒绝起来 |
| Worker 与 Server 时间漂导致 SSE 顺序错乱 | 所有 obs/ts 用 server-time（worker 上报时带 `client_ts_ms`，server 校准后写入）|

---

## 6. 收尾协议

每个 T 落地完成后：

1. 在本文件勾掉 `- [ ]`（commit 携带此变化）
2. commit message：`feat(server|console|infra|worker): <t-id> <短描述>\n\nRefs: docs/project_management/2026.07.10/end-to-end-product-roadmap.md#<t-id>`
3. 推到远端，开 PR
4. 不直接合 main —— 等 review

P0 全部勾完后，启动 P1 排期，**清零即更新本文件 §0 周期字段 + 进度条**。

---

*owner：HnsX squad · last_updated：2026-07-10*
*Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>*
