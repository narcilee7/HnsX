# Python Worker W9 规划：Eval 闭环 + 可观测 + 生产沙箱

> 日期：2026.07.10
> 负责人：Python Worker 迭代线
> 依赖：W0–W8 已全部完成（`hnsx-worker` 已具备 adapter、tool registry、policy、sandbox/store、resilience、eval scorer）
> 北极星：`docs/vision.md` — Harness as a Service，让企业安全、可控、可评估地驾驭最强 Agent。

---

## 1. 目标

W8 实现了基础 Eval scorer 和回传链路。W9 要让 Python worker 从“能跑评测”进化到：

1. **Eval 可闭环**：真实 LLM Judge、批量 EvalSet、Baseline 对比、Eval 报告生成。
2. **运行可观测**：Observation → OTLP Trace/Metric、成本聚合、结构化日志。
3. **执行可进生产**：Container/Process Sandbox 真正隔离外部代码。
4. **Worker 可运维**：Observation 批量化、背压降级、优雅关闭 flush。

**验收底线**：一个 10-case EvalSet 能自动跑完、LLM Judge 给出理由、Grafana 可见 Trace、危险 Python tool 被 Sandbox 拦截。

---

## 2. 任务分解

### 2.1 Eval 平台化

| # | 任务 | 关键文件 | 验收标准 |
|---|---|---|---|
| 2.1.1 | 真实 LLM Judge 实现 | `hnsx_worker/eval/scorers.py` | `llm_judge` 调用配置的 adapter，解析 verdict（`pass/partial/fail`）与理由，不再用 keyword fallback。 |
| 2.1.2 | Judge 配置化 | `hnsx_worker/eval/scorers.py` | 支持 `judge_model`、`rubric`、`temperature`、`max_tokens`；可指定投票 judge。 |
| 2.1.3 | EvalSet 批量运行 | `hnsx_worker/eval/runner.py` | 接收 `eval_set`（多个 case），串行或按并发度执行，输出每个 case 的分数与原始输出。 |
| 2.1.4 | Baseline 对比 | `hnsx_worker/eval/report.py`（新建） | 对比本次 run 与 `baseline_run_id` 的分数，标记 `regression` / `improvement` / `unchanged`。 |
| 2.1.5 | Eval 报告回传 | `hnsx_worker/session_executor.py`、`worker_service.py` | `SessionFinalResult.result["eval_report"]` 包含 summary、per-case scores、latency、cost。 |

### 2.2 可观测与成本治理

| # | 任务 | 关键文件 | 验收标准 |
|---|---|---|---|
| 2.2.1 | OTLP Trace 导出 | `hnsx_worker/telemetry/traces.py`（新建） | Observation 以 OTLP span/event 形式导出，本地 Tempo 可见。 |
| 2.2.2 | Metrics 采集 | `hnsx_worker/telemetry/metrics.py`（新建） | 暴露 session/turn/adapter 调用/token/成本/延迟/eval score 等指标。 |
| 2.2.3 | 成本聚合回填 | `hnsx_worker/session_executor.py` | `SessionFinalResult` 回填 `total_cost_usd / total_prompt_tokens / total_completion_tokens`。 |
| 2.2.4 | 结构化日志 | `hnsx_worker/logging.py`（新建） | 所有模块输出 JSON 日志，携带 `session_id / trace_id / worker_id / correlation_id`。 |
| 2.2.5 | 关联 ID 透传 | `session_runtime.py`、`worker_service.py` | `trace_id`、`correlation_id`、`session_id` 在 observation、log、metric 中一致。 |

### 2.3 生产级 Sandbox

| # | 任务 | 关键文件 | 验收标准 |
|---|---|---|---|
| 2.3.1 | Container 后端 | `hnsx_worker/sandbox/container.py`（新建） | 用 Docker/Podman 执行 `tools/python`，支持镜像、env、网络开关。 |
| 2.3.2 | Process 后端增强 | `hnsx_worker/sandbox/process.py`（新建） | 子进程 + timeout/memory/cpu/stdout 限制；Linux 下可用 namespace/seccomp（可选）。 |
| 2.3.3 | Sandbox Profile | `hnsx_worker/sandbox/backend.py` | DomainSpec 按 tool/agent 指定 `sandbox: {backend, image, network, limits}`。 |
| 2.3.4 | 违规观测 | `hnsx_worker/sandbox/backend.py` | 超时/越权/资源超限自动 kill，emit `sandbox_violation` observation。 |

