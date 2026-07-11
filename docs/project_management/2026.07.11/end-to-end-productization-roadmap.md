# HnsX 端到端产品化 Roadmap

> 日期：2026-07-11
> 周期：从「实验骨架」到「可产品化」—— Phase A → Phase B → Phase C
> 北极星：[`docs/vision.md`](../../vision.md) — Harness as a Service
> 当前分支：`feat/tui_migration`
> 协议真相源：[`proto/hnsx/v1/`](../../../proto/hnsx/v1/)
> 前置状态：[`docs/project_management/2026.07.10/end-to-end-product-roadmap.md`](../2026.07.10/end-to-end-product-roadmap.md) 的 P0/P1 已基本清零，T15/T17 等 P2 项并入本路线图的 Phase C

---

## 0. 工作方式

- 每个 **T 票** = 一个独立分支 + 一个常规 commit。commit message 用 Conventional Commits，并在 commit body 引用本文件锚点（如 `Refs: docs/project_management/2026.07.11/end-to-end-productization-roadmap.md#a1`）。
- 每完成一个 T：在本文件**勾掉对应 `- [ ]`**（勾成 `- [x]`），该勾选与代码 commit **一起**入库，让"勾选即落库"成为强约束。
- 全栈纵切：涉及 server + worker + console + cli + proto 的 T，**必须同一 PR 内闭环**，不留"后端先合、前端再补"的中间态。
- 验收：每个 T 自带 `## 验收` 段，PR 模板强制对照勾选。

---

## 1. 本轮基线（2026-07-11 的真实状态）

| 层 | 当前 | 本轮要解决的核心问题 |
|---|---|---|
| **CLI / TUI** | 22 个顶层命令 + 7 tab TUI 已就位；human 输出不完整、配置层未落地、发布工程未通 | 从「命令树齐全」变成「日常可用 + 可安装升级」 |
| **Console** | 16 个页面、SSE、审批、审计、Trace 已落地；Dashboard mock、Domain 编辑器不能保存、缺业务用户入口 | 从「资源控制台」变成「业务工作空间」 |
| **Server / Worker** | Control Plane 与 Python Runtime 分离；P0 治理（Secret/Policy/Approval/Runtime）已闭环 | Server 本地执行仍只能 noop/echo；supervisor 等高级编排未调度；Eval 未发到 worker 池 |
| **Domain 创作** | 10 个 example-domains 存在；`hnsx validate` / `hnsx try` 已通 | `skills-demo` 校验失败；Go/Worker/Docs 三处 schema 不一致；无 `hnsx init`；`hnsx run` 不保真 |
| **SDK** | Node 仅导出 proto 类型；Go 是 placeholder；Python 空目录 | 三端无一可用，与"SDK + 平台"双形态差距最大 |
| **可观测性** | 协议和页面已建，本地默认不启 Tempo/Grafana | 默认链路未通，用户首次启动看不到 trace/cost/metric |
| **多租户** | `X-Tenant-ID` header 透传 | `auth_token → tenant` 映射未做，企业/SaaS 无法隔离 |

---

## 2. 本轮已确认的产品决策

以下问题已在 2026-07-11 讨论中拍板，本路线图按此执行：

1. **第一用户**：先服务 **Harness 设计师 / 业务工程师**（让普通人能编 Harness），再扩展到终端业务用户。
2. **Runtime 统一**：本地开发统一走 Python worker；`hnsx up` 默认启动 embedded worker，`hnsx run` 也走 worker 执行，不再在 Go server 里补真实适配器。
3. **SDK 优先级**：Node/TypeScript 第一（Console + Claude Code 生态），Python 第二（AI 工程师写 Skill/Eval），Go 第三（CI/运维）。
4. **不做低代码拖拽编排**，但做 **业务模板 + 输入表单 + 策略 presets** 作为中间层。
5. **CLI 默认入口**：保留 `hnsx` 无参数进 TUI，首次启动给 3 秒提示：`按 q 退出，hnsx --help 查看命令，hnsx --no-tui 永久禁用`。

---

## 3. Phase A — 可信的本地开发闭环（2–3 周）

目标：一个新人能在 10 分钟内 `hnsx init` → 改 prompt → `hnsx run` → 看到真实 Agent 输出。

---

### A1 — DomainSpec 单一真相源与 schema 收敛

