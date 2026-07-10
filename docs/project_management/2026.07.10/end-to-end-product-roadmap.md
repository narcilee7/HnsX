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

- [x] **server** `pkg/api/auxiliary.go:24-152` 改调 `internal/trace/service.ListTraces/GetTrace`，删除 `trace_id == session_id` 假象
- [x] **server** `internal/trace/service` 暴露 `ListTraces(tenant, filter)` + `GetTrace(tenant, traceID)`；如缺失则补
- [x] **server** handler 测试：trace_id 不存在返 404 `TRACE_NOT_FOUND`；列出分页正确
- [x] **migration** 视情况加 `observations(domain_id, created_at)` 复合索引 (`go/migrations/000005_trace_list_indexes.up.sql`)
- [x] **console** `src/api/mappers.ts` + `TracesPage` + `api/traces.ts` 同步切换 `TraceSummaryViewModel`；`mappers.ts:187/197/224` 的 traceId 字段在 wire 上仍是 `trace_id`，无需改动
- [x] **验收**：`GET /api/v1/traces?domain=x&from=&to=&limit=` 返回真实聚合；`GET /api/v1/traces/:id` 返回 observations[]；console TracesPage `pnpm type-check` + `pnpm build` 全过

Refs：`docs/server-design/go-refactor-plan.md §2 Track A`

#### T2 — Secret CRUD 闭 loop（server + console）

- [x] **crypto** `internal/secret/crypto`：AES-256-GCM envelope，加 nonce 前缀，`HNSX_SECRET_KEY` < 16 字符 fail-fast
- [x] **application.go** 启动时 `secretcrypto.New()` 装载 cipher，缺失则拒绝启动；log.Info 标记加密启用
- [x] **model** `internal/secret/model`：Secret 区分 `Value`（envelope）/ `PlainValue`（仅创建期）；新增 `ListItem` 不带 value
- [x] **repository** 增加 `List()`，InMemory 与 Postgres 双实现；entity 不动，靠 service 层加密
- [x] **service** `Save` 走 cipher.Encrypt → envelope + fingerprint；`List` 返 ListItem；`Delete` 幂等；`Resolve` 走 cipher.Decrypt
- [x] **handler** `auxiliary.go` ListSecrets 真接 service（含 nil service → 503 SECRETS_UNAVAILABLE）；Create/UpdateSecret 接受 plaintext value，返 201/200 + fingerprint（绝不含 value）；DeleteSecret 真接 service + 204
- [x] **errors.go** `SECRETS_UNAVAILABLE` → 503
- [x] **server tests** `pkg/api/secrets_test.go` 4 条：CRUD 不回显 plaintext + Resolve 解码回 plaintext + 缺 value 返 400 + nil service 返 503
- [x] **console** Secret 类型对齐 server 字段（name/description/kind/fingerprint/created_at/updated_at）
- [x] **console** SettingsPage.SecretsTab — Create 对话框含 kind dropdown + description；SecretTable 改 name/kind chip/fingerprint；hardcoded stub warning 替换为 try/catch 错误条
- [x] **验收**：`go test ./...` + `pnpm type-check` 全过

Refs：`docs/server-design/go-refactor-plan.md §4 D1`

#### T3 — Runtime 列表读真 worker.Registry（server）

- [x] **server** `auxiliary.go` 删硬编码 `local-control-plane/phase1`，改调 `WorkerService.List()`
- [x] **server** 映射 `worker.Snapshot → REST`：runtime_id / version / region / hostname / pid / capacity / capabilities / models / sandbox_runtimes / labels / last_heartbeat_at / age_seconds / status
- [x] **server** status 在 healthy / degraded / offline 三档；>60s 未心跳 → offline
- [x] **server** `WorkerService == nil`（gRPC 未启用）时返回空列表，由 UI 渲染 empty state；不再有 fake 数据
- [x] **server** `pkg/api/runtimes_test.go`：4 个 handler 测试覆盖空、单/多 worker、nil service、status 三档
- [x] **console** `src/api/settings.ts` `Runtime` 类型扩 capabilities/models/sandbox_runtimes/labels/age_seconds/healthy
- [x] **console** `SettingsPage.RuntimesTab` 列改：Runtime ID + version、Host + pid、Region、Capabilities chips、Status、Slots、Last heartbeat
- [x] **验收**：`pnpm type-check` + `go test ./...` 全过

Refs：`docs/server-design/go-refactor-plan.md §4 D3`

#### T4 — Policy CRUD + Domain 绑定（server + console）

