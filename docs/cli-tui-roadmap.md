# HnsX TUI RoadMap & 技术方案

> **起草日期：2026-07-11** · **状态：草案，待评审**
> **目标版本：v1.1（在 v1.0 之上叠加，不破坏现有 CLI / Console）**

TUI 不是 `hnsx` 的一个子命令，而是 **CLI 产品本身的默认交互面**。新人装好 CLI 后，敲 `hnsx` 就应该进入一个键盘可达、终端原生、可日常使用的运维界面；带参数调用时则保持现有精确命令行为，供 CI / Agent / 脚本消费。

---

## 0. 现状盘点

| 项 | 现状 | 体感 |
|---|---|---|
| `hnsx tui` 命令 | 占位符（v0.5 引入），只打印引导信息 | 没用 |
| 命令拼装能力 | 完整（v0.3-v1.0 落地） | 强 |
| Console（GUI） | 16 个页面已就位 | 强 |
| **默认交互面**：无参数即进入 TUI | **零** | 缺 |

产品定位：**TUI = CLI 的"主界面"**，是人在终端里操作 HnsX 的第一入口；Console 是复杂可视化场景的深度界面，命令式 CLI 是脚本与自动化的精确接口。

---

## 1. 设计目标

### 1.1 用户场景

| 场景 | 用什么 |
|---|---|
| 日常打开 HnsX 看看状态 | `hnsx` → TUI Dashboard |
| 排查一个失败的 session | TUI sessions tab + trace 详情 + debug bundle 入口 |
| 盯一个跑着的 session 看 Observation 流 | TUI sessions tail |
| 处理 pending approvals | TUI approvals tab（一键 a/r） |
| 跑一个 eval、看 diff | TUI eval tab |
| 翻审计 / 成本 / 异常 | TUI audit + dashboard tab |
| 探索式发现新 domain | TUI domains tab |
| CI / cron 里触发 session | `hnsx session trigger --domain X --json` |
| 给老板做可视化汇报 | Console（GUI） |

**TUI 不替代 GUI，也不替代命令式 CLI**——它是 CLI 作为"产品"时给人的默认交互层。

### 1.2 非目标

- ❌ 不做鼠标交互（terminal-first）
- ❌ 不做复杂图表（用 `+` / `●` / `✓` 字符够用，需要复杂图表请用 Grafana）
- ❌ 不重做 Console 已有页面（Domain 编辑、YAML diff 等）
- ❌ 不支持多 server profile（v1.1 限定一个 server；profile 留给 v1.2）

---

## 2. 入口策略：TUI 是默认，命令是精确层

```text
$ hnsx
┌─ HnsX TUI · hnsx 1.1.0 · http://127.0.0.1:50052 · server ✓ ok · 14:32 ──┐
│ ...                                                                     │

$ hnsx session list --domain customer-service --json
{ "sessions": [...] }

$ hnsx --help              # 显式请求帮助，不走 TUI
$ hnsx --no-tui            # 本次禁用 TUI，直接输出 help
$ HNSX_NO_TUI=1 hnsx       # 环境变量永久禁用
```

### 2.1 启动规则

| 条件 | 行为 |
|---|---|
| `hnsx` 无参数 + TTY + 未禁用 | 启动 TUI |
| `hnsx` 无参数 + 非 TTY / 管道 | 输出 help（现有行为） |
| `hnsx <command>` | 执行对应命令，与今天一致 |
| `hnsx --no-tui` / `-T` | 禁用 TUI，输出 help |
| `HNSX_NO_TUI=1` | 环境变量永久禁用 |

### 2.2 与 `hnsx tui` 的关系

- v1.1 起 **`hnsx tui` 不再作为独立子命令存在**。
- 保留的等价入口：
  - `hnsx`（推荐）
  - `hnsx --tui`（显式）
- v0.5 的占位命令在 v1.1 移除，避免用户形成"TUI 是一个命令"的心智。

### 2.3 与 Console 的关系

| 界面 | 入口 | 定位 |
|---|---|---|
| **TUI** | `hnsx` | 日常运维、快速查看、审批、tail |
| **Console** | `hnsx console` | 复杂可视化、YAML 编辑、报告、给非终端用户 |
| **CLI 命令** | `hnsx <verb> <resource>` | CI、脚本、Agent、精确操作 |