- [x] **proto** 敲定 `SkillSpec` / `ToolSpec` / `PolicySpec`（含 `approval` 字段）/ `Store` 命名，删除已废弃的 `memory` 字段
- [x] **server** `hnsx-server/pkg/spec/domain.go` 与 proto 严格对齐
- [x] **worker** `hnsx-worker/hnsx_worker/domain_loader.py` / `skills/registry.py` / `tools/registry.py` 与 proto 对齐
- [x] **docs** 更新 `docs/know-how/我们如何建模Harness.md`，删除旧 `memory` 描述，补充 `store` 与 `policy.approval`
- [x] **examples** 修复 `example-domains/skills-demo/domain.yaml` 的 `skills[].tools` 类型；确保所有 example 通过 `hnsx validate`
- [x] **smoke** 在 `scripts/smoke-cli.sh` 或新增 `scripts/validate-examples.sh` 中加入全 example 校验

#### 验收

```bash
for d in example-domains/*/domain.yaml; do
  ./bin/hnsx validate --domain "$d" --output quiet
done
# 全部 rc=0
```

Refs：`proto/hnsx/v1/domain.proto`、`hnsx-server/pkg/spec/domain.go`、`example-domains/skills-demo/domain.yaml`

---

### A2 — `hnsx init` + `hnsx domain format`

- [x] **cli** 新增 `hnsx init` 命令（`cli/discovery.go` 或新建 `cli/init.go`），支持 `--template` / `--output-dir`
- [x] **templates** 内置 4 个模板：`customer-service`、`code-review`、`research-assistant`、`blank`
- [x] **cli** 在 `hnsx domain` 下新增 `format` 子命令（与现有 `hnsx power format` 共存并标注推荐路径）
- [x] **formatter** 实现 YAML 标准化：固定缩进、按字段顺序排序、删除空行、统一字符串样式
- [x] **tests** 单元测试覆盖模板生成和 format

#### 验收

```bash
./bin/hnsx init --template customer-service ./my-cs
# ./my-cs/domain.yaml 存在且 valid

./bin/hnsx domain format ./my-cs/domain.yaml
# 输出标准化后的 YAML，不改变语义
```

Refs：`cli/discovery.go`、`hnsx-server/pkg/domain/format/`（新增）

---

### A3 — `hnsx run` 真实执行（走 embedded Python worker）

- [x] **cli** `hnsx run` 改为：加载 DomainSpec → 启动/复用 embedded Python worker → 触发执行 → tail SSE/observations
- [x] **worker** 支持 CLI 本地模式传入 domain spec（stdin 或临时文件），不走 server registry
- [x] **adapters** 本地模式支持 `openai` / `anthropic` / `claudecode` / `codex` / `mcp`（复用 worker adapters）
- [x] **orchestration** 本地模式支持 `workflow` / `supervisor` / `multi-turn`（复用 Python runner）
- [x] **policy** 本地模式启用 budget / permission / guardrail / approval（可配 `--no-policy` 调试用）
- [x] **smoke** `scripts/smoke-cli.sh` 增加 `hnsx run --domain example-domains/mcp-demo/domain.yaml` 真实 MCP 调用断言

#### 验收

```bash
./bin/hnsx run \
  --domain example-domains/mcp-demo/domain.yaml \
  --trigger '{"path":"example-domains/mcp-demo"}'
# 返回真实文件列表（非 echo）
```

Refs：`hnsx-server/cmd/hnsx/cli/run.go`、`hnsx-worker/hnsx_worker/session_executor.py`、`hnsx-worker/hnsx_worker/adapters/`

---

### A4 — Console Domain 编辑器闭环

- [x] **server** 实现 `PUT /api/v1/domains/:id` 与 `POST /api/v1/domains/:id/validate`
- [x] **server** `POST /api/v1/domains/:id/run` 支持从已注册 domain 触发本地/远端 session
- [x] **console** `DomainDetailPage.tsx` 从 API 加载真实 harness YAML 并传入编辑器
- [x] **console** `DomainEditor.tsx` 绑定 `onSave` / `onValidate` / `onRun`
- [x] **console** `DomainsPage.tsx` 的 "Register Domain" 按钮绑定上传/粘贴 YAML 弹窗
- [x] **console** `VersionsPanel` 接入真实版本历史（当前硬编码 `total:1`，见 `hnsx-server/pkg/api/domains.go:122-138`）

#### 验收

- Console 中打开 `/domains/customer-service`，编辑 YAML 后 Save 成功；Validate 返回 valid；Run 触发新 session 并跳转详情。

Refs：`hnsx-console/src/pages/DomainDetailPage.tsx`、`hnsx-console/src/components/DomainEditor.tsx`、`hnsx-server/pkg/api/domains.go`

---

## 4. Phase B — 从控制台到业务工作空间（3–4 周）

目标：业务用户不用写 YAML，就能在 Console/CLI 上完成一个业务任务。

