import { useParams, Link } from 'react-router-dom'
import { useMemo, useState } from 'react'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/PageHeader'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import {
  MetricCard,
  TrendIndicator,
  Sparkline,
  TokenUsageChart,
  LatencyHistogram,
  AgentFlowDiagram,
} from '@hnsx/observability'
import { useSession, useSessionEvents, usePauseSession, useResumeSession } from '@/hooks/useSessions'
import { useResolveApproval, useApprovals } from '@/hooks/useApprovals'
import { useDomain } from '@/hooks/useDomains'
import {
  ObservationTimeline,
  useObservationFilters,
} from '@/components/session/ObservationTimeline'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { ArrowLeft, Check, Pause, Play, Radio, X } from 'lucide-react'
import type { ObservationViewModel } from '@/api/mappers'

export default function SessionDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: session, isLoading, error, refetch } = useSession(id)
  const { observations, state, connected } = useSessionEvents(id)
  const [agentFilter, setAgentFilter] = useState('')
  const [kindFilter, setKindFilter] = useState('')

  const { data: pendingApprovals } = useApprovals({ session: id, status: 'pending', limit: 1 })
  const resolve = useResolveApproval()
  const pause = usePauseSession()
  const resume = useResumeSession()
  const liveState = state || session?.state
  const pendingApprovalId = pendingApprovals?.items[0]?.id
  const { data: domain } = useDomain(session?.domainId)

  const budget = useMemo(() => {
    if (!domain) return null
    const d = domain as Record<string, unknown>
    const policy = d.policy as Record<string, unknown> | undefined
    const budgetSpec = policy?.budget as Record<string, unknown> | undefined
    const maxCostUsd = typeof budgetSpec?.max_cost_usd === 'number' ? budgetSpec.max_cost_usd : undefined
    return {
      maxCostUsd,
      requireHumanApproval: !!policy?.require_human_approval,
      guardrails: typeof policy?.guardrails === 'string' ? policy.guardrails : undefined,
    }
  }, [domain])

  const { agents, kinds } = useObservationFilters(observations)

  const stats = useMemo(() => summarize(observations), [observations])
  const durationMs = useMemo(() => {
    if (!session?.startedAt || !session?.completedAt) return undefined
    return session.completedAt.getTime() - session.startedAt.getTime()
  }, [session])

  const tokenSeries = useMemo(() => buildTokenSeries(observations), [observations])
  const latencyBuckets = useMemo(() => buildLatencyBuckets(stats.perStepLatency), [stats.perStepLatency])
  const flowData = useMemo(() => buildAgentFlow(observations), [observations])

  if (isLoading) return <Loading />
  if (error || !session) {
    return <ErrorState description={error?.message || 'Session not found'} onRetry={refetch} />
  }

  const isPaused = (state || session.state) === 'paused'

  return (
    <div className="space-y-6">
      <PageHeader
        title={`Session ${session.id}`}
        breadcrumbs={[
          { label: 'Sessions', href: '/sessions' },
          { label: session.id },
        ]}
      >
        <div className="flex items-center gap-2">
          {liveState === 'running' && (
            <button
              className={cn(buttonVariants({ variant: 'outline' }))}
              onClick={() => pause.mutate({ id: session.id })}
              disabled={pause.isPending}
              title="Stop pulling new turns; the current turn finishes."
            >
              <Pause className="mr-2 h-4 w-4" /> Pause
            </button>
          )}
          {liveState === 'paused' && (
            <button
              className={cn(buttonVariants({ variant: 'default' }))}
              onClick={() => resume.mutate(session.id)}
              disabled={resume.isPending}
              title="Resume pulling turns for this session."
            >
              <Play className="mr-2 h-4 w-4" /> Resume
            </button>
          )}
          {isPaused && pendingApprovalId && (
            <>
              <button
                className={cn(buttonVariants({ variant: 'default' }))}
                onClick={() => resolve.mutate({ id: pendingApprovalId, decision: 'approve' })}
                disabled={resolve.isPending}
              >
                <Check className="mr-2 h-4 w-4" /> Approve
              </button>
              <button
                className={cn(buttonVariants({ variant: 'outline' }))}
                onClick={() => resolve.mutate({ id: pendingApprovalId, decision: 'reject' })}
                disabled={resolve.isPending}
              >
                <X className="mr-2 h-4 w-4" /> Reject
              </button>
            </>
          )}
          <Link
            to="/sessions"
            className={cn(buttonVariants({ variant: 'outline' }), 'no-underline')}
          >
            <ArrowLeft className="mr-2 h-4 w-4" /> Back
          </Link>
        </div>
      </PageHeader>

      {/* KPI row — MetricCard × 4 */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          label="总 Token"
          value={stats.totalTokens}
          formatValue={(n) => (n >= 1000 ? `${(n / 1000).toFixed(1)}K` : `${n}`)}
          unit="tok"
          caption="input + output"
          footer={<Sparkline data={tokenSeries.map((t) => t.input + t.output)} variant="chart-1" />}
        />
        <MetricCard
          label="耗时"
          value={durationMs ?? 0}
          formatValue={(n) => (n >= 1000 ? `${(n / 1000).toFixed(2)}s` : `${Math.round(n)}ms`)}
          caption="首 observation 到末 observation"
          footer={<Sparkline data={stats.perStepLatency.slice(-12)} variant="chart-2" />}
        />
        <MetricCard
          label="Agent 切换"
          value={stats.agentSwitches}
          caption="不同 agent 之间的 hand-off 次数"
          trend={<TrendIndicator value={stats.agentSwitches} previous={Math.max(stats.agentSwitches - 1, 0)} goodWhen="down" />}
        />
        <MetricCard
          label="状态"
          value={
            <span className="flex items-center gap-2">
              <StatusBadge status={state || session.state || ''} />
              {connected && (
                <span className="inline-flex items-center gap-1 text-xs font-medium text-[var(--success-text)]">
                  <Radio className="h-3 w-3 animate-pulse" />
                  live
                </span>
              )}
            </span>
          }
          caption={`${observations.length} observations`}
        />
      </div>

      {/* 上下文 + Budget/Policy + 成本/token 拆解 */}
      <div className="grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Session 上下文</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <Row label="Domain" value={
              <Link to={`/domains/${session.domainId}`} className="font-medium hover:underline">
                {session.domainId}{session.domainVersion ? ` @${session.domainVersion}` : ''}
              </Link>
            } />
            <Row label="State" value={<StatusBadge status={state || session.state || ''} />} />
            <Row label="Started At" value={<Timestamp date={session.startedAt} />} />
            <Row label="Completed At" value={<Timestamp date={session.completedAt} />} />
            {session.traceId && (
              <Row
                label="Trace"
                value={
                  <Link to={`/traces/${session.traceId}`} className="font-mono text-xs hover:underline">
                    {session.traceId}
                  </Link>
                }
              />
            )}
            {session.result && (
              <Row label="Result" value={<span className="text-xs">{session.result}</span>} />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Budget & Policy</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            {budget ? (
              <>
                <Row
                  label="Budget"
                  value={
                    budget.maxCostUsd !== undefined
                      ? `$${budget.maxCostUsd.toFixed(2)}`
                      : <span className="text-muted-foreground">unset</span>
                  }
                />
                <Row
                  label="Human Approval"
                  value={
                    budget.requireHumanApproval
                      ? <span className="text-[var(--warning-text)]">Required</span>
                      : <span className="text-muted-foreground">Optional</span>
                  }
                />
                {budget.guardrails && (
                  <Row label="Guardrails" value={<span className="text-xs">{budget.guardrails}</span>} />
                )}
              </>
            ) : (
              <p className="text-xs text-muted-foreground">Loading…</p>
            )}
          </CardContent>
        </Card>

        <div className="space-y-4 lg:col-span-1">
          <TokenUsageChart data={tokenSeries} height={220} />
          <div className="grid gap-4 lg:grid-cols-2">
            <LatencyHistogram data={latencyBuckets} height={200} />
            <AgentFlowDiagram data={flowData} height={200} />
          </div>
        </div>
      </div>

      {/* Timeline */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Trace Timeline</CardTitle>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <Label htmlFor="agent-filter" className="text-sm">Agent</Label>
              <Select value={agentFilter} onValueChange={(v) => setAgentFilter(v || '')}>
                <SelectTrigger id="agent-filter" className="w-36">
                  <SelectValue placeholder="All agents" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="">All agents</SelectItem>
                  {agents.map((agent) => (
                    <SelectItem key={agent} value={agent}>{agent}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <Label htmlFor="kind-filter" className="text-sm">Kind</Label>
              <Select value={kindFilter} onValueChange={(v) => setKindFilter(v || '')}>
                <SelectTrigger id="kind-filter" className="w-36">
                  <SelectValue placeholder="All kinds" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="">All kinds</SelectItem>
                  {kinds.map((kind) => (
                    <SelectItem key={kind} value={kind}>{kind}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-2">
          <ObservationTimeline
            observations={observations}
            filterAgent={agentFilter || undefined}
            filterKind={kindFilter || undefined}
          />
        </CardContent>
      </Card>
    </div>
  )
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className="text-muted-foreground">{label}</span>
      <span>{value}</span>
    </div>
  )
}

// ----------------- helpers -----------------

interface Summary {
  totalTokens: number
  totalInputTokens: number
  totalOutputTokens: number
  agentSwitches: number
  perStepLatency: number[]
}

function summarize(observations: ObservationViewModel[]): Summary {
  let totalInput = 0
  let totalOutput = 0
  let agentSwitches = 0
  const perStepLatency: number[] = []
  let lastAgent: string | undefined

  for (const obs of observations) {
    const p = obs.payload
    if (p && typeof p === 'object' && !Array.isArray(p)) {
      const tokens = (p as Record<string, unknown>).tokens ?? (p as Record<string, unknown>).usage
      if (tokens && typeof tokens === 'object') {
        const t = tokens as Record<string, unknown>
        if (typeof t.input === 'number') totalInput += t.input
        else if (typeof t.prompt_tokens === 'number') totalInput += t.prompt_tokens as number
        if (typeof t.output === 'number') totalOutput += t.output
        else if (typeof t.completion_tokens === 'number') totalOutput += t.completion_tokens as number
      }
      const lat =
        (p as Record<string, unknown>).duration_ms ??
        (p as Record<string, unknown>).durationMs ??
        (p as Record<string, unknown>).latency_ms ??
        (p as Record<string, unknown>).latencyMs
      if (typeof lat === 'number' && lat > 0) perStepLatency.push(lat)
    }
    if (obs.agentId && lastAgent && obs.agentId !== lastAgent) agentSwitches += 1
    if (obs.agentId) lastAgent = obs.agentId
  }

  return {
    totalTokens: totalInput + totalOutput,
    totalInputTokens: totalInput,
    totalOutputTokens: totalOutput,
    agentSwitches,
    perStepLatency,
  }
}

function buildTokenSeries(observations: ObservationViewModel[]): { label: string; input: number; output: number }[] {
  // 按 createdAt 分桶到 12 个等宽窗口
  if (observations.length === 0) return []
  const timestamps = observations
    .map((o) => o.createdAt?.getTime() ?? 0)
    .filter((t) => t > 0)
  if (timestamps.length === 0) return []
  const min = Math.min(...timestamps)
  const max = Math.max(...timestamps)
  const bucketCount = 12
  const span = Math.max(max - min, 1)
  const width = span / bucketCount
  const buckets = Array.from({ length: bucketCount }, (_, i) => ({
    label: formatBucketLabel(min + i * width + width / 2),
    input: 0,
    output: 0,
  }))
  for (const obs of observations) {
    const t = obs.createdAt?.getTime() ?? 0
    if (t <= 0) continue
    const idx = Math.min(bucketCount - 1, Math.floor((t - min) / Math.max(width, 1)))
    const p = obs.payload
    if (p && typeof p === 'object' && !Array.isArray(p)) {
      const tokens = (p as Record<string, unknown>).tokens ?? (p as Record<string, unknown>).usage
      if (tokens && typeof tokens === 'object') {
        const tt = tokens as Record<string, unknown>
        const inp =
          typeof tt.input === 'number'
            ? (tt.input as number)
            : typeof tt.prompt_tokens === 'number'
              ? (tt.prompt_tokens as number)
              : 0
        const out =
          typeof tt.output === 'number'
            ? (tt.output as number)
            : typeof tt.completion_tokens === 'number'
              ? (tt.completion_tokens as number)
              : 0
        buckets[idx]!.input += inp
        buckets[idx]!.output += out
      }
    }
  }
  return buckets
}

function buildLatencyBuckets(latencies: number[]): { label: string; count: number }[] {
  const buckets = [
    { label: '<100ms', count: 0 },
    { label: '100-300ms', count: 0 },
    { label: '300ms-1s', count: 0 },
    { label: '1-3s', count: 0 },
    { label: '>3s', count: 0 },
  ]
  for (const ms of latencies) {
    if (ms < 100) buckets[0]!.count += 1
    else if (ms < 300) buckets[1]!.count += 1
    else if (ms < 1000) buckets[2]!.count += 1
    else if (ms < 3000) buckets[3]!.count += 1
    else buckets[4]!.count += 1
  }
  return buckets
}

function buildAgentFlow(observations: ObservationViewModel[]): { source: string; target: string; count: number; avgMs?: number }[] {
  const sorted = [...observations].sort(
    (a, b) => (a.createdAt?.getTime() ?? 0) - (b.createdAt?.getTime() ?? 0),
  )
  const transitions = new Map<string, { count: number; totalMs: number }>()
  for (let i = 1; i < sorted.length; i++) {
    const prev = sorted[i - 1]!
    const cur = sorted[i]!
    if (!prev.agentId || !cur.agentId || prev.agentId === cur.agentId) continue
    const key = `${prev.agentId}->${cur.agentId}`
    const entry = transitions.get(key) ?? { count: 0, totalMs: 0 }
    entry.count += 1
    const p = cur.payload
    if (p && typeof p === 'object' && !Array.isArray(p)) {
      const lat =
        (p as Record<string, unknown>).duration_ms ??
        (p as Record<string, unknown>).durationMs ??
        (p as Record<string, unknown>).latency_ms ??
        (p as Record<string, unknown>).latencyMs
      if (typeof lat === 'number') entry.totalMs += lat
    }
    transitions.set(key, entry)
  }
  return Array.from(transitions.entries()).map(([key, { count, totalMs }]) => {
    const [source, target] = key.split('->') as [string, string]
    return { source, target, count, avgMs: totalMs / Math.max(count, 1) }
  })
}

function formatBucketLabel(ms: number): string {
  const d = new Date(ms)
  const hh = String(d.getHours()).padStart(2, '0')
  const mm = String(d.getMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}