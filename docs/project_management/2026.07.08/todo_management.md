# TODO Management

> 周期：2026.07.08（本周）<br>
> 北极星：跑通 `CLI → Go Control Plane → Python Worker → Noop Adapter → Observation` 的最小端到端链路。

---

## 本周目标

1. **后端 Worker**：完成 Python worker parent 进程、SessionRuntime 子进程和 Noop adapter，能独立跑通一次完整 Session。
2. **Go 服务调度**：完成 WorkerRegistry / Scheduler 与 Python worker 的 gRPC 对接，支持心跳、任务拉取和双向流取消。
3. **前端工程化**：补齐控制台与后端 API 的对接，重点把 Domain 列表、Session 触发、SSE Observation 实时流页面串起来。

---

## 1. 后端 Worker（hnsx-worker）

### 1.1 Worker parent 进程
- [x] 实现 `hnsx-worker run --server=... --worker-id=...` CLI 入口
- [x] 通过 gRPC `PullSession` 长轮询从 Go Scheduler 拉取待执行 Session
- [x] 每 5s 发送 `Heartbeat`，上报 worker 状态与当前负载
- [x] 维护 `StreamChannel` 双向流，上传 Observation / 接收 Cancel

### 1.2 SessionRuntime 子进程
- [x] 用 `subprocess.Popen` 隔离每个 Session 执行
- [x] 子进程内加载 DomainSpec 并校验 Harness 配置
- [x] 子进程退出时父进程回收资源，异常退出时上报 error observation
- [x] 收到 Cancel 信号时向子进程发送 SIGTERM，支持优雅退出

### 1.3 Noop adapter
- [x] 实现 `NoopAdapter`，直接返回固定文本 observation
- [x] 跑通 `trigger → session → turn → observation → end` 的完整状态机
- [x] 把 observation 序列通过 `StreamChannel` 实时回传 Go Control Plane

### 1.4 测试与验收
- [x] `make worker-test` 全部通过
- [x] 端到端覆盖：拉任务 → fork runtime → noop adapter → 回传 3 条以上 observation（见 `scripts/smoke-worker.sh`）

---

## 2. Go 服务调度（hnsx-server）

### 2.1 Worker Registry
- [x] 维护内存中的 worker 列表（worker-id、capabilities、last heartbeat、current session）
- [x] 心跳超时通过 `EvictStale` 自动清理离线 worker
- [x] 提供 `Register` / `Heartbeat` / `List` / `Get` / `Inbound` 接口
- [x] 新增 `AssignSession` / `SessionWorker` / `UnassignSession`，维护 session → worker 映射

### 2.2 Scheduler
- [x] Session 触发后 `Enqueue` 到 `SessionQueue`
- [x] Worker 通过 `PullSession` 长轮询拉取任务
- [ ] Worker 离线时自动重新调度（当前行为：等长轮询 30s 超时后重新入队；需 queue 感知 worker liveness）

### 2.3 gRPC 服务对接
- [x] 补全 `proto/hnsx/v1/worker.proto` 的 Go service 实现：
  - `PullSession`（long-poll）
  - `Heartbeat`（unary）
  - `StreamChannel`（bidirectional streaming）
- [x] 与 Python worker 完成真实 gRPC 握手并跑通全链路

### 2.4 Cancel 传播
- [x] Console / CLI 调用 Cancel Session 时，通过 `Registry.SessionWorker` 找到对应 worker
- [x] 通过 `StreamChannel` 下发 `CancelSessionCommand`
- [x] Python worker 收到后终止子进程并回传 `cancelled` observation

### 2.5 测试与验收
- [x] `go test ./pkg/worker/... ./pkg/controlplane/... ./pkg/api/...` 通过
- [x] 端到端 smoke：`scripts/smoke-worker.sh` 验证注册 → 拉任务 → 回传 observation → cancel

---

## 3. 前端工程化（hnsx-console）

### 3.1 API 层补齐
- [x] 确认并统一 `src/api/client.ts` 的 base URL / 错误处理 / JSON 解析
- [x] 补齐 Domain 相关 API：list、get、create、update、validate
- [x] 补齐 Session 相关 API：trigger、get、list、cancel
- [x] 补齐 SSE 实时 Observation 消费：`GET /api/v1/sessions/:id/events`

### 3.2 页面与组件
- [x] `DomainsPage`：Domain 列表 + 新建入口
- [x] `DomainDetailPage`：YAML/JSON 编辑器（Monaco）+ 版本切换 + 保存/校验
- [x] `DomainRunPage`：选择 Domain、填写 trigger、触发 Session
- [x] `SessionDetailPage`：SSE 接收 observation，用 `ObservationTimeline` 实时渲染
- [x] 错误/加载态统一走 `src/components/ui/Error.tsx`、`Loading.tsx`、`Empty.tsx`

### 3.3 工程化规范
- [x] `pnpm type-check` 通过
- [x] `pnpm build` 通过
- [ ] `pnpm lint` / `pnpm test` / `pnpm dead-deps` 尚未跑（本周重点在端到端链路）
- [x] 主题/颜色统一走 CSS 变量（`--chart-1..5`、`--success/warning/danger/info`），不硬编码色值

### 3.4 与 SDK / observability workspace 联动
- [x] 确认 `@hnsx/sdk-node` 提供的类型与 API 返回一致
- [x] 确认 `@hnsx/observability` 组件在控制台能正常渲染 trace/metric 数据

### 3.5 验收
- [x] `pnpm build` 成功产出 dist
- [x] 控制台能：列出 Domain → 编辑保存 → 触发 Session → 实时看到 Observation 流

---

## 跨线集成验收（本周终极目标）

- [x] 启动 Go server
- [x] 启动 Python worker 并注册成功
- [x] 控制台（或 CLI）触发一个 Session
- [x] Go Scheduler 把任务分发给 Python worker
- [x] Python worker fork SessionRuntime，Noop adapter 产生 observation
- [x] observation 经 gRPC 回传到 Go server，再经 SSE 推送到控制台展示
- [x] 整个链路的 trace / cost / audit 有记录（trace 通过 session id 关联；cost 由 noop 产生零值记录）

---

## 遗留问题 / 下周入口

- **Worker 异常断开时的 queue waiter**：如果 worker 在 `PullSession` 长轮询中异常断开，它占用的 waiter 会让新 session 延迟到 30s 超时后才重新可见。需要让 `SessionQueue.Dequeue` 感知 worker liveness 或支持主动取消 waiter。
- **前端 lint / test / dead-deps**：下周补齐并加入 CI。
- **真实 Anthropic / OpenAI adapter**：V1.1 Step 3，接入真实 LLM provider。
- **Trace / cost / audit 持久化**：当前 observation 只走内存 broadcaster，DB 持久化未接入。

---

## 不在这周做的事

- 真实 Anthropic / OpenAI adapter 调用（Step 3，下周）
- Tools / MCP / Sandbox / Memory 完整实现（Step 5）
- Supervisor / workflow / autonomous 编排（Step 6）
- Helm / K8s / 多租户 SaaS
- 复杂 Workflow DAG 可视化编辑器