---

### B1 — Domain Workspace 视图

- [x] **console** 新增页面 `/domains/:id/workspace`（`hnsx-console/src/pages/DomainWorkspacePage.tsx`）
- [x] **console** 根据 `trigger_schema` 自动生成输入表单（优先用 React Hook Form + Zod）
- [x] **console** 提供自然语言输入框 + 模板选择 + 历史记录列表
- [x] **console** 结果展示卡片：最终输出、执行步骤摘要、cost/耗时
- [x] **console** 侧边栏路由区分 "Workspace" / "Editor" / "Runs" / "Evals"
- [x] **server** 如需 `GET /domains/:id/schema`，补充 endpoint 暴露 trigger_schema 与输入提示

#### 验收

- 业务用户打开 `/domains/customer-service/workspace`，输入 "我想退款"，看到 Agent 路由到 billing + 必要时触发审批 + 最终回答，全程不写 JSON。

Refs：`hnsx-console/src/App.tsx`、`hnsx-console/src/components/layout/Sidebar.tsx`、`hnsx-console/src/pages/DomainWorkspacePage.tsx`

---

### B2 — Session 实时流中的审批与 Policy

- [x] **console** `SessionDetailPage.tsx` 为 `paused` 状态绑定 Approve/Reject 按钮
- [x] **console** `ObservationTimeline.tsx` 渲染 approval 中断标记、预算/Policy 违规卡片
- [x] **console** Session 头部增加 Budget/Policy 摘要卡片（复用 server 返回的 cost/state）
- [x] **server** 确保 approval 决议后正确唤醒 session 并推送 SSE `session_resumed`
- [x] **worker** 接入 approval bus（当前 `session_executor.py:597` 中 `approval_bus=None`），让 worker 侧 policy 触发审批时能同步挂起

#### 验收

- `customer-service` 中触发 refund 审批，Console session 详情页出现 "Approve / Reject"；点击 Approve 后 session 继续并在时间线显示决议事件。

Refs：`hnsx-console/src/pages/SessionDetailPage.tsx`、`hnsx-console/src/components/ObservationTimeline.tsx`、`hnsx-worker/hnsx_worker/session_executor.py`

---

### B3 — Dashboard 真实数据 + 可观测性默认开启

- [ ] **console** `DashboardPage.tsx` 接入 `/api/v1/metrics`：24h cost、失败率、pending approvals、最近 sessions
- [ ] **console** `LocalObservabilityDashboard.tsx` 移除 mock，接入真实 metrics / trace 摘要
- [ ] **deploy** `deployments/local/docker-compose.yml` 默认启动 server + postgres + worker + tempo + grafana + OTLP exporter
- [ ] **server** `HNSX_OTEL_EXPORTER` 默认值为 `otlp`，未设置时不静默关闭
- [ ] **infra** 提供 5 张 Grafana dashboard JSON：session overview / cost / latency / policy violations / worker health
- [ ] **console** `/observability` 页面在未起 Grafana 时显示友好空状态 + 启动命令

#### 验收

- `hnsx up` 后打开 Console Dashboard，数字不是 mock；Grafana `http://127.0.0.1:3002` 可直接查看 trace。

Refs：`hnsx-console/src/pages/DashboardPage.tsx`、`deployments/local/docker-compose.yml`、`hnsx-server/internal/config/`

---

### B4 — 模板市场与策略 Presets

- [ ] **repo** 新增 `templates/` 目录或 `example-domains/templates.yaml` 索引，含标签、描述、变量、最小运行要求、策略包
- [ ] **cli** `hnsx examples` 读取模板索引，支持 `--tag` 过滤
- [ ] **console** 新增 `/gallery` 页面展示模板卡片，支持一键复制/初始化
- [ ] **proto/spec** 扩展 `PolicySpec` 支持 `presets: [safe_customer_service]` 高层 DSL
- [ ] **server/worker** policy engine 解析 presets 并展开为底层 guardrails
- [ ] **examples** 为 `customer-service` 添加 `policy.presets: [safe_customer_service]`，默认禁止 shell/文件写、退款需审批

#### 验收

```bash
./bin/hnsx init --template customer-service ./cs --set company_name=Acme
# domain.yaml 中 company_name 已被替换
```

Refs：`templates/`、`hnsx-server/pkg/policy/engine.go`、`hnsx-worker/hnsx_worker/policy/engine.py`

---

### B5 — CLI human 输出与配置层