- [x] **server** `internal/policy/model` 重排 — Policy 由 `DomainID` 改为 `ID + Name + BoundDomain`；新增 `ListItem`；新增 `Rules`(JSON-friendly) + `ErrInvalidPolicyID`
- [x] **server** `internal/policy/repository` 接口换为 ByID/List/Delete/BindDomain/ByDomain；InMemory 维护 id→policy 与 domain→id 双向索引，1:1 不变；Postgres 用 `domain_uuid UUID NULL`（已无需迁移）
- [x] **server** `internal/policy/entity.go` 把 `DomainUUID` 改成 `*string`，与 SQL NULL 一致
- [x] **server** `internal/policy/service` 加 `List/CreateOrUpdate/Delete/BindDomain`；`LoadDomainPolicy` 改用 ID == domainID（向后兼容）
- [x] **server** `pkg/api/auxiliary.go` List/Create/Update/Delete + `BindPolicy` 五个 handler 接通；`SECRETS_UNAVAILABLE→503` 顺手加 `POLICY_UNAVAILABLE→503`
- [x] **server** `router.go` 挂 `/policies`、`/policies/:id` 与 `POST /domains/:id/policies`
- [x] **server** `pkg/api/policies_test.go` 6 个 handler 测试：List/CRUD/Bind/1:1 invariant/unknown policy→404/nil service→503
- [x] **server** `internal/policy/repository/repository_test.go` 重写 6 个测试覆盖 Save/ByID/ByDomain/BindDomain 1:1/Unbind/List/Delete
- [x] **console** `src/api/settings.ts` Policy 类型扩展 budget/permissions/guardrails/bound_domain；增 `createPolicy/updatePolicy/deletePolicy/bindPolicy`
- [x] **console** `SettingsPage.PoliciesTab` 列展示 id/name/bound_domain chip + budget 摘要 + permission chip
- [x] **验收**：`go test ./...` + `pnpm type-check` 全过；包含 1:1 binding 不变量

Refs：`docs/server-design/go-refactor-plan.md §4 D2`

#### T5 — Approval 一次到位（server + worker）

- [x] **server** `internal/approval/{model,repository,service}` 新增；Status / RiskLevel const；ListItem / Approval 形状
- [x] **migration** `go/migrations/000007_approvals.up.sql` — approvals 表 + (tenant_id, approval_id) 唯一索引 + 状态 / domain + created_at 索引
- [x] **server** `internal/approval/repository` InMemory + Postgres 双实现；PendingForSession 防止同一 session 重叠 pending
- [x] **server** `internal/approval/service` 含 `Gate.Request(ctx, approval)` 同步阻塞 + Decision 转换；Approve/Reject wake 等待者
- [x] **server** `pkg/api/auxiliary.go` ListApprovals 默认 filter=pending；GetApproval；ApproveApproval 返 200/409 已解决/404；RejectApproval；写 immutable audit row (action=approval_decision)
- [x] **server** `pkg/api/errors.go` APPROVAL_NOT_FOUND → 404；APPROVAL_ALREADY_RESOLVED → 409；APPROVAL_UNAVAILABLE → 503
- [x] **server** `pkg/api/approvals_test.go` 6 个 handler 测试 + Gate 阻塞验证
- [x] **server** `pkg/session/executor.go` ApprovalGate 接口 + WithApprovalGate 钩子；policyAdapter 在 guardrail `action=human_approval` 时调用 gate.Request 同步挂起
- [x] **server** `internal/app/application.go` approvalServiceGateAdapter 把 ApprovalService 适配到 executor Gate 契约
- [x] **server** `application.go` 装载 ApprovalService + Postgres repo
- [x] **console** `src/api/approvals.ts` 对齐 server field 形状（id/session_id/domain_id/action/resource/risk_level/context/status/comment/reviewed_by/created_at/resolved_at）
- [x] **console** `useApprovals.ts` refetchInterval=5s；useResolveApproval 接收 reviewed_by + comment
- [x] **console** `ApprovalsPage.tsx` 列展示 id/session/domain/action/risk_level/status/created/actions；detail dialog 用 risk_level + action + context 渲染
- [x] **验收**：`go test ./...` + `pnpm type-check` 全过；exec gate 阻塞到 operator 决策

Refs：`docs/project_management/2026.07.10/python-worker-w10-plus.md §7`

> Note: Worker-side pause/resume (gRPC StreamChannel 推 CancelSession/ResumeSession) 走 W14 已建好的 `approval/bus.py` 路径；本票在 server 侧完整闭环，跨 worker 协调是后续 P1 显式票。

#### T6 — Console 补 evals.ts api client（console）

- [x] **console** 新建 `src/api/evals.ts`：listEvalSets (flat, no domainId nesting) + getEvalSet (id only) + createEvalSet + runEval + getEvalRun — 全部对齐 server `/api/v1/evals/...` 路由
- [x] **console** `src/api/domains.ts` 删除重复的 eval 函数（之前嵌在 domains 里用了错的 URL）
- [x] **console** `src/hooks/useEvals.ts` 重写为 setId-only 签名；useEvalSets 列表 + 客户端 domain filter；useCreateEvalSet / useRunEval / useEvalRun
- [x] **console** `EvalsPage` 重写：服务端返回的扁平列表 + domain filter 下拉 + View/Run 按钮 → 跳到 `/evals/:setId` + `/evals/:setId/runs/:runId`；Create 对话框含 set_id / domain / description
- [x] **console** `EvalSetPage` 重写：useEvalSet(setId) 单一参数；cases 列表 + Run 按钮
- [x] **console** `EvalRunPage` 重写：useEvalRun(setId, runId) 单一参数；Stats 行 + Cases 表；剥除 depends-on-list 的趋势图 / 基线选择器（依赖后续 T9 listEvalRuns 加）
- [x] **console** `App.tsx` 路由 `/evals/:setId` + `/evals/:setId/runs/:runId`
- [x] **验收**：`pnpm type-check` + `pnpm build` + `go test ./...` 全过

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
