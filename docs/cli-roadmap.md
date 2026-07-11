# HnsX CLI RoadMap

> **起草日期：2026-07-11** · **作者：Claude (与 narcilee7 协作)** · **状态：草案 v0.1，待评审**

把 `hnsx` 从 "AI Agent SDK" 重塑为**面向人 / CI 的 Operator CLI**，让任何人不读 README 就能在 30 秒内跑通第一个 Session。

---

## 0. 现状（基线，对照表）

| 项 | 现状 | 来源 |
|---|---|---|
| 命令数 | 4 个顶层（`validate` / `run` / `remote` / `version`），`remote` 下挂 14 个子命令 | `hnsx-server/cmd/hnsx/main.go` (918 行) |
| 框架 | 手写 `switch` + `flag.NewFlagSet`，无 cobra/pflag | 同上 |
| 输出 | 全部 `json.MarshalIndent` 裸输出 | 同上 |
| 生命周期 | **零**（不知道 server 在哪 / DB 在哪 / worker 在哪） | — |
| Onboard | **零**（要 `docker compose up` + `curl`） | `scripts/smoke.sh` |
| 表格 / watch / 流 | **零** | — |
| 安装 / 升级 | **零**（只能 `make build-cli`） | Makefile |
| Console / TUI 启动 | **零**（Console 是独立 dev server） | `hnsx-console/` |
| 文档 | 只有 `printUsage()` 一段 help | `main.go:48` |

**Roadmap 就是从这张基线走到 v1.0 的路径。**

---

## 1. 设计原则（所有版本通用）

| # | 原则 | 含义 |
|---|---|---|
| P1 | **资源导向命名** | `hnsx <verb> <resource>`：`domain list / session trigger / eval run / approval approve / trace show`，类似 `kubectl` / `gh` |
| P2 | **人 / Agent 两种 profile** | `hnsx session list --human` 给表格；`--json` 给 Agent 消费；默认 `--human` |
| P3 | **Output 三态** | `--human`（表格 + 颜色）/ `--json`（line-delimited）/ `--quiet`（只返 ID） |
| P4 | **退出码语义化** | `0` 成功 / `1` 用户错 / `2` 资源错 / `3` 服务器错 / `4` 权限错 / `5` 网络错 — CI 友好 |
| P5 | **配置三层** | flag > env (`HNSX_*`) > config (`~/.config/hnsx/config.yaml`)；server URL / output / log level 全部覆盖 |
| P6 | **secret 不入 env 日志** | secret/policy 命令默写 stderr 流、强制 `--confirm`，exit code 严 |
| P7 | **本地优先 + 自动 fallback** | 不带 server 也能用 `validate` / `format` / `diff` / `init`；带 server 才走远程；fallback 时 warn |
| P8 | **命令可发现** | `hnsx` 无参 → 顶部 5 个常用 + 完整 list；每级 `--help` 给例子，不止给 flag 列表 |
| P9 | **不重新发明协议** | 所有 remote 命令最终走 `client.*`，与 Console / SDK 共用 — 见 §7 |
| P10 | **deprecate 而不是删** | 旧 `hnsx remote <x>` 全 alias 到新命令，至少 2 个 minor 周期 |

---

## 2. 命令词表（v1.0 全景，单源真相）

> 一次性画出来，后续版本逐批点亮。分组对应独立子包 / cobra command tree。

### 2.1 Lifecycle（产品门面）

```text
hnsx up         [flags]   # 启 server + worker + db（可选 tempo/grafana），前台感知 + detach
hnsx down                 # 停
hnsx restart              # down + up
hnsx status               # 谁在跑、端口、health、PID、log 位置
hnsx doctor               # 诊断：env / port / db migration / adapter key / disk
hnsx logs     [-s] [-f]   # 跟日志，-s server | -w worker | -d db
hnsx reset    [--hard]    # 清 db + 清缓存；--hard 清 example domains
```

### 2.2 Discovery / Onboard（新人 30 秒路径）

```text
hnsx try <domain>                  # 一键跑 example-domains/<x>，自动 up + register + trigger + tail
hnsx init                          # 在当前目录生成 domain.yaml 模板（按业务类型）
hnsx examples                      # 列出 example-domains 画廊（name / description / tags）
hnsx completion (bash|zsh|fish|powershell)
hnsx version                       # version + commit + built + server 版本（若可达）
hnsx update                        # 自更新（v0.5+）
```

### 2.3 Domain（资源）