- [ ] **cli** 实现 `~/.config/hnsx/config.yaml` 读取：`cli/config.go` 补全 viper/自写 loader，优先级 flag > env > config
- [ ] **cli** `show/get` 类命令增加 human 卡片/键值渲染（`cli/output.go`），不再只输出美化 JSON
- [ ] **cli** 统一 `--output human|json|quiet`，确保每条 `show` 命令三种模式都合理
- [ ] **cli** `hnsx config get/set/show` 命令组（可放入 `power` 或顶层）
- [ ] **tests** 单元测试覆盖配置加载优先级和 human 输出 snapshot

#### 验收

```bash
./bin/hnsx session show <id>
# human 模式下显示键值卡片，json 模式下仍是机器可读
```

Refs：`hnsx-server/cmd/hnsx/cli/config.go`、`hnsx-server/cmd/hnsx/cli/output.go`

---

## 5. Phase C — 企业就绪与生态（4–6 周）

目标：外部团队能把 HnsX 集成进自己的系统，企业能隔离、计费、审计。

---

### C1 — Node/TypeScript SDK

- [ ] **sdk/node** 新增 `src/client.ts`：封装 `DomainRegistry`、`Session`、`Eval`、`Trace`、`Approval` REST API
- [ ] **sdk/node** 新增 SSE 消费 helper：`streamSessionEvents(sessionId)`
- [ ] **sdk/node** 更新 `package.json` exports：`@hnsx/sdk-node/client`
- [ ] **sdk/node** 补 README + 使用示例 + 单元测试（msw）
- [ ] **sdk/node** 与 Console 共享类型：`src/gen/` 已存在，client 复用

#### 验收

```ts
import { HnsXClient } from '@hnsx/sdk-node/client';
const client = new HnsXClient('http://127.0.0.1:50052');
const session = await client.sessions.trigger({ domainId: 'customer-service', trigger: { question: 'hi' } });
```

Refs：`sdk/node/src/index.ts`、`sdk/node/src/client.ts`

---

### C2 — Python SDK

- [ ] **sdk/python** 新建包：`hnsx/client.py`、`hnsx/models.py`、`hnsx/errors.py`
- [ ] **sdk/python** 复用 `hnsx_worker/proto_client` 或新写 REST client（优先 REST，与 Node 一致）
- [ ] **sdk/python** DomainSpec builder helper：`DomainSpecBuilder`
- [ ] **sdk/python** SSE 消费 helper
- [ ] **sdk/python** `pyproject.toml`、README、pytest 测试

#### 验收

```python
from hnsx import HnsXClient
client = HnsXClient("http://127.0.0.1:50052")
session = client.sessions.trigger(domain_id="customer-service", trigger={"question": "hi"})
```

Refs：`sdk/python/pyproject.toml`、`sdk/python/hnsx/client.py`

---

### C3 — Go SDK

- [ ] **sdk/go** 新建 `client.go`：封装 `internal/client` 的 REST/Connect 调用
- [ ] **sdk/go** 复用 `hnsx-server/pkg/spec` 的 DomainSpec 类型
- [ ] **sdk/go** 提供 `NewClient(baseURL)`、`RegisterDomain`、`TriggerSession`、`ListTraces` 等
- [ ] **sdk/go** 补 README、go test、纳入 `go.work`

#### 验收

```go
import "github.com/hnsx-io/hnsx/sdk/go"
client := hnsx.NewClient("http://127.0.0.1:50052")
session, _ := client.Sessions.Trigger(ctx, "customer-service", trigger)
```

Refs：`sdk/go/client.go`、`sdk/go/README.md`、`go.work`

---

### C4 — 认证 → 租户映射与 RBAC

- [ ] **server** auth middleware：从 `Authorization` header 解析 token/jwt，映射到 tenant_id
- [ ] **server** 无 token 时返回 401；token 映射失败返回 403
- [ ] **server** 所有 repo 查询增加 `tenant_id` 过滤（或启用 Postgres RLS）
- [ ] **server** 新增 RBAC 角色：platform_admin / harness_designer / operator / auditor
- [ ] **console** 新增 `/login` 页与 auth status 组件
- [ ] **cli** `hnsx auth login/status/logout` 命令组

#### 验收

- 请求不带 `Authorization` 时返回 401；tenant A 的 session 在 tenant B 下不可见。

Refs：`hnsx-server/pkg/api/router.go`、`hnsx-server/internal/auth/`（新增）、`hnsx-server/internal/tenant/`

---

### C5 — Worker 池上的 Eval 与高级编排