---

## 3. 顶层布局（视觉稿）

```text
┌─ HnsX TUI · hnsx 1.1.0 · http://127.0.0.1:50052 · server ✓ ok · 14:32 ──┐
│                                                                        │
│  Sessions   Traces   Approvals(2)   Eval   Audit   Domains   Dashboard │
│ ────────────────────────────────────────────────────────────────────── │
│                                                                        │
│  [当前 tab 的主视图]                                                   │
│                                                                        │
│                                                                        │
│                                                                        │
│                                                                        │
│                                                                        │
├────────────────────────────────────────────────────────────────────────┤
│ tab:1/7  │  ↑↓/jk 选择  │  enter 详情  │  a approve  │  r rerun       │
│ refresh: 2s  │  q quit  │  ? help  │  / command  │  esc back        │
└────────────────────────────────────────────────────────────────────────┘
```

三段式：**Header（状态） + Tab Bar（导航） + Body（视图） + Footer（快捷键 / 命令行）**。按 `/` 时 Footer 变为命令输入行。

---

## 4. 词汇表（Tabs）

| Tab | 主视图 | 详情视图 | 关键动作 |
|---|---|---|---|
| **Sessions** | 表格（state / domain / id / age / cost） | Observation 时间线 + SSE 流 | `enter` tail，`r` rerun，`x` cancel，`a` approve |
| **Traces** | 表格（id / session / domain / duration / cost） | 树形 observation | `enter` 看树，`/` filter |
| **Approvals** | 表格（session / 风险 / age） | 详情 + reason | `a` approve，`x` reject（带 reason 输入） |
| **Eval** | 表格（set / domain / cases / last run） | run 详情 + case diff | `r` 跑，`d` diff（两 run 比） |
| **Audit** | 反向时间流表格 | 单条 detail | `/` filter by actor / resource |
| **Domains** | 表格（id / version / status / updated） | 单 spec | `r` run 一次 session |
| **Dashboard** | 数字卡片 + sparkline（pending / 24h cost / 失败率） | — | `g` refresh |

`approvals(2)` 这种小数字表示待办数量，紧贴 tab 名，每 5s 自动刷新。

---

## 5. 键盘绑定

### 5.1 全局

| Key | 动作 |
|---|---|
| `1`–`7` | 切换 tab |
| `tab` / `shift+tab` | 下一个 / 上一个 tab |
| `?` | 帮助浮层 |
| `q` / `ctrl+c` | 退出 TUI |
| `/` | 进入命令模式（输入 `/session <id>` 等） |
| `esc` | 返回 / 关闭浮层 / 取消命令模式 |
| `f` | 当前 tab 内快速过滤（等价 `/filter`） |
| `g` / `G` | 跳到第一/最后一行 |
| `r` | 刷新当前视图 |

### 5.2 各 tab 专属

| Tab | Key | 动作 |
|---|---|---|
| Sessions | `enter` | 进入 tail 视图 |
| | `x` | cancel |
| | `R` | rerun |
| | `a` | approve（如 paused） |
| Sessions/Traces | `f` | filter by state |
| Traces | `enter` | 树形 observation |
| Approvals | `a` | approve（无 reason） |
| | `x` | reject（弹 reason 输入） |
| Eval | `r` | start run |
| | `d` | diff（选两个 run） |
| Domains | `R` | trigger session（弹 trigger 输入） |
| | `enter` | 展开 spec |

### 5.3 设计原则

- **单字母为常用动作**（`a/r/x/q/g`），不需要 modifier
- **破坏性操作必二次确认**（reject / cancel / rerun 弹 y/n）
- **`?` 永远显示当前 tab 的快捷键**
- **`/` 是命令模式入口**，把"记不住快捷键"转化为"打得出的命令"；命令与 CLI 词表保持一致

---

## 6. 状态 / 数据流

