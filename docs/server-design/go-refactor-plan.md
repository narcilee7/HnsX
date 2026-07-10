# Go 服务改造设计方案（下一步）

> 状态：设计稿，待 review。作者：Claude Code。日期：2026-07-10。
> 关联：`docs/tech_overview.md`（Phase 划分）、`docs/server-design/api-design.md`（API 契约）、`proto/hnsx/v1/*.proto`（协议真相源）。

---

## 0. 背景与目标

对照文档 Phase 地图，Go 服务当前位置：

| 能力 | 文档 Phase | 现状 |
|---|---|---|
| Domain CRUD / Session Trigger / SSE / Trace 查询 | P1 | ✅ 完整 |
| Eval / Metric / Audit API | P2 | ⚠️ Metric/Audit 完整，**RunEval 是 neutral-score 骨架** |
| Approval / Runtime / Secret / Policy API | P3 | ❌ 全 stub |
| gRPC 全服务 + 认证 | P4 | ⚠️ 仅 Worker/Scheduler 两个服务实装 |

进程骨架、gin 路由、Postgres+goose、Repository 双实现、Worker 池+Redis、`single`/`workflow` runtime、SSE broadcaster 均已完整。因此本轮**不是补零功能，而是还三处结构性欠债**：

- **A. Trace API 归位** —— 数据层（`observations` 表 + `internal/trace` 聚合）已就绪，但 REST handler 走了 session 转换的捷径。
- **B. Eval 执行闭环** —— worker/Executor 已能跑 session，RunEval 却还在返 neutral score。
- **C. 协议栈收敛（Connect-RPC）** —— `v1connect/` 已生成零引用；gin 手写 handler 与 gRPC 定义正在各自漂移；CLI 名义走 gRPC 实际走 HTTP。
- **D. Phase 3 控制面** —— Secret / Policy / Approval / Runtime 四个 stub 变真。

**本轮范围**：A + B + C + D。质量债（handler 测试、死代码清理、tenant 解析）作为每个 Track 的伴随项，不单列。

**不做**：真实 LLM Adapter、MCP Client、非 None Sandbox、supervisor/autonomous 编排模式、多租户 SaaS —— 这些是 runtime/后续 Phase 的事。

---

## 1. 改造总览与顺序

```
里程碑 M1 ──► M2 ──► M3 ──► M4
   A         B       D       C
Trace归位   Eval闭环  P3控制面  协议收敛
(低风险)   (中风险)  (中风险)  (高风险,压轴)
```

排序理由：A/B 前置依赖已就绪、ROI 最高、直接兑现 vision 的「可观测 + 可评估」；D 是纯增量、彼此独立、可并行拆；C 是大改动高风险，放最后并单独开分支，前面几个 Track 先把内部 service 边界理清，正好为 C 抽出干净的「一份实现、两个传输」打好底。

---

## 2. Track A —— Trace API 归位（里程碑 M1）

### 现状问题
`pkg/api/auxiliary.go:18-72` 的 `ListTraces`/`GetTrace` 把 session 列表伪装成 trace，`trace_id == session_id`，`observations` 完全没进来。而 `internal/trace/repository` 已实装 `BySession/ByTrace/Aggregate` 且有测试，`GET /api/v1/sessions/:id/trace`（`sessions.go:245`）已经正确用上了它。也就是说**能力在，只是 traces 顶层 endpoint 没接**。

### 目标
`GET /api/v1/traces` 与 `GET /api/v1/traces/:traceId` 直接建在 trace 聚合上，返回契约对齐 `api-design.md §5`。

### 落点
1. `internal/trace/service`（若无 List 方法则新增）暴露：
   - `ListTraces(tenant, filter TraceFilter) ([]TraceSummary, int, error)` —— filter 支持 `domain / session / from / to / limit / offset`。
   - `GetTrace(tenant, traceID) (TraceDetail, error)` —— 返回 `trace_id + session_id + observations[]`。
   - Summary 用 repository 的 `Aggregate`（trace_id 聚合出 status/started_at/duration/cost）。
2. `internal/trace/repository/postgres.go` 补一个按 `created_at` 范围 + `domain_id` 过滤的 `List`（`observations` 表已有 `(session_id, created_at)`、`trace_id`、`kind`、`agent_id` 索引；若按 domain 过滤命中不了索引，加一条 `(domain_id, created_at)` 迁移 `000004`）。
3. `pkg/api/auxiliary.go`：`ListTraces`/`GetTrace` 改调 `s.TraceService`，删掉 session 转换分支。`GetTrace` 的 404 用 `TRACE_NOT_FOUND`。
4. `internal/app/application.go`：确认 `TraceService` 已注入 Server（`GetSessionTrace` 已依赖它，应已在）。

