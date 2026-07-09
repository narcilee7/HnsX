# @hnsx/observability

> 可观测前端组件库

为 HnsX 控制台量身打造的"运维/审计"领域组件库，覆盖 KPI 卡、时间序列预览、token/latency 图表、状态分布、活动热力图、Agent 流向、Grafana 嵌入、Span 列表等可观测场景。

> 设计原则：**不自研**复杂可视化算法，全部基于业界成熟库（recharts、react-sparklines、react-calendar-heatmap）；自研集中在壳层、配色、主题、交互。

---

## 安装

包已 workspace 内嵌，无需发版：

```json
{
  "dependencies": {
    "@hnsx/observability": "file:../observability"
  }
}
```

注入 CSS tokens（**必须**，否则颜色变量找不到）：

```ts
// src/main.tsx 或入口 CSS
import '@hnsx/observability/tokens/morandi.css'
```

> 完整 shadcn 体系内：直接 `import` 即可，宿主 CSS 会自动级联 `var(--chart-1..5)`、`var(--success/warning/danger/info)`。

---

## 快速开始

```tsx
import {
  MetricCard,
  TrendIndicator,
  Sparkline,
  TokenUsageChart,
  StatusDonut,
} from '@hnsx/observability'

export function SessionsOverview() {
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <MetricCard
        label="今日 Session"
        value={1247}
        caption="对比昨日"
        trend={<TrendIndicator value={1247} previous={1180} />}
        footer={<Sparkline data={[40, 38, 45, 60, 55, 70, 90, 88, 95]} variant="chart-1" />}
      />
      <TokenUsageChart
        data={[
          { label: '00:00', input: 120, output: 80 },
          { label: '06:00', input: 480, output: 320 },
        ]}
      />
      <StatusDonut
        centerLabel="Sessions"
        data={[
          { label: 'completed', value: 1089, variant: 'success' },
          { label: 'failed', value: 24, variant: 'danger' },
        ]}
      />
    </div>
  )
}
```

---

## 组件清单

### Primitives（无依赖的薄壳组件）

#### `<MetricCard>`

KPI 卡。左色条 + 标题 + 主数值 + trend + 可选 sparkline slot。

```tsx
<MetricCard
  label="今日 Session"             // 标题
  value={1247}                     // string | number | ReactNode
  formatValue={(n) => n.toLocaleString()}  // 仅当 value 是 number 时生效
  unit="USD"                       // 单位
  caption="对比昨日"               // 副标题
  trend={<TrendIndicator ... />}   // 右上角装饰
  footer={<Sparkline ... />}       // 底部装饰
  onClick={() => navigate('/sessions')}  // 传了变可点击 + hover 抬升
/>
```

#### `<Sparkline>`

紧凑时序预览。基于 `react-sparklines`，不拉 recharts ResponsiveContainer。

```tsx
<Sparkline
  data={[1, 2, 3, 5, 8, 13, 21]}
  variant="chart-1"   // chart-1..5 | success | warning | danger | info
  kind="area"         // 'area' | 'line'
  height={36}
  width="100%"
  endDot={false}
/>
```

#### `<TrendIndicator>`

同比/环比小标识。`invertColor` 用于"错误率下降是好事"这类反向指标。

```tsx
<TrendIndicator value={0.42} previous={0.48} invertColor />  // 红色 ↓（变好）
<TrendIndicator value={1247} previous={1180} />              // 绿色 ↑（变好）
```

### Charts（recharts 封装 + 专门库）

#### `<TokenUsageChart>`

Token 用量趋势（input/output stacked area）。

```tsx
<TokenUsageChart
  data={[
    { label: '00:00', input: 120, output: 80 },
    { label: '06:00', input: 480, output: 320 },
  ]}
  loading={false}      // 显示骨架
  hideHeader={false}   // playground 嵌入时设 true
/>
```

`data` 还可以只传 `{ label, total }`，组件自动 fallback 到单系列。

#### `<LatencyHistogram>`

延迟分布直方图，按 p95 阈值自动染色（warning / danger）。

```tsx
<LatencyHistogram
  data={[
    { label: '<100ms', count: 1240 },
    { label: '100-300ms', count: 880 },
    { label: '>3s', count: 38 },
  ]}
  loading={false}
/>
```

#### `<StatusDonut>`

状态占比环形图。扇区按值降序，中心数值 + 内嵌图例。