### 2.4 Worker 运维与韧性

| # | 任务 | 关键文件 | 验收标准 |
|---|---|---|---|
| 2.4.1 | Observation 批量化 | `hnsx_worker/worker_service.py` | `_obs_queue` 按大小/时间窗口 batch 发送，降低 gRPC 往返。 |
| 2.4.2 | 背压与降级 | `hnsx_worker/worker_service.py` | 队列超阈值上报 `WorkerHealthStatus.DEGRADED`，控制面可限流。 |
| 2.4.3 | 优雅关闭 flush | `hnsx_worker/worker_service.py` | shutdown 时 flush 完 `_obs_queue` 再退出，避免 observation 丢失。 |
| 2.4.4 | Worker 能力声明 | `hnsx_worker/worker_service.py` | Register 时上报支持的 sandbox backend、模型、provider，控制面按能力调度。 |

### 2.5 Policy / 人工审批（与控制面协同）

| # | 任务 | 关键文件 | 验收标准 |
|---|---|---|---|
| 2.5.1 | 输出 Guardrails | `hnsx_worker/policy/engine.py` | 支持关键词过滤、正则拦截、PII 检测，命中 emit `policy_violation`。 |
| 2.5.2 | 人工审批暂停 | `hnsx_worker/session_executor.py` | 收到 `hold` 命令后 session 进入 `paused`，保存上下文；收到 `resume` 后继续。 |
| 2.5.3 | Eval 预算 | `hnsx_worker/eval/runner.py` | EvalRun 级别 `max_cost_usd`，超预算自动停止未跑 case 并标记 `budget_exceeded`。 |

---

## 3. 推荐优先级

按“没有它后面没法量化”的原则排序：

1. **2.2 可观测与成本治理** — 先让每次运行都可被看见、可被度量。
2. **2.1 Eval 平台化** — 让 Eval 真正驱动 Harness 进化。
3. **2.3 生产级 Sandbox** — 决定外部代码能不能上生产。
4. **2.4 Worker 运维与韧性** — 让 worker 从“能跑”到“敢跑”。
5. **2.5 Policy / 人工审批** — 治理闭环，可与控制面并行推进。

---

## 4. 关键依赖

- Go 控制面需提供：
  - `EvalSet` 注入 `SessionRequest.eval_set`。
  - `baseline_run_id` 注入 EvalRun。
  - `hold` / `resume` ServerEvent。
  - Worker capability-based 调度。
- Protobuf 可能需扩展：
  - `SessionFinalResult` 已支持 `result` dict，当前足够，无需改 proto。
  - 如需新增 observation kind，再议。

---

## 5. 风险与应对

| 风险 | 应对 |
|---|---|
| OTLP 依赖增加 worker 体积 | 用 `opentelemetry-api/sdk` 可选依赖，本地模式可关闭。 |
| Container Sandbox 需要 Docker | 后端抽象保持 `none/process/container` 可插拔，测试用 `none`。 |
| LLM Judge 成本高 | 默认用 cheap model（Haiku/4o-mini），支持 judge model 配置。 |
| 批量化导致 observation 延迟 | 配置 `max_batch_size` + `max_batch_delay_ms`，默认保守。 |

---

## 6. 验收清单

- [ ] `make worker-test` 全绿。
- [ ] 一个 10-case EvalSet 跑完，LLM Judge 给出 pass/partial/fail 与理由。
- [ ] Baseline 对比能标记 regression。
- [ ] Grafana/Tempo 能看到一个完整 session 的 trace 和成本曲线。
- [ ] `tools/python` 在 container 后端下执行，无限循环 / 网络请求被拦截。
- [ ] Worker 队列打满后上报 `DEGRADED`，shutdown 时不丢 observation。