### 验收
- `GET /api/v1/traces?domain=x&from=&to=&limit=` 返回真实聚合，分页正确。
- `GET /api/v1/traces/:id` 返回 observations 数组。
- 新增 `pkg/api` handler 测试（用 InMemory trace repo）—— 顺带开 handler 测试的头。

### 风险
低。纯读路径，不动写入。唯一注意：`traceId` 语义从「= session_id」变成「真 trace_id」，需确认 Console 侧 `mappers.ts` 是否假设两者相等。**行动**：改动前 grep console 的 `trace_id`/`traceId` 用法，必要时同步调 ViewModel。

---

## 3. Track B —— Eval 执行闭环（里程碑 M2）

### 现状问题
`RunEval`（`auxiliary.go:239-292`）创建 `EvalRun` 后立即 `FinishRun(run.ID, 0.0, 0, total, 0, 0)`，注释明说「等 worker pipeline 能 batch 跑 session」。而 batch 能力现在**已经有了**：`triggerSession` 已能走 `SessionQueue`（worker 模式）或 `Executor`（本地模式）。

### 目标
RunEval 真正为每个 `EvalCase` 触发一次 session、收集输出、跑 scorer 打分、聚合成 EvalRun。对齐 `api-design.md §6.6/§6.7` 与 `tech_overview.md §7.3` 的 Eval 数据流。

### 设计：新增 `internal/evaluation/runner`
把执行逻辑从 handler 里拆出来，handler 只负责建 run + 起异步 runner（与 session trigger 的 `go runInBackground` 同构）。

```
EvalRunner.Run(ctx, run, set, domain):
  for case in set.Cases:                     # 顺序或有界并发(信号量, 默认 4)
    sess := triggerSessionForEval(domain, case.Input)   # 复用 SessionCommands.Trigger
    waitForTerminal(sess.ID)                 # 订阅 broadcaster 或轮询 session 状态
    output := sess.Result
    score := Scorer.Score(case, output)      # 见下
    accumulate(score, cost, passed)
  EvalService.FinishRun(run.ID, avgScore, passed, total, totalCost, durationMs)
  # 每个 case 落一条 eval_case_result(见迁移)
```

### Scorer（本轮只做规则类，够闭环）
`internal/evaluation/scorer`，接口 `Score(case EvalCase, actual any) (float64, bool, details)`。对齐 proto `Scorer{name,kind,weight,config}`：
- `exact` —— 精确匹配 `case.Expect`。
- `contains` —— 子串/包含。
- `json_match` —— 结构等价。
- `llm_judge` —— **本轮留接口 stub**（依赖真实 Adapter，不在范围内），注册但返 `ErrScorerNotAvailable`，多 scorer 按 `weight` 加权，遇不可用的降级跳过并在 details 标注。

### 落点
1. `internal/evaluation/runner/runner.go` 新增（含并发信号量、超时、取消）。
2. `internal/evaluation/scorer/{scorer.go,exact.go,contains.go,json.go}` 新增。
3. 迁移 `000005_eval_case_results`（若 `eval_cases`/`eval_case_results` 未覆盖 per-case 结果）：`eval_case_id / eval_run_id / session_id / score / passed / actual / details`。对齐 proto `EvalCaseResult`。
4. `EvalService` 补 `RecordCaseResult` + `GetRun` 带 cases。
5. `pkg/api/auxiliary.go:RunEval`：删 skeleton，改成建 run → `go runner.Run(...)` → 返 `202 {run_id, state:"running"}`。`GetEvalRun` 已能读回，补 `cases[]` 字段（对齐 §6.7）。
6. 复用而非新造：session 触发走现有 `SessionCommands.Trigger`；等终态复用 `broadcaster` 或 session repo 轮询。

### 关键决策点（需 review 确认）
- **执行位置**：Eval 的 case session 走 worker 队列 还是本地 Executor？建议**跟随部署形态**——有 `SessionQueue` 就入队（团队/生产），否则本地 Executor（本地开发）。与 `triggerSession` 保持同一策略，零新分支。
- **并发度**：默认 4，`Policy.budget_usd` 做全局熔断（跑超预算即停并标 run failed）。

### 风险
中。异步执行 + 状态回写有竞态；case session 失败要不要整 run failed（建议：单 case 失败记 0 分继续，run 只在超预算/取消时 failed）。**回滚**：runner 出问题时 handler 可 feature-flag 退回 skeleton（`HNSX_EVAL_RUNNER=off`）。