```tsx
<StatusDonut
  centerLabel="Sessions"
  centerValue={1247}            // 不传则显示总和
  data={[
    { label: 'completed', value: 1089, variant: 'success' },
    { label: 'running', value: 96, variant: 'info' },
    { label: 'paused', value: 38, variant: 'warning' },
    { label: 'failed', value: 24, variant: 'danger' },
  ]}
/>
```

#### `<ActivityHeatmap>`

GitHub 风格的日活热力图。基于 `react-calendar-heatmap` + `rectRender` 接管 fill。

```tsx
<ActivityHeatmap
  data={[{ date: '2026-07-08', value: 47 }, ...]}
  cols={26}                  // 显示多少列
  variant="chart-1"          // 颜色槽位
  cap="p95"                  // 分位数后端
  unit="sessions"
/>
```

#### `<AgentFlowDiagram>`

Agent hand-off 流向图。横向 stacked bar，source agent × target agent 矩阵。

```tsx
<AgentFlowDiagram
  data={[
    { source: 'orchestrator', target: 'planner', count: 480, avgMs: 120 },
    { source: 'planner', target: 'coder', count: 180, avgMs: 1200 },
    { source: 'coder', target: 'reviewer', count: 220, avgMs: 540 },
  ]}
/>
```

### States

#### `<ChartSkeleton>` / `<ChartEmpty>`

骨架与空态。所有 chart 组件内部已自动处理（`loading` / 空数据），不需要单独使用。也可以单独调用于自定义位置。

```tsx
<ChartSkeleton shape="line" height={240} />
<ChartSkeleton shape="bars" />
<ChartSkeleton shape="donut" />
<ChartSkeleton shape="heatmap" />
<ChartSkeleton shape="flow" />

<ChartEmpty message="暂无数据" description="等待事件触发后会在这里显示。" />
```

#### `<ChartFrame>`

图表外壳（title / description / actions / loading / empty）。通常不直接用，所有 chart 内部已包。

### Integrations（外部系统嵌入）

#### `<GrafanaEmbed>`

Grafana iframe 嵌入。自动 dark/light 同步，time range 透传。

```tsx
<GrafanaEmbed
  baseUrl="http://localhost:3000"
  dashboardUid="hnsx-overview"
  panelId={2}                               // 可选 — 聚焦到单个 panel
  timeRange={{ kind: 'relative', value: '6h' }}
  theme="auto"                              // 'light' | 'dark' | 'auto'
  vars={{ domain: 'customer-service' }}     // dashboard 模板变量
  height={620}
  solo={true}                               // 用 d-solo URL（去掉 chrome）
  title="Grafana overview"
/>
```

URL 解析：`resolveGrafanaBaseUrl(override?)` 优先级 → override > `VITE_HNSX_GRAFANA_URL` env > null（显示 fallback）

#### `<TraceMiniBar>`

紧凑多 span 时间线，用于 trace 列表每行的快速对比。

```tsx
<TraceMiniBar
  spans={[
    { id: '1', startMs: 0,    endMs: 320,  variant: 'chart-1' },
    { id: '2', startMs: 320,  endMs: 1100, variant: 'chart-2' },
    { id: '3', startMs: 1100, endMs: 1850, variant: 'chart-1' },
  ]}
  height={8}
  totalMs={1850}
/>
```

#### `<SpanList>`

Span 列表视图。嵌套层级 + duration bar + token 提取 + payload 展开。

```tsx
<SpanList
  spans={observations.map(o => ({
    id: o.observationId,
    name: `${o.kind}:${o.stepId ?? o.agentId}`,
    kind: o.kind,
    agentId: o.agentId,
    role: o.role,
    startMs: o.createdAt.getTime(),
    endMs: ...,
    parentId: o.parentId,
    tokens: o.payload.tokens,
    payload: o.payload,
    metadata: o.metadata,
  }))}
  height={520}
  header={<div>Trace 总览</div>}
  defaultExpanded={false}
/>
```

---

## 主题 Tokens

所有颜色通过 CSS 变量绑定，light/dark 由 `:root` 与 `.dark` 切换：

### Categorical（5 色图表系列）

