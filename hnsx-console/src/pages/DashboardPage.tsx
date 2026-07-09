import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import {
  MetricCard,
  TrendIndicator,
  Sparkline,
  TokenUsageChart,
  StatusDonut,
  ActivityHeatmap,
  LatencyHistogram,
} from '@hnsx/observability'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/PageHeader'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Activity, AlertTriangle, CheckCircle2, ClipboardList, ShieldCheck } from 'lucide-react'

export default function DashboardPage() {
  const kpi = useMemo(() => generateMockKpi(), [])
  const tokenSeries = useMemo(() => generateMockTokenSeries(24), [])
  const statusSlices = useMemo(() => generateMockStatusSlices(), [])
  const latencyBuckets = useMemo(() => generateMockLatencyBuckets(), [])
  const heatmap = useMemo(() => generateMockHeatmap(26 * 7), [])
  const alerts = useMemo(() => generateMockAlerts(), [])
  const pendingApprovals = useMemo(() => generateMockApprovals(), [])
  const recentSessions = useMemo(() => generateMockRecentSessions(), [])

  return (
    <div className="space-y-6">
      <PageHeader
        title="Dashboard"
        description="HnsX 平台核心指标总览 — 今日 Session / 成本 / Token / 错误率。"
      />

      {/* KPI row */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          label="今日 Session"
          value={kpi.sessionsToday.value}
          caption="对比昨日"
          trend={<TrendIndicator value={kpi.sessionsToday.value} previous={kpi.sessionsToday.previous} />}
          footer={<Sparkline data={kpi.sessionsToday.sparkline} variant="chart-1" />}
          onClick={() => (window.location.href = '/sessions')}
        />
        <MetricCard
          label="总成本"
          value={kpi.costToday.value}
          formatValue={(n) => `$${n.toFixed(2)}`}
          unit="USD"
          caption="本周"
          trend={<TrendIndicator value={kpi.costToday.value} previous={kpi.costToday.previous} invertColor />}
          footer={<Sparkline data={kpi.costToday.sparkline} variant="chart-2" />}
          onClick={() => (window.location.href = '/audit')}
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
          footer={
            <Sparkline
              data={kpi.errorRate.sparkline}
              variant={kpi.errorRate.value > 0.02 ? 'danger' : 'success'}
            />
          }
        />
      </div>

      {/* Trend + Status */}
      <div className="grid gap-4 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <TokenUsageChart data={tokenSeries} />
        </div>
        <StatusDonut centerLabel="Sessions" data={statusSlices} />
      </div>

      {/* Latency + Heatmap */}
      <div className="grid gap-4 lg:grid-cols-2">
        <LatencyHistogram data={latencyBuckets} />
        <ActivityHeatmap data={heatmap} unit="sessions" />
      </div>

      {/* Alerts / Approvals / Recent */}
      <div className="grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="flex items-center gap-2 text-sm">
              <AlertTriangle className="h-4 w-4 text-[var(--warning)]" />
              最近告警
            </CardTitle>
            <Link to="/audit" className={cn(buttonVariants({ variant: 'ghost', size: 'sm' }), 'no-underline text-xs')}>
              查看全部
            </Link>
          </CardHeader>
          <CardContent className="space-y-2">
            {alerts.map((a, i) => (
              <div key={i} className="flex items-center gap-2">
                <span
                  className={cn(
                    'inline-flex h-5 items-center rounded px-2 text-[10px] font-medium uppercase tracking-wide',
                    a.severity === 'danger'
                      ? 'bg-[var(--danger-soft)] text-[var(--danger-text)]'
                      : a.severity === 'warning'
                        ? 'bg-[var(--warning-soft)] text-[var(--warning-text)]'
                        : 'bg-[var(--info-soft)] text-[var(--info-text)]',
                  )}
                >
                  {a.severity}
                </span>
                <span className="flex-1 truncate text-xs">{a.text}</span>
                <span className="text-[10px] text-muted-foreground">{a.time}</span>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="flex items-center gap-2 text-sm">
              <ClipboardList className="h-4 w-4 text-[var(--info)]" />
              待审批 ({pendingApprovals.length})
            </CardTitle>
            <Link to="/approvals" className={cn(buttonVariants({ variant: 'ghost', size: 'sm' }), 'no-underline text-xs')}>
              全部
            </Link>
          </CardHeader>
          <CardContent className="space-y-2">
            {pendingApprovals.length === 0 ? (
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <CheckCircle2 className="h-4 w-4 text-[var(--success)]" />
                当前没有待审批事项
              </div>
            ) : (
              pendingApprovals.slice(0, 4).map((p) => (
                <div key={p.id} className="flex items-center gap-2 text-xs">
                  <ShieldCheck className="h-3.5 w-3.5 text-[var(--info)]" />
                  <span className="font-mono text-[10px] text-muted-foreground">{p.session.slice(0, 12)}</span>
                  <span className="flex-1 truncate">{p.action}</span>
                  <Button size="sm" variant="outline" className="h-6 px-2 text-[10px]">
                    审批
                  </Button>
                </div>
              ))
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="flex items-center gap-2 text-sm">
              <Activity className="h-4 w-4 text-[var(--chart-1)]" />
              最近 Session
            </CardTitle>
            <Link to="/sessions" className={cn(buttonVariants({ variant: 'ghost', size: 'sm' }), 'no-underline text-xs')}>
              全部
            </Link>
          </CardHeader>
          <CardContent className="space-y-2">
            {recentSessions.map((s) => (
              <Link
                key={s.id}
                to={`/sessions/${s.id}`}
                className="flex items-center gap-2 rounded-md px-1 py-0.5 text-xs transition-colors hover:bg-[var(--chart-grid)]/40"
              >
                <span
                  className={cn(
                    'inline-flex h-5 items-center rounded px-1.5 text-[10px] font-medium',
                    s.state === 'completed'
                      ? 'bg-[var(--success-soft)] text-[var(--success-text)]'
                      : s.state === 'failed'
                        ? 'bg-[var(--danger-soft)] text-[var(--danger-text)]'
                        : 'bg-[var(--info-soft)] text-[var(--info-text)]',
                  )}
                >
                  {s.state}
                </span>
                <span className="flex-1 truncate font-mono text-[10px]">{s.id.slice(0, 14)}</span>
                <span className="text-[10px] text-muted-foreground">{s.duration}</span>
              </Link>
            ))}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

// ----------------- mock data generators -----------------

function generateMockKpi() {
  return {
    sessionsToday: {
      value: 1247,
      previous: 1180,
      sparkline: [40, 38, 45, 60, 55, 70, 90, 88, 95, 110, 105, 124, 130, 128],
    },
    costToday: {
      value: 487.32,
      previous: 521.18,
      sparkline: [62, 58, 65, 70, 68, 72, 75, 71, 68, 65, 62, 60, 58, 56],
    },
    tokensToday: {
      value: 3.4 * 1_000_000,
      previous: 3.1 * 1_000_000,
      sparkline: [1.2, 1.4, 1.5, 1.8, 2.0, 2.3, 2.5, 2.8, 3.0, 3.2, 3.3, 3.4, 3.3, 3.4].map((n) => n * 1_000_000),
    },
    errorRate: {
      value: 0.023,
      previous: 0.018,
      sparkline: [0.01, 0.012, 0.014, 0.016, 0.019, 0.022, 0.025, 0.024, 0.023, 0.022, 0.024, 0.023, 0.022, 0.023],
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
    const dow = d.getDay()
    const weekend = dow === 0 || dow === 6
    const base = weekend ? 5 : 30
    const noise = Math.random() * (weekend ? 8 : 35)
    out.push({ date: d.toISOString().slice(0, 10), value: Math.round(base + noise) })
  }
  return out
}

function generateMockAlerts() {
  return [
    { severity: 'warning' as const, text: 'cost-burn 超过预算 80%', time: '12 分钟前' },
    { severity: 'danger' as const, text: 'tool_run p95 连续 5 分钟 > 5s', time: '38 分钟前' },
    { severity: 'info' as const, text: '新 Domain "code-review" v0.3.0 发布', time: '1 小时前' },
    { severity: 'warning' as const, text: 'Eval "claude-triage" score 下降 3.2%', time: '2 小时前' },
  ]
}

function generateMockApprovals() {
  return [
    { id: 'apr-1', session: 'sess_a3f9b2', action: 'tool_call: shell_exec(rm -rf /tmp/build)' },
    { id: 'apr-2', session: 'sess_b1c4d8', action: 'tool_call: http_request(prod-api.example.com)' },
    { id: 'apr-3', session: 'sess_e9f7c1', action: 'agent_handoff: reviewer → coder' },
  ]
}

function generateMockRecentSessions() {
  return [
    { id: 'sess_a3f9b2e7c1', state: 'completed', duration: '4.2s' },
    { id: 'sess_b1c4d8e9f2', state: 'running', duration: '12.7s' },
    { id: 'sess_e9f7c1d3b8', state: 'failed', duration: '1.1s' },
    { id: 'sess_c7a2b5f8e1', state: 'completed', duration: '8.4s' },
    { id: 'sess_d4e6f9a2c7', state: 'completed', duration: '6.1s' },
  ]
}