---

## 4. Track D —— Phase 3 控制面（里程碑 M3）

四个子项彼此独立，可并行拆票。共同模式：`internal/<x>/{model,repository,service}` 已有骨架的就填实，没有的补齐 Postgres+InMemory 双实现，handler 去掉 stub 分支。

### D1. Secret（`auxiliary.go:451-480`，现返 `ADAPTER_NOT_IMPLEMENTED`）
- 迁移 `000003` 已建 secret 子表；`internal/secret/{model,repository}` 骨架在，缺 service + 测试。
- **安全底线**：value 落库前加密（本轮至少 AES-GCM + 来自 `HNSX_SECRET_KEY` 的主密钥；`List`/`Get` **绝不回显明文**，只返 `id/created_at/masked_preview`）。
- 打通 `${secret:name}` 注入：runtime 解释 DomainSpec 时从 secret store 解引用（`internal/secret/model` 注释已提到占位解析）。**注意**：这一步触碰 runtime，范围内只做「store + API + 注入点接口」，不做 Adapter 侧真实使用。
- handler：`List/Create/Update/Delete` 接真 service，`Create` 返 201 不含 value。

### D2. Policy（`auxiliary.go:483`，现返空数组）
- `internal/policy` 聚合 + repository 已实装且有测试，`pkg/policy.Engine` 也在。缺的是 **CRUD handler + Domain 绑定**。
- 落点：`ListPolicies` 接 repo；补 `CreatePolicy` / `POST /domains/:id/policies`（绑定，对齐 §12.3）。绑定关系存 `domain_policies` 关联表（迁移 `000006`）。
- 运行时已有 `pkg/policy.Engine` 做预算/权限，绑定后 Executor 读生效 policy。

### D3. Runtime（`auxiliary.go:435`，现硬编码 fake `local-control-plane/phase1`）
- 真实数据源已存在：`internal/worker`（Registry + 心跳 + Postgres 持久化 + 60s GC）。
- 落点：`ListRuntimes`/`GetRuntime` 改读 `worker.Registry`/worker repo，映射 `WorkerInfo`→REST（对齐 §10 与 proto `WorkerInfo/ResourceCapacity/ResourceUsage/WorkerHealth`）。删硬编码。
- 这是四项里最省的——**只是把已有 worker 数据换个 REST 视图**。

### D4. Approval（`auxiliary.go:74-98`，现全 stub）
- 从零：`internal/approval/{model,repository,service}` 新增 + 迁移 `000007_approvals`（`id/session_id/domain_id/action/resource/status/comment/created_at/decided_at/decided_by`）。
- 与 Policy `require_human_approval` + runtime 联动：Executor 命中需审批的动作时**挂起 session + 建 approval 记录**，SSE 推 `event: approval_required`；`approve/reject` 后恢复/终止。**注意**：挂起/恢复触碰 runtime 执行流，是本 Track 最重的一项。若想控风险，可拆两步：先做 Approval 的 CRUD + 列表（Console 可见），runtime 挂起联动作为 M3.5 独立票。

### 风险
中。D1 加密密钥管理、D4 runtime 挂起是主要风险点。D2/D3 低。**建议顺序**：D3 → D2 → D1 → D4（由易到难，D4 的 runtime 联动可延后）。

---

## 5. Track C —— 协议栈收敛（Connect-RPC，里程碑 M4，压轴）

### 现状问题
- 文档：CLI/内部走 gRPC，Console/SDK 走 REST。
- 实际：CLI 的 `internal/client` 打 **HTTP** `http://127.0.0.1:50051`；gRPC 只有 Worker/Scheduler；其余 6 个 control_plane service 是 protoc `Unimplemented`；gin handler 与 proto 各写各的，字段映射靠手抄，正在漂移。
- `proto/gen/go/hnsx/v1/v1connect/` 已生成，**零引用**。

### 目标
用 **Connect-RPC** 做「一份 service 实现，同时讲 gRPC + gRPC-Web + HTTP/JSON」，消除 gin 手写 handler 与 gRPC 定义的双栈重复。`connectrpc.com/connect v1.18.1` 已在 go.mod。

### 收敛策略（务实，不推倒重来）
不追求一次替换所有 gin 路由。分两层：

