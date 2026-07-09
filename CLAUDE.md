# CLAUDE.md

> 给 AI 编程助手（Claude Code 等）的 HnsX 工作指南

---

## 项目定位

**HnsX = Harness as a Service**。把 Claude Code / Codex / Ollama 等 SOTA Coding Agent 当作运行时，对外提供 Harness 编排能力（SystemPrompt、Skills、Rules、Policy、Sandbox），并配套评估、可观测、审计。

> 不要把 HnsX 理解成"又一个 agent 平台"。**HnsX 不造 agent 底座**，只造 Harness 约束层 + 控制面 + 运维控制台。底层 agent 由 Claude Code / Codex / OpenAI / Anthropic 等提供。

完整愿景见 `docs/vision.md`、产品形态见 `docs/web-console-design/整体设计.md`、API 见 `docs/server-design/api-design.md`。

---

## 仓库结构

```
HnsX/
├── docs/                            设计文档
│   ├── vision.md                    项目愿景
│   ├── tech_overview.md             技术总览 + roadmap
│   ├── web-console-design/          Console 整体设计
│   ├── server-design/               Server API / tablebase 设计
│   ├── know-how/                    我们如何建模 / 编排 / 观测 / 评测
│   └── project_management/          周会 / TODO 跟踪
├── proto/                           Protobuf — API 单一真相源
│   └── hnsx/v1/*.proto              domain / control_plane / observation / runtime
├── hnsx-server/                     Go 后端（CLI + 控制面 + Runtime）
├── hnsx-core/                       Go 核心库
├── sdk/node/                        Node/TS SDK（protobuf 生成的类型）
├── observability/                   @hnsx/observability 前端组件库 ⭐
├── hnsx-console/                    React 19 + Vite + Shadcn 控制台 ⭐
├── example-domains/                 示例 DomainSpec YAML
├── bin/                             构建产物
├── scripts/                         build.sh / test.sh / smoke.sh
├── deployments/local/               docker-compose（Postgres + Tempo + Grafana）
└── Makefile                         顶层入口
```

⭐ = 本仓库当前最活跃的前端两条线。

---

## 前端工作约定

### 工作目录区分

- **`observability/`** — 通用可观测组件库，独立包。可以单独 type-check。
  - `pnpm install`
  - `pnpm type-check`
  - 修改后必须到 `hnsx-console` 跑 `pnpm install --force` 才能在 host 看到最新代码（因为 `file:` 依赖被复制到 virtual store）。
- **`hnsx-console/`** — 控制台，业务页面 + 整合。
  - `pnpm install --force`（确保拿到最新 `observability`）
  - `pnpm dev` / `pnpm type-check` / `pnpm build`

### Tech stack

| 层级 | 技术 |
|---|---|
| 框架 | React 19 + TypeScript + Vite |
| 路由 | React Router v7 |
| 状态 | Zustand（全局）+ TanStack Query（服务端） |
| UI | Tailwind + Shadcn 风格 + Radix UI |
| 图表 | `@hnsx/observability`（自研 + recharts + react-sparklines + react-calendar-heatmap） |
| 编辑器 | Monaco |
| 协议 | REST + SSE（详见 `docs/server-design/api-design.md`） |

### 不自研原则（强约束）

| 需要 | 用现成的 | 不要 |
|---|---|---|
| sparkline | `react-sparklines` | 自己写 recharts ResponsiveContainer |
| calendar heatmap | `react-calendar-heatmap` | 自己写 grid + 分位数 |
| 完整图表 | recharts / Tremor | 从 SVG 手画 |
| schema 校验 | Zod | 手写 if/else |
| 表单 | React Hook Form | 自己管 onChange |
| icon | lucide-react | 自己画 SVG |
| Toast | sonner | 自己实现 |

只在以下场景自研：**壳层 / 配色 / 主题 / token 绑定 / 业务逻辑**。

### 颜色主题约定

所有颜色**必须**通过 CSS 变量绑定，绝不硬编码：

```tsx
// ✅ 正确
<rect fill="var(--chart-1)" />
<span className="text-[var(--chart-text-primary)]">...</span>

// ❌ 错误
<rect fill="#7A9E7E" />
<span className="text-gray-900">...</span>
```

主题：`observability/src/tokens/morandi.css` 是 Morandi 浅色优先调色板。  
5 色图表系列固定 1..5，> 5 系列应折叠成 "Other" 或 small multiples。  
状态色（success/warning/danger/info）**保留**给语义场景，**绝不**充当"系列 4"。

### 文件引用规则