```text
┌────────────────────────────────────────────────────────────┐
│  Root Model (tea.Model)                                    │
│                                                            │
│  ┌─────────┐  ┌──────────┐  ┌──────────┐                  │
│  │ header  │  │ tab bar  │  │ footer   │                  │
│  └─────────┘  └──────────┘  └──────────┘                  │
│       │            │             ▲                          │
│       ▼            ▼             │                          │
│  ┌──────────────────────────────────────────────┐          │
│  │  Active Tab (tea.Model)                       │          │
│  │                                               │          │
│  │   Init() -> tea.Cmd (load data)               │          │
│  │   Update(msg) -> (Model, tea.Cmd)             │          │
│  │   View() -> string                            │          │
│  └───────────────────┬──────────────────────────┘          │
│                      │                                     │
│                      ▼ tick (2s)                            │
│               ┌────────────────┐                            │
│               │ Data Fetcher   │  ── uses ──> internal/client
│               └────────────────┘                            │
└────────────────────────────────────────────────────────────┘
```

### 6.1 数据获取

| 模式 | 用在哪 |
|---|---|
| **Poll** | 列表（sessions/traces/approvals/audit/domains）每 2s 拉一次 |
| **SSE** | 进入 session 详情时订阅 `/events` 实时流 |
| **One-shot** | eval run start / approve / cancel |

### 6.2 并发

bubbletea 是单 goroutine，所有 `tea.Cmd` 都通过 channel 喂回消息。CLI 自己起后台 goroutine 跑 poll/SSE（带 ctx cancel），完成后 `program.Send()` 把结果推进消息队列。

### 6.3 Client

复用 `internal/client/client.go`。TUI 子包加一层薄包装，提供：

```go
type Client struct{ *client.Client }
func (c *Client) ListSessions(ctx) ([]client.SessionListItem, error)
func (c *Client) StreamSessionEvents(ctx, id) (<-chan Event, <-chan error, error)
// ...
```

不重写协议。

---

## 7. 视觉主题

跟 Web Console 保持一致（morandi 调色板）：

| 用途 | 颜色（hex） | lipgloss name |
|---|---|---|
| Primary | `#7A8B7F`（灰绿） | `morandi.primary` |
| Success | `#7E9B7A` | `morandi.success` |
| Warning | `#C9A87C` | `morandi.warning` |
| Danger | `#B57F7F` | `morandi.danger` |
| Info | `#7E8FB0` | `morandi.info` |
| Muted | `#9C9C92` | `morandi.muted` |
| Bg | `#F5F1EA`（亮）/ `#2B2A28`（暗） | `morandi.bg` |

**自适应**：检测 `NO_COLOR` / 终端能力 → 退化到 256 色或纯 ASCII 边框。

样式构件：

- `Title` — 大标题
- `Tab.Active` / `Tab.Inactive` — tab 高亮
- `Row.Selected` / `Row.Normal` — 列表行
- `Badge.{Success,Warning,Danger,Info}` — 状态徽章
- `Footer` — 灰底分隔栏

---

## 8. 组件拆分

```
cli/tui/
├── tui.go           # 入口 (Program 启动)
├── theme.go         # morandi 调色板 + 样式
├── keys.go          # 全局 KeyMap
├── model.go         # Root Model（持有 ActiveTab）
├── client.go        # 复用 internal/client 的薄包装
├── statusbar.go     # 顶部 server 健康 / version / 时钟
├── tabs/
│   ├── sessions.go    # SessionsTab + SessionDetail (tail)
│   ├── traces.go      # TracesTab + TraceDetail (tree)
│   ├── approvals.go   # ApprovalsTab + ApproveAction
│   ├── eval.go        # EvalTab + DiffView
│   ├── audit.go       # AuditTab (live stream)
│   ├── domains.go     # DomainsTab + DomainShow
│   └── dashboard.go   # DashboardTab (cards + sparkline)
└── components/
    ├── table.go    # 通用表格（header + rows + selected）
    ├── detail.go   # 详情面板（key/value）
    └── help.go     # ? 帮助浮层
```

每个 tab 实现 `tea.Model` 接口（`Init / Update / View`），root 在 tab 切换时调用对应的 `Update`。

---

## 9. 与 CLI 命令的集成

TUI 内部不重新实现业务逻辑，而是调用现有 cobra 命令或 `internal/client`：