- [ ] **server** `RunEval` 改为将每个 case 作为 session 请求 enqueue 到 worker pool
- [ ] **server** 聚合 worker 回传的 observation 与 result，生成 EvalRun report
- [ ] **server** scheduler 识别 `supervisor` / `hierarchical` / `autonomous` 模式并分发给 worker
- [ ] **worker** eval runner 与 server 上报格式对齐（复用 `hnsx-worker/hnsx_worker/eval/runner.py`）
- [ ] **examples** 确保 `claude-triage`、`workflow-demo` 走 server → worker 全链路跑通

#### 验收

```bash
./bin/hnsx eval run claude-triage-eval --concurrency 4
# 跑在 worker 池上，输出包含 baseline diff 的报告
```

Refs：`hnsx-server/internal/evaluation/runner/runner.go`、`hnsx-server/pkg/controlplane/scheduler_service.go`、`hnsx-worker/hnsx_worker/eval/runner.py`

---

### C6 — 插件机制与发布工程

- [ ] **cli** 实现 `hnsx plugin list/install/uninstall`：扫描 `~/.config/hnsx/plugins/` 外部子命令
- [ ] **ci** GitHub Actions release workflow：构建 `hnsx_<os>_<arch>.tar.gz`、计算 sha256、cosign 签名、SBOM
- [ ] **packaging** 替换 `packaging/homebrew/hnsx.rb` 占位值，创建 `hnsx-io/homebrew-hnsx` tap
- [ ] **scripts** `scripts/install.sh` 启用 checksum 校验，修正 `REPO` 指向真实仓库
- [ ] **repo** `Makefile` `VERSION` 提升至 1.0.0，发布 v1.0.0 tag
- [ ] **docs** README 首行改为 `curl -sSL hnsx.dev/install.sh | sh` + `hnsx try customer-service`

#### 验收

```bash
curl -sSL hnsx.dev/install.sh | sh
hnsx version          # 1.0.0
hnsx try customer-service
brew install hnsx-io/hnsx/hnsx
```

Refs：`hnsx-server/cmd/hnsx/cli/power.go`、`packaging/homebrew/hnsx.rb`、`scripts/install.sh`、`.github/workflows/`

---

## 6. 跨 Track 跟随项（每个 T 都要做到）

- [ ] 每个 T 必跑 `cd hnsx-server && go build ./... && go test ./...`
- [ ] 动 proto 的跑 `make proto` 后再动 Go/TS/Python
- [ ] 动 Console 的到 `hnsx-console` 跑 `pnpm install --force && pnpm type-check && pnpm build`
- [ ] 动 worker 的跑 `make worker-test` 或 `python -m pytest`
- [ ] 动 CLI 的更新 `scripts/smoke-cli.sh` 并跑通
- [ ] 每个 T 的 PR 末尾带：`🤖 Generated with [Claude Code]` + 引用本文件锚点

---

## 7. 风险与守则

| 风险 | 守则 |
|---|---|
| Phase A schema 统一改动面大，可能把 console/worker 同时搞挂 | 先把 proto 定稿，再同步三端，**同一 PR 内跑通 `hnsx validate` 全 example** |
| `hnsx run` 走 embedded worker会引入 Python 依赖，本地安装体验变差 | 提供 `--adapter noop` 纯 Go 快速模式；生产安装包可带可选 worker bundle |
| Phase B Workspace 与现有 Domain Editor 功能重叠 | 明确分层：Workspace 给业务用户，Editor 给 Harness 设计师，两入口并存 |
| Dashboard 接真实数据后 P95 变差 | 依赖 `/api/v1/metrics` 聚合端点，避免前端直接扫 observations 表 |
| SDK 三端同时开工会人力分散 | 按 Node → Python → Go 顺序做，**C1 不完成不开 C2/C3** |
| Phase C 多租户与 RBAC 改动侵入所有 repo | 单独开一个 `feat/tenant-rbac` 分支，不在其他 T 里混着改 |
| 发布工程依赖 GitHub release 和域名 | 先用 GitHub Releases 做 tar，域名 `hnsx.dev` 可用后再切 install.sh URL |

---

## 8. 收尾协议

每个 T 落地完成后：

1. 在本文件勾掉 `- [ ]`（commit 携带此变化）
2. commit message：`feat(server|console|cli|worker|sdk|infra|docs): <t-id> <短描述>\n\nRefs: docs/project_management/2026.07.11/end-to-end-productization-roadmap.md#<t-id>`
3. 推到远端，开 PR
4. 不直接合 `main` —— 等 review

Phase A 全部勾完后，启动 Phase B 排期；Phase B 全部勾完后，启动 Phase C。**每个 Phase 清零时更新本文件 §0 周期字段与进度条。**

---

*owner：HnsX squad · last_updated：2026-07-11*
