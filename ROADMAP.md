# HarnessX Roadmap v0.2

> **状态**：Active — W0
> **最后更新**：2026-07-14
> **修订记录**：v0.2 删 Python worker，daemon 改为单一 Go 二进制

---

## TL;DR

| 阶段 | 周 | 主题 | Demo |
|---|---|---|---|
| **P0** | W1-W4 | Multica 接 HarnessX 后端 | Multica 全功能跑通 HarnessX server |
| **P1** | W5-W12 | **HarnessX Daemon（单 Go 二进制）+ 治理** | 删 `hnsx-worker/`，单 Go daemon 跑治理 |
| **P2** | W13-W20 | HarnessX Domain 注册表 + Squad 2.0 | 创建 Domain → 关联 Multica agent → 全程治理 |
| **P3** | W21-W28 | 发布 + 5 pilot | `brew install harnessx-ai/tap/harnessx`、5 团队跑通 |

**总投入**：~44 人周。2 人 × 22 周理论，×1.5 buffer = **7 个月（28 周）**。

---

## 重大变更（vs v0.1）

| 项 | v0.1 | **v0.2（当前）** |
|---|---|---|
| Daemon 实现 | Multica Go daemon + HnsX Python worker | **单一 HarnessX Daemon（Go）** |
| Python 代码 | ~2 万行 worker 代码 | **0 行**（`hnsx-worker/` 整目录删） |
| Harness engine 在哪 | Python worker 子进程 | **daemon in-process** |
| CC/Codex 怎么用 Harness | Python engine 包一层 | **CC 通过 MCP 直接调 daemon 的 harnessx_\* tools** |
| 用户安装 | brew + Python venv | **brew install 一行** |

**为什么删 Python worker**：CC/Codex 本身就是 binary CLI，daemon 再起一个 Python 进程做 harness 是给已经在跑 binary 的 binary 加 wrapper，没有意义。Go 完全能干 harness engine 的活（DomainSpec 解析、Policy、Approval、Skill resolver、MCP server 都有成熟生态），单一语言 = 单一团队 = 单一进程。

---

## P0（W1-W4）：Multica 接 HarnessX 后端

**Goal**：Multica Next.js + CLI + Daemon 一行不改，跑在 HarnessX server 上。**第一个真产品**。

**daemon 状态**：暂时还是 Multica 的 Go daemon，没 HarnessX 能力。

### 周交付

| 周 | Deliverable |
|---|---|
| **W1** | • Fork `multica-ai/multica` 到 HarnessX org，pin main commit<br>• 建 `harnessx-server/pkg/multica_adapter/` 目录<br>• 写 3 份学习笔记：`documents/learn/multica-{squad,protocol,schema}.md`<br>• 决定 daemon fork commit hash |
| **W2** | • HarnessX server 加 `--multica-mode`，把 Multica chi 路径抄成 gin 路由表（**只抄壳**）<br>• 实现 `multica_adapter/handler_agent.go`：`ListAgents` / `GetAgent` / `CreateAgent`<br>• CLI 验证：`multica agent list` 走 HarnessX 后端 |
| **W3** | • 实现 `handler_issue.go`：`ListIssues` / `CreateIssue` / `AssignIssue`<br>• 实现 `handler_daemon.go`：`Register` / `Heartbeat` / `ClaimTask` / `StartTask` / `CompleteTask`<br>• 实现 `protocol_ws.go`：HarnessX Observation ↔ Multica TaskMessage 翻译 |
| **W4** | • 实现 `handler_squad.go` + `handler_autopilot.go`<br>• 跑 Multica e2e 套件<br>• **P0 demo**：assign issue → CC 跑 → Next.js 时间线滚动<br>• 录 demo 视频 |

### Definition of Done

- [ ] Multica Next.js 一行不改，连 HarnessX server 全功能
- [ ] Multica CLI 一行不改，所有命令走 HarnessX
- [ ] Multica Daemon 一行不改，spawn CC 跑通
- [ ] Multica e2e 测试 HarnessX 后端全绿
- [ ] P0 demo 5 分钟跑通，录视频