```text
TUI 按键 ──► action handler ──► cobra command / client call ──► 结果写回 TUI
```

示例：

| TUI 动作 | 底层调用 |
|---|---|
| Sessions tab 按 `R` rerun | `hnsx session rerun <id>` 等价的 client 调用 |
| Approvals tab 按 `a` | `hnsx approval approve <id>` |
| Domains tab 按 `R` trigger | `hnsx session trigger --domain <id>` + 弹窗输入 trigger |
| Dashboard tab 刷新 | `hnsx session list --json` + 聚合计算 |

这样保证 **TUI 与命令式 CLI 共用同一份业务逻辑**，不会漂移。

---

## 10. 实现分期（RoadMap）

### Phase T-1：入口改造 + 核心骨架

- 修改 `hnsx` 无参数行为：TTY 下启动 TUI，非 TTY 下保持 help
- 移除 `hnsx tui` 子命令；保留 `--no-tui` / `HNSX_NO_TUI`
- 引入 bubbletea + bubbles + lipgloss
- Root Model + tab bar + header + footer + status bar
- Theme + 全局 KeyMap + 帮助浮层
- SessionsTab 占位（list + refresh）
- 验收：`hnsx` 进入 → 看到 tab bar / header / 空 sessions tab / footer

### Phase T-2：Sessions + Traces

- SessionsTab 完整：list 2s 轮询 + `enter` 进入 tail 视图（SSE 流）
- TracesTab：list + `enter` 看 observation 树
- 验收：能实时 tail 一个 session，能浏览 trace 详情

### Phase T-3：Approvals + Eval

- ApprovalsTab：pending 列表 + 一键 approve/reject（reject 弹 reason 输入）
- EvalTab：set/run 列表 + `r` 启动 + `d` diff（多选两个 run）
- 验收：能在 TUI 里完成一次 approve + 一次 eval diff

### Phase T-4：Audit + Domains + Dashboard

- AuditTab：live stream，每 2s 刷，filter by actor/resource
- DomainsTab：list + `enter` 看 spec + `R` trigger session
- DashboardTab：数字卡片 + 24h cost sparkline（用 observability 那种最小 sparkline 渲染）
- 验收：七个 tab 全部可用

### Phase T-5：打磨（可选）

- 错误重试 + 离线提示
- `--record <file>` 回放 + 分享
- mouse 支持（bubbletea v1 支持）
- 自适应暗色

### Phase T-6：Command Mode（`/command`）

把 `/` 从"当前 tab 内过滤"升级为全局 **命令模式入口**。按 `/` 在底部弹出 **命令面板（Command Palette）**：命令输入行 + 可筛选的命令列表，输入 `/session <id>`、`/approve <id>` 等直接跳转或执行动作，降低纯快捷键的学习成本。

**命令词表**：

| 命令 | 行为 |
|---|---|
| `/session <id>` | 切到 Sessions tab，选中并打开 tail |
| `/domain <id>` | 切到 Domains tab，定位 domain |
| `/trace <id>` | 切到 Traces tab，打开 trace 详情 |
| `/approve <id>` | 直接通过指定 approval |
| `/reject <id> [reason]` | 直接拒绝指定 approval |
| `/trigger <domain> [json]` | 触发指定 domain 的 session |
| `/filter <text>` | 当前 tab 内过滤 |
| `/refresh` | 刷新当前 tab |
| `/quit` | 退出 TUI |

**实现要点**：

- Root Model 持有 `commandMode bool` + `textinput.Model` + `commandList`
- `/` 进入命令模式，`esc` 取消，`enter` 解析执行
- 命令面板行为：
  - 按 `/` 立即列出全部命令并高亮第一项
  - 输入字符实时过滤命令列表
  - `↑/↓` 在列表中移动，`enter` 选中当前命令并填充到输入行（无参数命令直接执行）
  - 命令名后带空格进入参数输入，列表自动收起
- 命令解析后分两类：
  - **跳转类**：切 tab + 发送 `SelectMsg` / `FilterMsg` 给目标 tab
  - **动作类**：直接调用 `common.Client` 并显示结果
- Footer 在命令模式下切换为命令输入行
- 保留原有单键快捷方式作为高效路径