1. **内部/控制面 RPC 走 Connect**：`DomainRegistry / SessionScheduler(GetSession,ListSessions) / RuntimeDiscovery / Telemetry / Eval` 用 Connect handler 实装，CLI `internal/client` 从手写 HTTP 换成 **Connect Go client**，让「CLI↔Control Plane 走 gRPC」名副其实。Worker/Scheduler 保持现有 grpc-go（bidi stream 更成熟，不动）。
2. **REST 对外保留 gin**：Console/SDK 面向的 `/api/v1/*` REST + SSE 保持不变——它们是外部契约，且 SSE 用 gin 更顺手。但 **handler 内部改为调用同一批 Connect service 实现**（而非各写一份逻辑），让 REST 退化成「协议适配壳」，业务逻辑唯一化。

即目标态：

```
              ┌───────────────────────────────┐
   Console/SDK│ gin REST + SSE (/api/v1/*)      │─┐
              └───────────────────────────────┘ │
                                                 ├─► 同一批 service 实现
   CLI/内部    ┌───────────────────────────────┐ │   (internal/*/service)
              │ Connect handler (gRPC+JSON)     │─┘
              └───────────────────────────────┘
   Worker     ┌───────────────────────────────┐
              │ grpc-go bidi (worker.proto)     │  ← 保持不动
              └───────────────────────────────┘
```

### 落点
1. `make proto` 确认 buf 生成 `v1connect`（已生成，验证 codegen 配置在案）。
2. `pkg/controlplane/` 或新 `internal/rpc/`：为 5 个 control_plane service 写 Connect handler，直接调 `internal/*/service`。
3. `internal/client`：手写 HTTP client → Connect Go client。CLI `remote` 子命令改造，输出契约不变。
4. `pkg/api/*`：handler 抽掉内联业务逻辑，改调 service（很多已经是这样，Track A/B/D 会进一步理清 service 边界——这也是把 C 放最后的原因）。
5. 删死代码：`server/`、`pkg/observation/`、`hnsx-core` 空子模块。

### 风险
高。这是唯一动到外部/CLI 契约的 Track。**强制要求**：
- 单开 `feat/connect-rpc` 分支，不与 A/B/D 混。
- CLI 输出做 golden 测试锁契约，改造前后 diff 为空。
- 分服务灰度：一次迁一个 service，`internal/client` 保留 HTTP fallback 到全部迁完再删。
- 若 review 认为 ROI 不足以承担风险，**C 可整体延后**——A/B/D 完成即已兑现本轮核心价值，C 是纯技术债偿还。

---

## 6. 里程碑与验收

| 里程碑 | Track | 交付 | 验收 |
|---|---|---|---|
| M1 | A | Trace API 归位 | `/api/v1/traces` 返真实聚合；新增 handler 测试；Console trace 页正常 |
| M2 | B | Eval 执行闭环 | `RunEval` 真跑 case，`GetEvalRun` 返 per-case 分数；规则 scorer 测试覆盖 |
| M3 | D | Phase 3 控制面 | Secret 加密不回显 / Policy CRUD+绑定 / Runtime 读真 worker / Approval CRUD | 
| M4 | C | 协议收敛 | CLI 走 Connect；业务逻辑单一化；死代码清零；CLI golden 测试 diff 空 |

每个里程碑收尾必跑：`cd hnsx-server && go build ./... && go test ./...`；触碰 proto 的跑 `make proto`；触碰 Console 契约的到 `hnsx-console` 跑 `pnpm type-check`。

## 7. 伴随质量债（穿插，不单列里程碑）
- 每个 Track 给新增/改动的 handler 补测试（`pkg/api` 当前近零覆盖）。
- M1 顺带清 `server/`、`pkg/observation/` 空目录（M4 统一删避免冲突则延后）。
- tenant 解析：中间件已读 `X-Tenant-ID`，但无 token→tenant 映射；本轮先保持 single-tenant `DefaultID`，仅在文档标注为 P4 认证的前置。
- `ListDomainVersions`（`domains.go:123` 硬编码单版本）接 `domain_versions` 表——可并入 M1 或独立小票。

## 8. 开放问题（待 review 拍板）
1. **C 做不做**：Connect 收敛 ROI vs 风险，是否本轮纳入，还是仅完成 A/B/D。
2. **Eval 执行位置**：跟随部署形态（有队列入队，无则本地）是否认可。
3. **Approval runtime 联动**：M3 一次做完，还是拆「CRUD 先行 + 挂起联动延后」。
4. **Secret 主密钥来源**：本轮 `HNSX_SECRET_KEY` 环境变量够用，还是要接 KMS/外部保险箱接口。
5. **trace_id 语义变更**对 Console `mappers.ts` 的影响面确认。