### 不在 P0

- ❌ Policy / Approval / Budget / Audit / Eval
- ❌ Domain registry
- ❌ HarnessX Daemon
- ❌ 任何治理能力

---

## P1（W5-W12）：HarnessX Daemon + 治理能力

**Goal**：**删 `hnsx-worker/`**（-2 万行 Python）。建 `harnessx-daemon/`（单 Go 二进制），把 CLI 生命周期 + Harness engine + MCP server 全装进来。然后接治理。

**这是 Roadmap 里最重的一段，2 人 × 8 周 = 16 人周。**

### 周交付

| 周 | Deliverable |
|---|---|
| **W5** | • **删** `hnsx-worker/` 整目录（连带 `hnsx-server/internal/worker/`、`hnsx-server/pkg/controlplane/` 里的 Python 桥接）<br>• 建 `harnessx-daemon/`（新顶级包，或 `hnsx-server/cmd/harnessx-daemon/`）<br>• `main.go` 骨架：`harnessx daemon install / start / status / logs / stop`<br>• Multica WS 协议：`Register` / `Heartbeat` / `PullSession` / `StreamChannel`（`wire/` 包） |
| **W6** | • CLI Lifecycle Manager：spawn `claude` / `codex` subprocess<br>• 进程 lease / renew / kill<br>• stream-json 解析 → Observation<br>• **移植 5 个核心 adapter**（CC / Codex / CodeBuddy / Copilot / Cursor）到 `cli/`<br>• HarnessX Daemon 替代 Multica daemon 跑通一条 issue |
| **W7** | • Harness Engine 骨架（`engine/executor.go`）：DomainSpec → HarnessRunner → Turn<br>• **MCP server in Go**（`github.com/mark3labs/mcp-go`）：stdin/stdout 暴露 harnessx_* tools<br>• 首批 5 个 MCP tools：`harnessx_skill_load` / `harnessx_policy_check` / `harnessx_approval_request` / `harnessx_audit_record` / `harnessx_observation_emit`<br>• Daemon 启动 CC 时注入 `mcp_servers` 配置 |
| **W8** | • Policy engine（`engine/policy.go`）：cost / permission / guardrail 三种 rule<br>• Approval flow（`engine/approval.go`）：daemon → server HTTP → 等待 approve/reject → 恢复 session<br>• Skill resolver（`engine/skills.go`）：YAML + Markdown skill 加载<br>• Memory backend（`engine/memory.go`）：sqlite local<br>• Eval runner（`engine/eval.go`）：scorer + regression baseline |
| **W9** | • Tool registry（`tools/`）：http / sql / shell / memory / approval / mcp_client（**全部 Go**）<br>• 移植剩余 10 个 agent（kimi / kiro / opencode / openclaw / hermes / pi / qoder / traecli / antigravity / codebuddy 之外的）<br>• Sandbox（`engine/sandbox.go`）：process namespace / os.Exec 隔离 |
| **W10** | • **Switchover**：Multica daemon 标记 deprecated，HarnessX Daemon 接管所有本地 runtime<br>• Next.js 加 `/cost` `/audit` `/approvals` 路由（复用现有 DataTable）<br>• 端到端跑通：assign issue → 高 cost → HarnessX daemon 拦下 → Approval UI 批 → 恢复 |
| **W11** | • 修所有 bug + 加测试<br>• Multica daemon 集成代码删干净<br>• 写 `documents/learn/harnessx-daemon.md` 架构文档 |
| **W12** | • **P1 demo**：3 个 Killer 场景全可演示<br>• 录 demo 视频（周末值班 / 成本失控 / SOC2 审计）<br>• 内部 review，准备 P2 |

### Definition of Done