**验收**：

- `hnsx` 进入 TUI → 按 `/` → 看到命令列表；继续输入 `se` 过滤到 `session`
- 按 `enter` 选中 `session`，输入 `<id>` 再按 `enter` → 自动切到 Sessions tab 并 tail
- `/approve <id>` → approval 被通过，页面刷新
- `/quit` → 退出 TUI

---

## 11. 测试策略

| 层 | 工具 | 覆盖 |
|---|---|---|
| 入口判断 | `_test.go` | TTY / 非 TTY / `--no-tui` / `HNSX_NO_TUI` |
| Model unit | `teatest`（bubbletea 官方测试包） | Update 状态机正确性 |
| View snapshot | `teatest` golden files | 不破坏现有 UI |
| Data fetcher | 普通 `_test.go` + httptest | poll / SSE / 错误路径 |
| Smoke | `scripts/smoke-tui.sh` | 进 TUI、模拟 `q` 退出、断言无 panic |

`teatest` 是 bubbletea v0.25+ 官方测试包，可以驱动 Model、注入消息、断言最终 View。每个 tab 至少 3 个测试：初始化 / 普通按键 / 错误路径。

---

## 12. 技术选型

| 维度 | 选择 | 理由 |
|---|---|---|
| TUI 框架 | `bubbletea` v1 | Elm 架构，单 goroutine，可测试 |
| 组件库 | `bubbles` v1 | 官方 list / table / viewport / spinner |
| 样式 | `lipgloss` v1 | 跟 Console 同样的颜色系统 |
| 终端检测 | `termenv`（lipgloss 内置） | 颜色能力自动降级 |
| HTTP 客户端 | 复用 `internal/client` | 不重写协议 |
| SSE | `internal/client.SessionEvents` 已实现 | 拿到 channel 转 tea.Cmd |
| 颜色定义 | 跟 `observability/src/tokens/morandi.css` 对齐 | 多端统一 |

---

## 13. 风险与对冲

| 风险 | 影响 | 对冲 |
|---|---|---|
| 用户习惯 `hnsx` 无参打 help | 进入 TUI 意外 | 首次启动显示"按 q 退出，--no-tui 永久禁用"提示；支持 `HNSX_NO_TUI` |
| bubbletea v1 与 v0 行为差异 | API 漂移 | 锁版本，文档固定在 v1.x |
| Tab 切换频繁触发数据请求 | 闪烁 / 抖动 | 加 200ms debounce，切换不立即发请求 |
| SSE 在窄窗口下刷屏 | 体验差 | 限速（每秒最多 N 行）+ 自动滚动到底 |
| Server 不可达 | 黑屏 | Status bar 红字 + 重试按钮 `R` |
| 终端过窄（< 80 列） | 布局错位 | 检测宽度，< 80 列隐藏 ID 列 |
| 大量 trace 拉取慢 | 阻塞 UI | 后台 goroutine + progress 提示 |

---

## 14. 评审 Checklist

请你拍板：

- [ ] **默认入口**：`hnsx` 无参数进 TUI 是否可接受？还是保留 help 作为默认？
- [ ] **Tab 列表**：7 个 tab 是否合适？（Sessions/Traces/Approvals/Eval/Audit/Domains/Dashboard）
- [ ] **键盘绑定**：单字母绑定是否够直观？是否要 vim 风格（j/k/h/l）？
- [ ] **刷新频率**：2s 轮询是否合适？还是 5s 更不打扰？
- [ ] **配色**：morandi 调色板是否需要先在终端里试一下？
- [ ] **TUI vs Console 边界**：Console 已经 16 页，TUI 做"轻量版"还是"全部"？
- [ ] **第一个 commit 的范围**：T-1 入口改造 + 骨架，OK 还是想直接 T-1+T-2？
- [ ] **多 server profile**：v1.1 要不要支持？还是 v1.2 再说？
- [ ] **mouse 支持**：要不要一开始就开？

---

*本文档评审通过后，我会按 Phase T-1 → T-4 顺序实现，每个 Phase 一个 commit。*
*任何调整建议直接写在评审 Checklist 下，我据此改完再开工。*