| 变量 | light | dark | 用途 |
|---|---|---|---|
| `--chart-1` | `oklch(0.65 0.07 145)` | `oklch(0.74 0.07 145)` | muted sage — primary metric |
| `--chart-2` | `oklch(0.66 0.07 20)` | `oklch(0.76 0.07 20)` | dusty rose — secondary |
| `--chart-3` | `oklch(0.58 0.05 250)` | `oklch(0.72 0.05 250)` | muted slate — neutral |
| `--chart-4` | `oklch(0.74 0.09 75)` | `oklch(0.80 0.09 75)` | muted ochre — attention |
| `--chart-5` | `oklch(0.55 0.03 70)` | `oklch(0.72 0.03 70)` | warm taupe — baseline |

### Status（保留语义色，绝不充当系列）

| 变量 | 用途 |
|---|---|
| `--success` / `--success-text` / `--success-soft` | good |
| `--warning` / `--warning-text` / `--warning-soft` | warning |
| `--danger` / `--danger-text` / `--danger-soft` | critical |
| `--info` / `--info-text` / `--info-soft` | informational |

### Chrome

| 变量 | 用途 |
|---|---|
| `--chart-text-primary` / `-secondary` / `-muted` | 文字层级 |
| `--chart-grid` / `--chart-baseline` | 网格线 / 轴线 |
| `--chart-tooltip-bg` / `-border` | tooltip 背景 |

### 类型化 palette 常量

```ts
import { CHART_SLOTS, STATUS_TOKENS, chartVar, statusVar, slotFor } from '@hnsx/observability'

CHART_SLOTS  // ['chart-1', 'chart-2', 'chart-3', 'chart-4', 'chart-5']
STATUS_TOKENS // ['success', 'warning', 'danger', 'info']

chartVar('chart-1')  // 'var(--chart-1)'
statusVar('danger')  // 'var(--danger)'

// 第 N 个系列取对应槽位（固定顺序、绝不循环）
slotFor(0) // 'chart-1'
slotFor(4) // 'chart-5'
slotFor(5) // 'chart-1'  // 第 6 个系列折叠成 "Other"，不要这样做
```

---

## 设计原则

1. **不自研**：复杂可视化算法全部走现成库（recharts / react-sparklines / react-calendar-heatmap）。自研部分集中在壳层、配色、交互、token 绑定。
2. **Morandi 调色板**：muted / dusty / 低饱和度，长时间阅读不刺眼；状态色保留语义性。
3. **固定顺序**：5 色图表系列固定 1..5，绝不循环。> 5 系列应折叠成 "Other"、small multiples 或组合编码。
4. **永远单 Y 轴**：never dual-axis。
5. **loading/empty 内建**：每个 chart 自动处理 `loading` 与空数据，传 `loading={true}` 显示骨架、`data=[]` 显示空态。
6. **零硬编码颜色**：所有颜色通过 CSS var 绑定，dark mode 一键切换。
7. **可访问性**：sparkline 标 `role="img"`、tooltip 有 aria-label、键盘可达。

---

## 格式化工具

```ts
import {
  formatNumber,      // Intl-aware
  formatCompact,     // 1.2K / 3.4M
  formatPercent,     // 12.3%
  formatDurationMs,  // 872ms / 1.4s / 2.5min
  formatCostUsd,     // $0.0024 / $1.23
  formatTokens,      // 124K tok
} from '@hnsx/observability'
```

所有函数接受 `null | undefined | NaN`，返回 `'—'` 占位符。

---

## 类型参考

每个组件都导出 Props 类型：

```ts
import type {
  MetricCardProps,
  SparklineProps,
  TrendIndicatorProps,
  TokenUsageChartProps,
  TokenUsagePoint,
  LatencyHistogramProps,
  LatencyBucket,
  StatusDonutProps,
  StatusDonutSlice,
  ActivityHeatmapProps,
  ActivityHeatmapCell,
  AgentFlowDiagramProps,
  AgentHandoff,
  TraceMiniBarProps,
  TraceMiniBarSpan,
  SpanListProps,
  SpanListItem,
  GrafanaEmbedProps,
  GrafanaTimeRange,
} from '@hnsx/observability'
```

---

## 开发

```bash
cd observability
pnpm install
pnpm type-check   # tsc --noEmit

# 改完后到 hnsx-console 验证
cd ../hnsx-console
pnpm install --force   # 刷新 file: 依赖
pnpm type-check
pnpm build
pnpm dev
# 访问 http://localhost:5173/playground
```

---

## License

见仓库根目录 LICENSE。