- [ ] `hnsx-worker/` 整目录不存在（`rm -rf` 验证）
- [ ] `harnessx-daemon/` 单 Go 二进制，包含 CLI lifecycle + Harness engine + MCP server
- [ ] 15 个 agent adapter 全移植到 Go，daemon 能 spawn 任何一个
- [ ] Multica daemon 集成代码删除
- [ ] 三个 Killer 场景 demo 跑通
- [ ] HarnessX Daemon 跑通：assign issue → CC 用 Harness MCP tools → Policy 卡住 → Approval 批 → 继续

### 不在 P1

- ❌ Domain registry UI（放 P2）
- ❌ Eval runner UI（放 P2）
- ❌ Squad leader 2.0（放 P2）

---

## P2（W13-W20）：HarnessX Domain 注册表 + Squad leader 2.0

**Goal**：每个 Multica agent 能挂 HnsX Domain spec，**用 HarnessX transitions 替代纯 prompt routing**。

### 周交付

| 周 | Deliverable |
|---|---|
| **W13-W14** | • Schema：跑 Multica 170 migrations + 加 `hnsx_domain` / `hnsx_policy` / `hnsx_approval` / `hnsx_audit_log` / `hnsx_eval_*`<br>• 数据回填：从 Multica agent + skill 生成初始 `hnsx_domain`<br>• `ALTER TABLE agent_task_queue` 加 hnsx_* 列 |
| **W15-W16** | • `/api/harnessx/domains`：CRUD + version + run<br>• Next.js `/harnessx/domains` 路由 + DomainRegistryPage（复用 DataTable + Monaco 看 spec）<br>• Multica agent / squad detail 页加 "Linked HarnessX Domain" 字段 + 跳转 |
| **W17-W18** | • `multica_adapter/squad_harness_integration.go`：Squad leader 委派**优先**走 HnsX WorkflowSession<br>• 没 transitions 时回退 Multica 原 LLM prompt（兼容旧 squad）<br>• 加 Observation 通道：`kind=routing_decision` |
| **W19-W20** | • Eval runner UI：每个 Squad 关联 EvalSet，跑回归<br>• **P2 demo**：创建 Domain for "PR review with `policy.budget=$2`" → Multica agent 引用 → CC 跑 → cost 被 Budget 卡 → 走 Approval |

### Definition of Done

- [ ] 创建 HnsX Domain → 关联 Multica agent → 全程治理链路可演示
- [ ] Squad leader 委派决策作为 Observation 可见（`kind=routing_decision`）
- [ ] Eval runner 跑通：创建 EvalSet → run → 看 score + regression baseline
- [ ] Multica daemon 完全不参与运行时

---

## P3（W21-W28）：发布 + 首批 5 pilot

| 周 | Deliverable |
|---|---|
| **W21-W23** | • CLI binary `multica` → `harnessx` rebrand<br>• brew tap `harnessx-ai/tap/harnessx`<br>• `harnessx setup` / `harnessx daemon install` 一键安装（macOS launchd + Linux systemd）<br>• 域名 `harnessx.ai` 准备好 |
| **W24-W25** | • Docs 站 fork + rebrand（apps/web 顶部 HarnessX logo + 域名）<br>• README 重写：HarnessX = Multica + Harness 治理<br>• Self-host docker-compose 准备好（复用 Multica）<br>• 完整文档：Domain spec / Policy / Approval / Audit / Eval |
| **W26-W28** | • 找 5 个 20-50 人工程团队（CC/Codex 重度 + 有合规需求）做 pilot<br>• 免费用 3 个月，换：周会反馈 + case study 授权<br>• 1 篇 case study 发布<br>• 收集反馈，做 v0.2 计划 |

### Definition of Done

- [ ] `brew install harnessx-ai/tap/harnessx` 可装
- [ ] `harnessx setup` 一键拉起 daemon
- [ ] 5 个 pilot 团队跑通
- [ ] 1 篇 case study 发布
- [ ] v0.2 backlog 写好

---

## 不在 Roadmap 里（防 scope creep）