```text
hnsx domain list     [--all-versions] [--filter tag=...]
hnsx domain show     <id>[@<version>]
hnsx domain register <path>...     # 支持多文件、stdin、--watch
hnsx domain validate <path>...     # = 旧 validate
hnsx domain format   <path>...     # 标准化 yaml（v0.7+）
hnsx domain diff     <a> <b>       # 两版 diff（v0.7+）
hnsx domain delete   <id>[@<version>]
hnsx domain export   <id>          # 导出注册后的 resolved spec
```

### 2.4 Session（资源）

```text
hnsx session list     [--domain <id>] [--state ...] [--since 1h] [--limit N]
hnsx session show     <id>
hnsx session trigger  --domain <id> [--trigger <json|@file>] [--async]
hnsx session cancel   <id>
hnsx session rerun    <id> [--with trigger]
hnsx session tail     <id>         # 实时事件流（彩色）+ Ctrl+C 退出
hnsx session watch    <id>         # TUI-like 终端面板（v0.4+，被 TUI 取代前先做）
hnsx session approve  <id>         # 若 paused 等审批
hnsx session reject   <id> --reason ...
```

### 2.5 Trace / Observation

```text
hnsx trace list    [--session <id>] [--domain <id>] [--since ...] [--cost-min ...]
hnsx trace show    <id>           # 树形展开 observation
hnsx trace export  <id> [--format json|yaml|otlp]
```

### 2.6 Eval

```text
hnsx eval set list
hnsx eval set show  <id>
hnsx eval set create --domain <id> --file <yaml>
hnsx eval set delete <id>
hnsx eval run       <set-id> [--concurrency N] [--baseline <run-id>]
hnsx eval run show  <set-id> <run-id>
hnsx eval run diff  <set-id> <a> <b>
```

### 2.7 Governance（v0.6+）

```text
hnsx policy list / show / apply / delete
hnsx secret list / set / delete / rotate   # 强制 --confirm
hnsx approval list / approve / reject
hnsx audit list [--actor ...] [--resource ...]
```

### 2.8 Surfaces（v0.5+）

```text
hnsx console         # 启 Console dev/prod，打开浏览器
hnsx tui             # 进 TUI（需实现 v0.5+）
hnsx mcp <sub>...    # MCP Server 管理（v0.6+，预留）
```

### 2.9 工具 / 自省

```text
hnsx config  show | get <key> | set <key> <val> | unset <key>
hnsx context list | use | add     # 多 server profile（dev/staging/prod）
hnsx auth    login | status | logout   # 远端 server auth（v0.6+）
hnsx debug   info | bundle       # 收集 debug 信息打包
hnsx plugin  list | install      # 插件预留（v1.0）
```

---

## 3. 版本路线

### v0.3 — "Lifesaver" ⭐ 当前最该做

**主题**：吸收 `scripts/`，消灭 onboarding 摩擦。**第一体感革命。**

**新增**：
- `hnsx up` / `down` / `restart` / `status` / `doctor` / `logs` / `reset`
- `hnsx try <example>` / `hnsx examples` / `hnsx completion`
- 引入 **cobra + pflag** 框架，重构现有 4 个命令
- 引入 `~/.config/hnsx/config.yaml` + env fallback

**Done 标准**：
- 新人 `git clone` 后，**不读 README** 跑 `hnsx try customer-service`，30 秒内看到第一个 Observation
- `scripts/smoke.sh` 90% 步骤变成对 `hnsx up` / `hnsx try` 的调用；保留的 bash 只做断言
- `hnsx doctor` 能识别：DB 没启 / port 被占 / migration 没跑 / adapter key 缺失
- CI 通过：`make test` + `hnsx doctor` + `hnsx try` smoke

**废弃 / alias**：
- `hnsx remote <x>` 全部 alias 到新命令（`hnsx remote sessions list` → `hnsx session list`），warn 一次

**估时**：1 周

---

### v0.4 — "Operator"

**主题**：日常运维手感。Session / Trace / Eval 全部 human-friendly。

**新增**：
- `hnsx session list --human`（表格：ID / Domain / State / Cost / Age）
- `hnsx session tail <id>`：实时事件流（带颜色：`TEXT`/`TOOL`/`ERROR`/`COST`）
- `hnsx session watch <id>`：单 Session TUI-lite（rich-go 渲染 token / cost 进度条）
- `hnsx trace list --since 1h --cost-min 0.01`（query 真正可用）
- `hnsx trace show <id>`：树形 observation，折叠 / 展开、跳转 time
- `hnsx eval run --baseline`：跑完直接 diff
- 全部资源命令支持 `--filter`、`--limit`、`--since`、`--output {human,json,quiet}`

**Done 标准**：
- SRE 在 terminal 里能完成 80% 的日常运维，不打开 Console
- `hnsx session tail <id>` 的可读性高于 `curl -N .../events`
- Eval diff 输出能直接贴 PR comment

