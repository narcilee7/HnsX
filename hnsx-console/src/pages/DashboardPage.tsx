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
import { useMetrics } from '@/hooks/useMetrics'
import { useSessions } from '@/hooks/useSessions'
import { useApprovals } from '@/hooks/useApprovals'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'

export default function DashboardPage() {
  const { data: metrics } = useMetrics()
  const { data: sessionsData } = useSessions({ limit: 50 })
  const { data: approvalsData } = useApprovals({ status: 'pending', limit: 10 })

  const sessions = sessionsData?.items ?? []
  const pendingApprovals = approvalsData?.items ?? []

  const statusSlices = useMemo(() => {
    const failed = sessions.filter((s) => s.state === 'failed').length
    const running = sessions.filter((s) => s.state === 'running').length
    const paused = sessions.filter((s) => s.state === 'paused').length
    const completed = sessions.filter((s) => s.state === 'completed').length
    return [
      { label: 'completed', value: completed, variant: 'success' as const },
      { label: 'running', value: running, variant: 'info' as const },
      { label: 'paused', value: paused, variant: 'warning' as const },
      { label: 'failed', value: failed, variant: 'danger' as const },
    ].filter((s) => s.value > 0)
  }, [sessions])

  const recentSessions = useMemo(() => {
    return [...sessions]
      .sort((a, b) => (b.startedAt?.getTime() ?? 0) - (a.startedAt?.getTime() ?? 0))
      .slice(0, 5)
  }, [sessions])

  const kpi = useMemo(() => {
    const total = metrics?.total_sessions ?? 0
    const failed = metrics?.failed_sessions ?? 0
    const cost = metrics?.total_cost_usd ?? 0
    const tokens = (metrics?.prompt_tokens ?? 0) + (metrics?.completion_tokens ?? 0)
    const errorRate = total > 0 ? failed / total : 0
    return {
      sessionsToday: {
        value: total,
        previous: Math.max(0, total - Math.round(total * 0.08)),
        sparkline: generateSparkline(total, 14),
      },
      costToday: {
        value: cost,
        previous: Math.max(0, cost * 0.92),
        sparkline: generateSparkline(cost, 14),
      },
      tokensToday: {
        value: tokens,
        previous: Math.max(0, tokens * 0.9),
        sparkline: generateSparkline(tokens, 14),
      },
      errorRate: {
        value: errorRate,
        previous: Math.max(0, errorRate * 0.75),
        sparkline: generateSparkline(errorRate, 14),
      },
    }
  }, [metrics])

  const tokenSeries = useMemo(() => generateMockTokenSeries(24), [])
  const latencyBuckets = useMemo(() => generateMockLatencyBuckets(), [])
  const heatmap = useMemo(() => generateMockHeatmap(26 * 7), [])
  const alerts = useMemo(() => generateMockAlerts(), [])

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
                  <span className="font-mono text-[10px] text-muted-foreground">{p.session_id.slice(0, 12)}</span>
                  <span className="flex-1 truncate">{p.action}</span>
                  <Button size="sm" variant="outline" className="h-6 px-2 text-[10px]" asChild>
                    <Link to={`/sessions/${p.session_id}`}>审批</Link>
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
            {recentSessions.length === 0 ? (
              <div className="text-xs text-muted-foreground">暂无 Session</div>
            ) : (
              recentSessions.map((s) => (
                <Link
                  key={s.id}
                  to={`/sessions/${s.id}`}
                  className="flex items-center gap-2 rounded-md px-1 py-0.5 text-xs transition-colors hover:bg-[var(--chart-grid)]/40"
                >
                  <StatusBadge status={s.state} />
                  <span className="flex-1 truncate font-mono text-[10px]">{s.id.slice(0, 14)}</span>
                  <span className="text-[10px] text-muted-foreground">
                    <Timestamp date={s.startedAt} />
                  </span>
                </Link>
              ))
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

// ----------------- helpers / fallback generators -----------------

function generateSparkline(value: number, points: number): number[] {
  if (value === 0) return Array(points).fill(0)
  const out: number[] = []
  let current = value * 0.4
  for (let i = 0; i < points; i++) {
    const noise = (Math.random() - 0.5) * value * 0.2
    current = Math.max(0, current + noise + (value - current) / (points - i))
    out.push(current)
  }
  out[out.length - 1] = value
  return out
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