- ❌ 任何 Python 代码
- ❌ 重写 Multica Next.js
- ❌ 加新 agent adapter（HarnessX Daemon 用 Multica 移植过来的 15 个）
- ❌ Visual workflow editor
- ❌ 多租户 SaaS billing（pilot 阶段免费）
- ❌ Fine-tuning / training
- ❌ Self-improve / auto-evolve
- ❌ 移动端改造（复用 Multica iOS，pilot 之后再说）
- ❌ Multica iOS rebrand（先用 Multica iOS）

---

## 团队配比（W1-W28 持续）

| 角色 | 人数 | P0-P2 主力 |
|---|---|---|
| **Go 后端** | 1 | HarnessX Server adapter + **HarnessX Daemon 全栈**（W5-W12 主力） |
| **全栈** | 1 | Next.js 增量页（/approvals /cost /audit /harnessx/domains）+ Eval runner UI |
| **Python 工程师** | **0** | ❌ 整个 roadmap 不需要 |
| **产品 / GTM** | 0.5 | 文档 / demo 视频 / pilot 招募 |

**关键招聘**：W5-W12 需要 2 个 Go 工程师。如果只有 1 个，P1 拉长到 12 周，roadmap 整体到 32 周。**核心岗：1 个 Go 工程师熟悉 gRPC + 多进程管理**。

---

## 关键节点（4 个 demo gate）

| Gate | 周 | 通过标准 |
|---|---|---|
| **P0 demo** | W4 | Multica 全功能跑在 HarnessX 后端，录视频。daemon 仍为 Multica |
| **P1 demo** | W12 | HarnessX Daemon（单 Go 二进制）替代 Multica daemon + 治理三态 + Approval UI + Cost Dashboard + Audit Export |
| **P2 demo** | W20 | HarnessX Domain for PR review 走 Multica agent 全链路 |
| **P3 launch** | W28 | brew tap 上线、5 pilot 团队跑通、case study 发布 |

---

## 28 周后图景

- HarnessX = Multica（70%）+ Harness 治理（30%）融为一体的产品
- 单一 Go 二进制 daemon，零 Python 依赖
- 5 pilot 团队免费用完转付费，ARR 目标 $300k
- Multica 上游 2-5 个 PR（先小后大）
- 跑通商业化闭环，准备 Series A 或自收支持续

---

## 工作量汇总

| 阶段 | 人周 | 风险 |
|---|---|---|
| P0（W1-W4） | 8 | 低，Multica 文档全 |
| P1（W5-W12） | **16** | **高，单 Go 二进制从 0 写 + 移植 15 adapter + Harness engine** |
| P2（W13-W20） | 12 | 中，Domain registry + Squad 2.0 |
| P3（W21-W28） | 8 | 低，rebrand + 招募 |
| **合计** | **44 人周** | 2 人 × 22 周 = 5.5 月（理论） |

实际节奏要 ×1.5 buffer，所以 **7 个月**（即 Roadmap 28 周）。

---

## 关键决策（已锁定）

1. **品牌**：HarnessX（HnsX 作为内部 engine 名字保留）
2. **Fork Multica**，不重写前端/CLI/daemon
3. **Python worker 整体删除**，daemon 改为单 Go 二进制
4. **Multica 的 Squad leader 增强** 为 HarnessX 的差异化叙事核心
5. **Domain registry + Policy/Approval/Eval** 是 HnsX 注入的价值
6. **不碰 Coze 形态**（visual workflow editor），不跟 Anthropic 抢通用 agent 市场

---

## 相关文档

- `documents/learn/multica-squad.md`（W1 写）— Squad leader 学习笔记
- `documents/learn/multica-protocol.md`（W1 写）— Multica WS 协议分析
- `documents/learn/multica-schema.md`（W1 写）— Multica 数据库 schema 分析
- `documents/learn/harnessx-daemon.md`（W11 写）— HarnessX Daemon 架构文档
- `documents/learn/python-worker-reshape.md`（W5 写）— Python worker 删除决策与迁移方案
- `documents/learn/multica-fork-strategy.md`（W1 写）— Fork commit 选择 + 同步策略