**估时**：1.5 周

---

### v0.5 — "Bridge"

**主题**：把 Console / TUI / example-domains 收编为 `hnsx` 的一等子命令。

**新增**：
- `hnsx console`：内嵌启 Vite prod build → 自动 `open http://127.0.0.1:<port>`；支持 `--dev`（Vite dev server）
- `hnsx tui`：启动 bubbletea-based TUI（独立可装 binary 或 subcommand）
- `hnsx examples` 进化为画廊：本地 + GitHub source
- `hnsx update`：自更新到最新稳定版
- shell completion 默认装（首次 `hnsx up` 后探测 shell 提示安装）

**Done 标准**：
- 一次 `hnsx up` 后，`hnsx console` 一键拉起 GUI；`hnsx tui` 一键进终端
- `hnsx examples` 在 Console / TUI / CLI 三处呈现一致

**估时**：1.5 周

---

### v0.6 — "Governance"

**主题**：把控制面的"严肃操作"暴露给 CLI，但比 REST 更安全。

**新增**：
- `hnsx policy list/show/apply/delete`，`apply` 强制 `--dry-run` + `--confirm`
- `hnsx secret set/rotate`：内存中编辑 + 强制 `--confirm` + 不写历史
- `hnsx approval list/approve/reject`，可订阅 SSE → 终端通知
- `hnsx audit list`：流式输出（评估导出 CSV）
- `hnsx auth login`（远端 server 的 OIDC / token）

**Done 标准**：
- 安全/合规同学可以 `hnsx policy apply` + `hnsx audit export` 完成日常审计，无需 Console
- 任何破坏性命令都过 `hnsx doctor` + `--confirm` 双闸

**估时**：1.5 周

---

### v0.7 — "Power"

**主题**：Harness 开发者的高级动作。

**新增**：
- `hnsx domain format`：标准化 yaml（formatter，与 Go `gofmt` 同级）
- `hnsx domain diff <a> <b>`：patch view 高亮（agents/prompts/skills/tools/policy 各自 diff）
- `hnsx session replay <id>`：dry-run 重放（替换 adapter / 修改 trigger）
- `hnsx debug bundle`：收 server log + trace + config → tarball，方便贴 issue
- `hnsx plugin install <url>`：插件机制（external subcommand，参考 `kubectl plugin`）

**Done 标准**：
- Domain 改一版 PR，能 `hnsx domain diff old new` 自动生成 changelog
- 任何报错能 `hnsx debug bundle` 打包上报

**估时**：1 周

---

### v0.8 — "Ship"（预发布）

**主题**：让产品能装、能升级、能让 CI 信任。

**新增**：
- Release artifacts：`brew tap hnsx-io/hnsx`、`curl -sSL hnsx.dev/install.sh | sh`、Docker image、Debian/RPM、npm sidecar
- SBOM + cosign 签名
- 升级检测：`hnsx update --check`
- 文档站：`docs.hnsx.dev/cli/` 自动生成（cobra-doc）
- 完整 `--help` + 30 个示例

**Done 标准**：
- 任意平台 `curl ... | sh` 30 秒装上；`hnsx doctor` 全绿
- 旧 `hnsx remote *` 命令 deprecation 警告 + 跳转提示

**估时**：1 周

---

### v1.0 — "Product"

**主题**：第一次正式 GA。**RoadMap 终点。**

**新增**：
- v1.0 之前的 deprecation 全部落地，旧路径完整移除（保留 alias 至 v1.1）
- 语义化版本承诺 + 1 年 LTS
- 完整 E2E：CLI / TUI / Console / SDK 四端互通
- Telemetry opt-in（`hnsx telemetry off`），隐私友好

**Done 标准**：
- "**`hnsx try <example>`**" 成为 onboarding 的标准口号，写进 README 第一行
- 三端（CLI / TUI / GUI）共享同一份 client + 同一份命令词表
- 任意新人第一小时能跑通 demo + 写一个自己的 Domain

**估时**：收尾 + 文档

---

## 4. 能力矩阵（每版逐格点亮）

| 能力 | v0.3 | v0.4 | v0.5 | v0.6 | v0.7 | v1.0 |
|---|---|---|---|---|---|---|
| `up / down / status / doctor` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `try / examples` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| 资源命令 human 输出 | ⬜ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Session tail / watch | ⬜ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Eval diff | ⬜ | ✅ | ✅ | ✅ | ✅ | ✅ |
| Console / TUI 启动 | ⬜ | ⬜ | ✅ | ✅ | ✅ | ✅ |
| Policy / Secret / Approval | ⬜ | ⬜ | ⬜ | ✅ | ✅ | ✅ |
| Domain format / diff / replay | ⬜ | ⬜ | ⬜ | ⬜ | ✅ | ✅ |
| install / upgrade / signing | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | ✅ |
| 旧 `remote *` 移除 | ⬜ | ⬜ | ⬜ | ⬜ | ⬜ | ✅ |

