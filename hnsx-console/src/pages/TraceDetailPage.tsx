import { useParams, Link } from 'react-router-dom'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/PageHeader'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { Timestamp } from '@/components/ui/Timestamp'
import { MetricCard } from '@hnsx/observability'
import { Sparkline } from '@hnsx/observability'
import { SpanList, type SpanListItem } from '@hnsx/observability'
import { useTrace } from '@/hooks/useTraces'
import { ArrowLeft } from 'lucide-react'

export default function TraceDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: trace, isLoading, error, refetch } = useTrace(id)

  if (isLoading) return <Loading />
  if (error || !trace) {
    return <ErrorState description={error?.message || 'Trace not found'} onRetry={refetch} />
  }

  // 把 trace.observations（JsonValue[]）转成 SpanListItem
  const spans: SpanListItem[] = trace.observations
    .map((o) => normalizeObservation(o))
    .filter((s): s is SpanListItem => s !== null)

  // 顶部统计：总 spans / 总 token（mock — 等 ObservationViewModel 接进来后可读 payload.tokens）
  const errorCount = spans.filter((s) => s.kind === 'error').length
  const toolCallCount = spans.filter((s) => s.kind === 'tool_call' || s.kind === 'tool_result').length

  // sparkline 用的 latency 序列（按 spans 出现顺序的 endMs-startMs）
  const latencySeries = spans.map((s) => s.endMs - s.startMs)

  return (
    <div className="space-y-6">
      <PageHeader
        title={`Trace ${trace.traceId}`}
        breadcrumbs={[
          { label: 'Traces', href: '/traces' },
          { label: trace.traceId },
        ]}
      >
        <Link
          to={`/sessions/${trace.sessionId}`}
          className={cn(buttonVariants({ variant: 'outline' }), 'no-underline')}
        >
          <ArrowLeft className="mr-2 h-4 w-4" /> Session
        </Link>
      </PageHeader>

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          label="Spans"
          value={spans.length}
          caption="Observation 总数"
        />
        <MetricCard
          label="Duration"
          value={trace.durationMs ?? 0}
          formatValue={(n) => `${(n / 1000).toFixed(2)}s`}
          caption="首 observation 到末 observation"
        />
        <MetricCard
          label="Tool Calls"
          value={toolCallCount}
          caption="tool_call + tool_result"
          footer={<Sparkline data={latencySeries} variant="chart-2" />}
        />
        <MetricCard
          label="Errors"
          value={errorCount}
          caption="kind = error"
          footer={<Sparkline data={latencySeries} variant={errorCount > 0 ? 'danger' : 'success'} />}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Trace 上下文</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-muted-foreground">Trace ID</span>
              <span className="font-mono">{trace.traceId}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Session</span>
              <Link to={`/sessions/${trace.sessionId}`} className="font-medium hover:underline">
                {trace.sessionId}
              </Link>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Domain</span>
              <Link to={`/domains/${trace.domainId}`} className="font-medium hover:underline">
                {trace.domainId}
                {trace.domainVersion ? ` @${trace.domainVersion}` : ''}
              </Link>
            </div>
            <div className="flex justify-between">
              <span className="text-muted-foreground">Started At</span>
              <Timestamp date={trace.startedAt} />
            </div>
            {trace.agentRefs.length > 0 && (
              <div className="flex items-start justify-between gap-2">
                <span className="text-muted-foreground">Agents</span>
                <div className="flex flex-wrap justify-end gap-1">
                  {trace.agentRefs.map((a) => (
                    <span
                      key={a}
                      className="inline-flex items-center rounded-full bg-muted px-2 py-0.5 text-xs"
                    >
                      {a}
                    </span>
                  ))}
                </div>
              </div>
            )}
          </CardContent>
        </Card>

        <div className="lg:col-span-2">
          <SpanList spans={spans} height={520} header={<SpanListHeader count={spans.length} />} />
        </div>
      </div>
    </div>
  )
}

function SpanListHeader({ count }: { count: number }) {
  return (
    <div className="flex items-center justify-between border-b border-[var(--chart-grid)] px-3 py-2">
      <h3 className="text-sm font-semibold text-[var(--chart-text-primary)]">Observations</h3>
      <span className="text-xs text-[var(--chart-text-muted)]">{count} spans</span>
    </div>
  )
}

/**
 * 把 trace 里的 observation JSON 规整成 SpanListItem。
 * Observation 的字段不固定（kind/role/agentId/stepId/parentId/payload/metadata/createdAtMs），
 * 容错读取；读不到的字段就当作 null。
 */
function normalizeObservation(raw: unknown): SpanListItem | null {
  if (!raw || typeof raw !== 'object') return null
  const r = raw as Record<string, unknown>
  const id =
    (r.observation_id as string) ||
    (r.observationId as string) ||
    (r.id as string) ||
    ''
  const kind = (r.kind as string) || 'message'
  const role = (r.role as string) || undefined
  const agentId = ((r.agent_id as string) || (r.agentId as string)) || undefined
  const stepId = ((r.step_id as string) || (r.stepId as string)) || undefined
  const parentId = ((r.parent_id as string) || (r.parentId as string)) || undefined
  const startMs = toMs(r.created_at_ms ?? r.createdAtMs ?? r.created_at ?? r.createdAt)
  // 没有显式 endMs — 用下一个 observation 的 startMs 兜底，fallback +1s
  const endMs = toMs(r.end_at_ms ?? r.endAtMs) ?? startMs + 1000
  const payload = isRecord(r.payload) ? r.payload : undefined
  const metadata = isRecord(r.metadata) ? r.metadata : undefined
  return {
    id,
    name: stepId ? `${kind}:${stepId}` : agentId ? `${kind}:${agentId}` : `${kind}:${id.slice(0, 8)}`,
    kind,
    agentId,
    role,
    startMs,
    endMs,
    parentId,
    tokens: extractTokens(payload),
    payload,
    metadata,
  }
}

function extractTokens(payload: Record<string, unknown> | undefined): SpanListItem['tokens'] {
  if (!payload) return undefined
  const t = payload.tokens ?? payload.usage
  if (!isRecord(t)) return undefined
  const input = typeof t.input === 'number' ? t.input : typeof t.prompt_tokens === 'number' ? t.prompt_tokens : undefined
  const output = typeof t.output === 'number' ? t.output : typeof t.completion_tokens === 'number' ? t.completion_tokens : undefined
  const total = typeof t.total === 'number' ? t.total : input !== undefined || output !== undefined ? (input ?? 0) + (output ?? 0) : undefined
  if (input === undefined && output === undefined && total === undefined) return undefined
  return { input, output, total }
}

function toMs(v: unknown): number {
  if (typeof v === 'number') return v
  if (typeof v === 'string') {
    const n = Date.parse(v)
    return Number.isNaN(n) ? 0 : n
  }
  return 0
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}