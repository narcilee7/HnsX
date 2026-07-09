import { MetricCard } from '../primitives/MetricCard'
import { TrendIndicator } from '../primitives/TrendIndicator'
import { Sparkline } from '../primitives/Sparkline'
import { TokenUsageChart } from '../charts/TokenUsageChart'
import { LatencyHistogram } from '../charts/LatencyHistogram'
import { StatusDonut } from '../charts/StatusDonut'
import { ActivityHeatmap } from '../charts/ActivityHeatmap'
import { AgentFlowDiagram } from '../charts/AgentFlowDiagram'
import { formatCostUsd, formatDurationMs, formatTokens } from '../lib/utils'

/**
 * 一站式验收页 — 把所有组件用 mock 数据渲染一遍。
 * 宿主可以单独 <ObservabilityPlayground /> 嵌入或路由访问。
 */
export default function ObservabilityPlayground() {
  return (
    <div className="min-h-screen w-full bg-[var(--background)] p-8 text-[var(--chart-text-primary)]">
      <header className="mb-8">
        <h1 className="text-2xl font-semibold">@hnsx/observability · Playground</h1>
        <p className="mt-1 text-sm text-[var(--chart-text-muted)]">
          Morandi 调色板 · 浅色优先 · chart-1..5 + status 四色。所有组件消费同一份 token，
          dark mode 通过 <code className="rounded bg-[var(--chart-grid)]/60 px-1">.dark</code> 一键切换。
        </p>
      </header>

      <section className="mb-10 space-y-3">
        <h2 className="text-sm font-semibold uppercase tracking-wider text-[var(--chart-text-muted)]">
          1 · Primitives
        </h2>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <MetricCard
            label="今日 Session"
            value={1247}
            caption="对比昨日"
            trend={<TrendIndicator value={1247} previous={1180} />}
            footer={<Sparkline data={[40, 38, 45, 60, 55, 70, 90, 88, 95]} variant="chart-1" />}
          />
          <MetricCard
            label="平均成本"
            value={0.42}
            formatValue={(n: number) => `$${n.toFixed(2)}`}
            unit="/ session"
            caption="对比上周"
            trend={<TrendIndicator value={0.42} previous={0.48} invertColor />}
            footer={<Sparkline data={[0.5, 0.48, 0.45, 0.46, 0.44, 0.42, 0.42]} variant="success" />}
          />
          <MetricCard
            label="总 Token"
            value={3.4}
            unit="M"
            formatValue={(n: number) => n.toFixed(1)}
            caption="本周"
            trend={<TrendIndicator value={3.4} previous={3.1} />}
            footer={<Sparkline data={[1, 2, 1.5, 3, 2.5, 3.2, 3.4]} variant="chart-2" />}
          />
          <MetricCard
            label="错误率"
            value={0.023}
            formatValue={(n: number) => `${(n * 100).toFixed(1)}%`}
            caption="过去 24h"
            trend={<TrendIndicator value={0.023} previous={0.018} invertColor />}
            footer={<Sparkline data={[0.01, 0.015, 0.02, 0.025, 0.022, 0.024, 0.023]} variant="danger" />}
          />
        </div>
      </section>

      <section className="mb-10 grid gap-4 lg:grid-cols-2">
        <h2 className="col-span-full text-sm font-semibold uppercase tracking-wider text-[var(--chart-text-muted)]">
          2 · Token & Latency
        </h2>
        <TokenUsageChart
          data={[
            { label: '00:00', input: 120, output: 80 },
            { label: '03:00', input: 90, output: 70 },
            { label: '06:00', input: 200, output: 140 },
            { label: '09:00', input: 480, output: 320 },
            { label: '12:00', input: 620, output: 410 },
            { label: '15:00', input: 540, output: 360 },
            { label: '18:00', input: 380, output: 260 },
            { label: '21:00', input: 290, output: 180 },
          ]}
        />
        <LatencyHistogram
          data={[
            { label: '<100ms', count: 1240 },
            { label: '100-300ms', count: 880 },
            { label: '300ms-1s', count: 320 },
            { label: '1-3s', count: 140 },
            { label: '>3s', count: 38 },
          ]}
        />
      </section>

      <section className="mb-10 grid gap-4 lg:grid-cols-2">
        <h2 className="col-span-full text-sm font-semibold uppercase tracking-wider text-[var(--chart-text-muted)]">
          3 · Distribution & Activity
        </h2>
        <StatusDonut
          centerLabel="Sessions"
          centerValue={1247}
          data={[
            { label: 'completed', value: 1089, variant: 'success' },
            { label: 'running', value: 96, variant: 'info' },
            { label: 'paused', value: 38, variant: 'warning' },
            { label: 'failed', value: 24, variant: 'danger' },
          ]}
        />
        <ActivityHeatmap
          unit="sessions"
          data={generateHeatmapMock(26 * 7, () => Math.floor(Math.random() * 50))}
        />
      </section>

      <section className="mb-10">
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-[var(--chart-text-muted)]">
          4 · Agent Flow
        </h2>
        <AgentFlowDiagram
          data={[
            { source: 'orchestrator', target: 'planner', count: 480, avgMs: 120 },
            { source: 'orchestrator', target: 'retriever', count: 312, avgMs: 240 },
            { source: 'orchestrator', target: 'reviewer', count: 96, avgMs: 380 },
            { source: 'planner', target: 'retriever', count: 240, avgMs: 200 },
            { source: 'planner', target: 'coder', count: 180, avgMs: 1200 },
            { source: 'retriever', target: 'coder', count: 156, avgMs: 90 },
            { source: 'coder', target: 'reviewer', count: 220, avgMs: 540 },
            { source: 'reviewer', target: 'orchestrator', count: 280, avgMs: 60 },
            { source: 'reviewer', target: 'coder', count: 38, avgMs: 880 },
          ]}
        />
      </section>

      <section className="mb-10 grid gap-4 lg:grid-cols-3">
        <h2 className="col-span-full text-sm font-semibold uppercase tracking-wider text-[var(--chart-text-muted)]">
          5 · Loading / Empty 状态
        </h2>
        <TokenUsageChart data={[]} loading />
        <LatencyHistogram data={[]} />
        <StatusDonut data={[]} />
        <ActivityHeatmap data={[]} unit="sessions" />
        <AgentFlowDiagram data={[]} loading />
        <MetricCard label="Loading KPI" value={0} formatValue={() => '—'} caption="加载中" />
      </section>

      <footer className="border-t border-[var(--chart-grid)] pt-6 text-xs text-[var(--chart-text-muted)]">
        <div className="flex flex-wrap gap-6">
          <span>formatTokens · {formatTokens(124500)}</span>
          <span>formatDurationMs · {formatDurationMs(872)}</span>
          <span>formatCostUsd · {formatCostUsd(0.00237)}</span>
        </div>
      </footer>
    </div>
  )
}

function generateHeatmapMock(days: number, gen: () => number): { date: string; value: number }[] {
  const out: { date: string; value: number }[] = []
  const last = new Date()
  for (let i = days - 1; i >= 0; i--) {
    const d = new Date(last)
    d.setDate(d.getDate() - i)
    out.push({ date: d.toISOString().slice(0, 10), value: gen() })
  }
  return out
}