---

## 5. 迁移计划（不留历史包袱）

| 现状 | 目标 | 何时生效 |
|---|---|---|
| `hnsx remote domains list` | `hnsx domain list` | v0.3 起 alias，warn 一次 |
| `hnsx remote sessions trigger --domain X` | `hnsx session trigger --domain X` | v0.3 |
| `hnsx remote evals run <set>` | `hnsx eval run <set>` | v0.3 |
| `hnsx remote sessions events <id>` | `hnsx session tail <id>` | v0.4 |
| `scripts/smoke.sh` 中 `docker compose up` | `hnsx up` | v0.3 |
| `scripts/e2e/*.sh` | bash 内调 `hnsx up` + `hnsx try` + 断言 | v0.3 |
| 裸 `json.MarshalIndent` 输出 | `--json` 显式 opt-in | v0.3 |
| Makefile `build-cli` | 保留（CI 用）；用户改用 `hnsx update` | v0.8 |

---

## 6. 架构 / 实现要点（避免 RoadMap 飘）

| 决策 | 选择 | 理由 |
|---|---|---|
| CLI 框架 | **cobra + pflag** | 业界标准，gh/kubectl 都用；自动生成 docs |
| 配置 | **viper**（可选）或自写 | cobra 自带；env fallback 一行 |
| 输出渲染 | **text/tabwriter**（human）+ **json-iterator**（json） | 标准库够用 |
| 实时流 | **bubbletea**（TUI）+ **lipgloss**（颜色） | 已规划进 TUI；CLI tail 复用 |
| Client 复用 | **现有 `internal/client/`** 升级，所有子命令走它 | 不重写 API 客户端 |
| 子命令分包 | `cmd/hnsx/<resource>.go`，每个文件 1 个 cobra.Command | 单文件 < 300 行 |
| 错误模型 | 沿用 `client.APIError`，CLI 包一层 `cliError` 加 exit code | 复用 |
| 测试 | `cmd/hnsx/<x>_test.go`（单元）+ `scripts/e2e/`（黑盒） | 不重复 |

**重要**：不在 CLI 层重写 client / 重写协议 — 那不是 v0.3 的工作。

---

## 7. 风险与对冲

| 风险 | 影响 | 对冲 |
|---|---|---|
| `hnsx up` 抽象过深，藏着太多东西 | 排错难 | `hnsx up --verbose` / `hnsx logs -f` 兜底 |
| 引入 cobra 改变 14 个旧命令的行为 | 现有用户脚本炸 | 100% alias + 2 minor 周期保留 |
| TUI 抢 CLI 流量 | 资源浪费 | TUI 内部走 CLI 同一 client，不重写 |
| `scripts/` 改造不彻底 | "脚本感"残留 | v0.3 收尾时强制 `grep -r 'docker compose' scripts/` 必须为零 |
| Console / CLI 命令不同步 | 用户困惑 | 命令词表（§2）作为单源真相，所有 PR 改动两边同步 |

---

## 8. 第一个 Sprint 建议（v0.3 切分）

如果路线图通过，第一个 Sprint 拆成这样：

```text
Sprint 1 (本周) — v0.3 一半
  Day 1-2: cobra 框架接入，重构现有 4 个命令；config / env / output 三态
  Day 3:   hnsx up / down / status / doctor / logs
  Day 4:   hnsx try / examples
  Day 5:   scripts/ 收编 + smoke 全绿 + 文档

Sprint 2 (下周) — v0.3 收尾 + v0.4 起手
  Day 1-2: remote → 新命令的 alias + warn
  Day 3:   v0.3 GA（tag）
  Day 4-5: v0.4 起手 — table 输出 + tail
```

---

## 9. 评审 Checklist

- [ ] 命名（资源导向）是否符合团队习惯？是否需要 `domains` 复数形式？
- [ ] v0.3 "Lifesaver" 的 Done 标准是不是太低/太高？
- [ ] `hnsx up` 是否要默认 detach？是否要 `--foreground`？
- [ ] TUI 与 Console 谁先做？RoadMap 里是 Console → TUI，可否反过来？
- [ ] Governance 子树（policy/secret/approval）v0.6 的优先级是否合理？
- [ ] 迁移计划里 `hnsx remote *` 完全 alias 还是 deprecate-warn？
- [ ] 是否要把 §2 命令词表拆成 `docs/cli/vocabulary.md` 作为独立维护文件？

---