- `observability/src` 内部：用相对路径（`../lib/utils`），不要用 `@/` 别名。包要保证独立可消费。
- `hnsx-console/src` 内部：用 `@/` 别名（`@/components/...`、`@/lib/utils`），由 tsconfig + vite 提供。

### 类型 shim

第三方无 TS 类型时（`react-sparklines`、`react-calendar-heatmap`）：
- 在 `observability/src/types/shims.d.ts` 写一份（包独立消费时用）
- 在 `hnsx-console/src/types/observability-shims.d.ts` 镜像一份（host tsc 编译观测包源码时不读包内 tsconfig）
- 两份保持同步

---

## 常见任务速查

### 加一个新组件到 observability 包

1. 在 `observability/src/charts/` 或 `primitives/` 下创建 `Foo.tsx`，**用相对 import**
2. 导出 props 类型
3. 加进 `observability/src/index.ts` barrel
4. 在 `observability/src/playground/index.tsx` 加演示
5. 到 `hnsx-console` 跑 `pnpm install --force && pnpm type-check && pnpm build`

### 加一个新页面到 console

1. 在 `hnsx-console/src/pages/FooPage.tsx` 创建
2. 路由加进 `src/App.tsx`
3. 侧边栏导航加进 `src/components/layout/Sidebar.tsx`
4. 复用 `PageHeader` / `Card` / `MetricCard` 等基础组件
5. 数据走 `useFoo` hook（`src/hooks/`）+ `@/api/mappers.ts` 的 ViewModel

### 改主题色

1. 改 `observability/src/tokens/morandi.css` 里的 oklch 值
2. 同步改 `hnsx-console/src/index.css` 里的 `--chart-1..5`（shadcn 层覆盖）
3. 两个文件都改，dev server 刷新看效果

### 后端协议变了

1. 改 `proto/hnsx/v1/*.proto`
2. 跑 `make proto`（buf 生成 Go + TS 类型）
3. 改 `hnsx-server/pkg/api/` 的 handler
4. 改 `hnsx-console/src/api/mappers.ts` 的 ViewModel
5. 跑 `pnpm type-check` 验证

---

## 不要做的事

- ❌ 重新发明 sparkline / heatmap / sankey —— 用现成库
- ❌ 硬编码颜色 —— 全部走 CSS var
- ❌ 在控制台造 agent —— 那是 Claude Code / Codex 的活
- ❌ 把控制台做成低代码 Workflow 编辑器 —— 设计明确说"不是低代码平台"
- ❌ 用 dual Y 轴 —— 永远单 Y 轴
- ❌ 第 9 个系列再生成一个新颜色 —— 折叠成 Other
- ❌ 跳过 type-check —— `pnpm type-check` 必跑
- ❌ 改完 observability 不跑 `pnpm install --force` —— host 看不懂

---

## 测试与验收

```bash
# observability
cd observability && pnpm type-check

# hnsx-console
cd hnsx-console
pnpm install --force
pnpm type-check
pnpm build
pnpm dev
# 验收：http://localhost:5173
# /playground  →  组件一屏验收
# /            →  Dashboard
# /observability  →  Grafana + 本地视图 tab
# /sessions/:id   →  完整 Session 详情
# /traces/:id     →  完整 SpanList
```

---

## 协作规则

- **任务跟踪**：用 `TaskCreate` / `TaskUpdate`。3 步以上的任务都该创建任务。
- **分支**：feat/* → PR → main，commit message 用 Conventional Commits。
- **Commit 末尾**：加 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- **PR 末尾**：加 `🤖 Generated with [Claude Code](https://claude.com/claude-code)`。
- **不要直接 push** 到 main，先 PR。

---

## 知识库（先读再做）

| 文件 | 何时读 |
|---|---|
| `docs/vision.md` | 拿不准产品方向时 |
| `docs/web-console-design/整体设计.md` | 加新页面 / 改交互前 |
| `docs/server-design/api-design.md` | 改 API 或加 endpoint 前 |
| `docs/know-how/我们如何观测Harness与Agent.md` | 改 Observation 模型时 |
| `docs/know-how/我们如何建模Harness.md` | 改 Domain 模型时 |
| `docs/know-how/我们如何编排Agent并集成Harness.md` | 改 Runtime / Workflow 时 |
| `docs/know-how/我们如何评测Harness与Agent.md` | 改 Eval 系统时 |
| `docs/tech_overview.md` | 想了解 roadmap / phase 划分 |
| `observability/README.md` | 加 / 改 observability 组件前 |