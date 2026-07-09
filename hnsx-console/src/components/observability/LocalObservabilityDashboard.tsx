import {
  MetricCard,
  TrendIndicator,
  Sparkline,
  TokenUsageChart,
  LatencyHistogram,
  StatusDonut,
  ActivityHeatmap,
} from '@hnsx/observability'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

/**
 * Observability 页面的"本地备胎"Dashboard。
 * 设计原则：
 *   - Grafana 没起 / 配置缺失 / 想看核心指标摘要时走这里
 *   - 所有数据走 mock —— 真接入时把 generateMock() 替换成 useMetrics() 即可
 *   - 跟 Grafana 视图共享同一套 Morandi 主题色
 *   - 信息密度对标 Grafana 顶层 dashboard
 */
export function LocalObservabilityDashboard() {
  const kpi = generateMockKpi()
  const tokenSeries = generateMockTokenSeries(24)
  const statusSlices = generateMockStatusSlices()
  const latencyBuckets = generateMockLatencyBuckets()
  const heatmap = generateMockHeatmap(26 * 7)

  return (
    <div className="space-y-4">
      {/* KPI row */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          label="今日 Session"
          value={kpi.sessionsToday.value}
          caption="对比昨日"
          trend={<TrendIndicator value={kpi.sessionsToday.value} previous={kpi.sessionsToday.previous} />}
          footer={<Sparkline data={kpi.sessionsToday.sparkline} variant="chart-1" />}
        />
        <MetricCard
          label="总成本"
          value={kpi.costToday.value}
          formatValue={(n) => `$${n.toFixed(2)}`}
          unit="USD"
          caption="本周"
          trend={<TrendIndicator value={kpi.costToday.value} previous={kpi.costToday.previous} invertColor />}
          footer={<Sparkline data={kpi.costToday.sparkline} variant="chart-2" />}
        />
        <MetricCard
          label="Token 消耗"
          value={kpi.tokensToday.value}
          formatValue={(n) => `${(n / 1_000_000).toFixed(2)}`}
          unit="M"
          caption="本周"
          trend={<TrendIndicator value={kpi.tokensToday.value} previous={kpi.tokensToday.previous} />}
          footer={<Sparkline data={kpi.tokensToday.sparkline} variant="chart-3" />}
        />
        <MetricCard
          label="错误率"
          value={kpi.errorRate.value}
          formatValue={(n) => `${(n * 100).toFixed(2)}%`}
          caption="过去 24h"
          trend={<TrendIndicator value={kpi.errorRate.value} previous={kpi.errorRate.previous} invertColor />}
          footer={<Sparkline data={kpi.errorRate.sparkline} variant={kpi.errorRate.value > 0.02 ? 'danger' : 'success'} />}
        />
      </div>

      {/* 趋势 + 状态 */}
      <div className="grid gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <TokenUsageChart data={tokenSeries} />
        </div>
        <StatusDonut
          centerLabel="Sessions"
          data={statusSlices}
        />
      </div>

      {/* 延迟 + 活动 */}
      <div className="grid gap-4 lg:grid-cols-2">
        <LatencyHistogram data={latencyBuckets} />
        <ActivityHeatmap data={heatmap} unit="sessions" />
      </div>

      {/* Tool topN + Agent 流向 */}
      <div className="grid gap-4 lg:grid-cols-2">
        <TopToolsCard />
        <RecentAlertsCard />
      </div>
    </div>
  )
}

function TopToolsCard() {
  const tools = [
    { name: 'web_search', calls: 1842, p95: 1240 },
    { name: 'code_run', calls: 932, p95: 8800 },
    { name: 'file_read', calls: 711, p95: 180 },
    { name: 'sql_query', calls: 480, p95: 920 },
    { name: 'image_gen', calls: 142, p95: 6200 },
  ]
  const max = Math.max(...tools.map((t) => t.calls))
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-sm">Tool 调用 top 5</CardTitle>
        <span className="text-xs text-muted-foreground">本周</span>
      </CardHeader>
      <CardContent className="space-y-2">
        {tools.map((t) => (
          <div key={t.name} className="flex items-center gap-3">
            <span className="w-28 truncate text-xs font-medium text-[var(--chart-text-primary)]" title={t.name}>
              {t.name}
            </span>
            <div className="relative h-5 flex-1 overflow-hidden rounded bg-[var(--chart-grid)]/40">
              <div
                className="absolute inset-y-0 left-0 rounded bg-[var(--chart-1)]"
                style={{ width: `${(t.calls / max) * 100}%`, opacity: 0.7 }}
              />
              <span className="absolute inset-y-0 right-2 flex items-center text-[10px] tabular-nums text-[var(--chart-text-primary)]">
                {t.calls.toLocaleString()}
              </span>
            </div>
            <span className="w-16 text-right text-[10px] tabular-nums text-[var(--chart-text-muted)]">
              p95 {t.p95}ms
            </span>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

function RecentAlertsCard() {
  const alerts = [
    { severity: 'warning', text: 'cost-burn 超过预算 80%', time: '12 分钟前' },
    { severity: 'danger', text: 'tool_run p95 连续 5 分钟 > 5s', time: '38 分钟前' },
    { severity: 'info', text: '新 Domain "code-review" v0.3.0 发布', time: '1 小时前' },
    { severity: 'warning', text: 'Eval "claude-triage" score 下降 3.2%', time: '2 小时前' },
  ]
  const toneClass = (s: string) =>
    s === 'danger'
      ? 'bg-[var(--danger-soft)] text-[var(--danger-text)]'
      : s === 'warning'
        ? 'bg-[var(--warning-soft)] text-[var(--warning-text)]'
        : 'bg-[var(--info-soft)] text-[var(--info-text)]'
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle className="text-sm">最近告警</CardTitle>
        <span className="text-xs text-muted-foreground">policy / cost / eval</span>
      </CardHeader>
      <CardContent className="space-y-2">
        {alerts.map((a, i) => (
          <div key={i} className="flex items-center gap-3">
            <span className={['inline-flex h-5 items-center rounded px-2 text-[10px] font-medium uppercase tracking-wide', toneClass(a.severity)].join(' ')}>
              {a.severity}
            </span>
            <span className="flex-1 truncate text-xs text-[var(--chart-text-primary)]">{a.text}</span>
            <span className="text-[10px] text-[var(--chart-text-muted)]">{a.time}</span>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

// ----------------- mock generators -----------------

function generateMockKpi() {
  return {
    sessionsToday: {
      value: 1247,
      previous: 1180,
      sparkline: [40, 38, 45, 60, 55, 70, 90, 88, 95, 110, 105, 124],
    },
    costToday: {
      value: 487.32,
      previous: 521.18,
      sparkline: [62, 58, 65, 70, 68, 72, 75, 71, 68, 65, 62, 60],
    },
    tokensToday: {
      value: 3.4 * 1_000_000,
      previous: 3.1 * 1_000_000,
      sparkline: [1.2, 1.4, 1.5, 1.8, 2.0, 2.3, 2.5, 2.8, 3.0, 3.2, 3.3, 3.4].map((n) => n * 1_000_000),
    },
    errorRate: {
      value: 0.023,
      previous: 0.018,
      sparkline: [0.01, 0.012, 0.014, 0.016, 0.019, 0.022, 0.025, 0.024, 0.023, 0.022, 0.024, 0.023],
    },
  }
}

function generateMockTokenSeries(hours: number) {
  const now = new Date()
  const out: { label: string; input: number; output: number }[] = []
  for (let i = hours - 1; i >= 0; i--) {
    const d = new Date(now.getTime() - i * 3600_000)
    const base = 200 + Math.sin((hours - i) / 3) * 80 + Math.random() * 60
    out.push({
      label: `${String(d.getHours()).padStart(2, '0')}:00`,
      input: Math.round(base * 1.6),
      output: Math.round(base),
    })
  }
  return out
}

function generateMockStatusSlices() {
  return [
    { label: 'completed', value: 1089, variant: 'success' as const },
    { label: 'running', value: 96, variant: 'info' as const },
    { label: 'paused', value: 38, variant: 'warning' as const },
    { label: 'failed', value: 24, variant: 'danger' as const },
  ]
}

function generateMockLatencyBuckets() {
  return [
    { label: '<100ms', count: 1240 },
    { label: '100-300ms', count: 880 },
    { label: '300ms-1s', count: 320 },
    { label: '1-3s', count: 140 },
    { label: '>3s', count: 38 },
  ]
}

function generateMockHeatmap(days: number) {
  const out: { date: string; value: number }[] = []
  const last = new Date()
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(last)
    d.setDate(d.getDate() - i)
    // 周末少、工作日多
    const dow = d.getDay()
    const weekend = dow === 0 || dow === 6
    const base = weekend ? 5 : 30
    const noise = Math.random() * (weekend ? 8 : 35)
    out.push({ date: d.toISOString().slice(0, 10), value: Math.round(base + noise) })
  }
  return out
}