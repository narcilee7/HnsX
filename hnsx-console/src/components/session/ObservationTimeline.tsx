import { useMemo, useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Timestamp } from '@/components/ui/Timestamp'
import type { JsonValue } from '@bufbuild/protobuf'
import type { ObservationViewModel } from '@/api/mappers'

interface ObservationCardProps {
  observation: ObservationViewModel
  depth?: number
  /** 与上一条 observation 的 agent 是否不同 — 用于插入分隔 */
  agentChanged?: boolean
}

const kindVariantMap: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  text: 'default',
  message: 'default',
  user: 'secondary',
  assistant: 'default',
  tool_call: 'secondary',
  tool_result: 'secondary',
  error: 'destructive',
  cost: 'outline',
  state: 'outline',
  thinking: 'outline',
}

const kindAccentVar: Record<string, string> = {
  text: 'var(--chart-1)',
  message: 'var(--chart-1)',
  user: 'var(--chart-3)',
  assistant: 'var(--chart-1)',
  tool_call: 'var(--chart-2)',
  tool_result: 'var(--chart-2)',
  error: 'var(--danger)',
  cost: 'var(--chart-4)',
  state: 'var(--chart-5)',
  thinking: 'var(--chart-5)',
}

export function ObservationCard({ observation, depth = 0, agentChanged }: ObservationCardProps) {
  const [expanded, setExpanded] = useState(true)
  const variant = kindVariantMap[observation.kind] || 'outline'
  const accent = kindAccentVar[observation.kind] ?? 'var(--chart-5)'

  const { tokens, latencyMs } = useMemo(() => extractMetrics(observation.payload), [observation.payload])

  return (
    <>
      {agentChanged && observation.agentId && (
        <div
          className="flex items-center gap-2 py-1 text-[10px] uppercase tracking-wider text-[var(--chart-text-muted)]"
          style={{ marginLeft: depth * 16 }}
        >
          <span className="h-px flex-1 bg-[var(--chart-grid)]" />
          <span>agent → {observation.agentId}</span>
          <span className="h-px flex-1 bg-[var(--chart-grid)]" />
        </div>
      )}
      <Card
        className={cn(
          'overflow-hidden border-l-4 transition-colors',
          depth === 0 ? 'border-l-[var(--chart-1)]' : 'border-l-[var(--chart-5)]',
        )}
        style={{ marginLeft: depth * 16, borderLeftColor: accent }}
      >
        <CardHeader className="flex flex-row items-center justify-between gap-2 p-3">
          <div className="flex flex-1 items-center gap-3 overflow-hidden">
            <Button variant="ghost" size="icon-xs" onClick={() => setExpanded((v) => !v)}>
              {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
            </Button>
            <Badge variant={variant}>{observation.kind}</Badge>
            {observation.agentId && (
              <span className="truncate text-sm text-muted-foreground">{observation.agentId}</span>
            )}
            {observation.role && <Badge variant="outline">{observation.role}</Badge>}
            {observation.stepId && (
              <span className="truncate text-xs text-muted-foreground">{observation.stepId}</span>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {latencyMs !== undefined && latencyMs > 0 && (
              <span className="rounded bg-[var(--chart-grid)]/60 px-1.5 py-0.5 text-[10px] tabular-nums text-[var(--chart-text-secondary)]">
                {formatDuration(latencyMs)}
              </span>
            )}
            {tokens && (tokens.total ?? 0) > 0 && (
              <span className="rounded bg-[var(--chart-grid)]/60 px-1.5 py-0.5 text-[10px] tabular-nums text-[var(--chart-text-secondary)]">
                {formatTokens(tokens.total ?? 0)}
              </span>
            )}
            <Timestamp date={observation.createdAt} />
          </div>
        </CardHeader>

        {/* token + latency 微条 */}
        {(tokens || latencyMs !== undefined) && (
          <div className="grid grid-cols-2 gap-3 px-3 pb-2">
            {tokens && (tokens.total ?? 0) > 0 && (
              <MetricMicroBar
                label="tokens"
                used={tokens.total ?? 0}
                parts={
                  tokens.input !== undefined || tokens.output !== undefined
                    ? [
                        { value: tokens.input ?? 0, color: 'var(--chart-3)' },
                        { value: tokens.output ?? 0, color: 'var(--chart-1)' },
                      ]
                    : [{ value: tokens.total ?? 0, color: 'var(--chart-1)' }]
                }
              />
            )}
            {latencyMs !== undefined && latencyMs > 0 && (
              <div className="flex items-center gap-2 text-[10px] text-[var(--chart-text-muted)]">
                <span className="w-12 shrink-0">latency</span>
                <div className="relative h-1.5 flex-1 overflow-hidden rounded bg-[var(--chart-grid)]/40">
                  <div
                    className="absolute inset-y-0 left-0 rounded"
                    style={{
                      width: `${Math.min((latencyMs / 5000) * 100, 100)}%`,
                      background:
                        latencyMs > 3000
                          ? 'var(--danger)'
                          : latencyMs > 1000
                            ? 'var(--warning)'
                            : 'var(--success)',
                    }}
                  />
                </div>
              </div>
            )}
          </div>
        )}

        {expanded && (
          <CardContent className="space-y-2 p-3 pt-0">
            {observation.payload && isObject(observation.payload) && Object.keys(observation.payload).length > 0 && (
              <div className="space-y-1">
                <p className="text-xs font-medium text-muted-foreground">Payload</p>
                <pre className="max-h-48 overflow-auto rounded-md bg-muted p-2 text-xs">
                  {JSON.stringify(observation.payload, null, 2)}
                </pre>
              </div>
            )}
            {observation.metadata && isObject(observation.metadata) && Object.keys(observation.metadata).length > 0 && (
              <div className="space-y-1">
                <p className="text-xs font-medium text-muted-foreground">Metadata</p>
                <pre className="max-h-32 overflow-auto rounded-md bg-muted p-2 text-xs">
                  {JSON.stringify(observation.metadata, null, 2)}
                </pre>
              </div>
            )}
          </CardContent>
        )}
      </Card>
    </>
  )
}

function MetricMicroBar({
  label,
  used,
  parts,
}: {
  label: string
  used: number
  parts: { value: number; color: string }[]
}) {
  const max = Math.max(used, 1)
  return (
    <div className="flex items-center gap-2 text-[10px] text-[var(--chart-text-muted)]">
      <span className="w-12 shrink-0">{label}</span>
      <div className="relative h-1.5 flex-1 overflow-hidden rounded bg-[var(--chart-grid)]/40">
        <div className="absolute inset-0 flex">
          {parts.map((p, i) => (
            <div
              key={`${label}-${i}`}
              style={{ width: `${(p.value / max) * 100}%`, background: p.color, opacity: 0.7 }}
            />
          ))}
        </div>
      </div>
      <span className="w-14 shrink-0 text-right tabular-nums text-[var(--chart-text-secondary)]">
        {formatTokens(used)}
      </span>
    </div>
  )
}

interface ObservationTimelineProps {
  observations: ObservationViewModel[]
  filterAgent?: string
  filterKind?: string
}

export function ObservationTimeline({ observations, filterAgent, filterKind }: ObservationTimelineProps) {
  const filtered = observations.filter((obs) => {
    if (filterAgent && obs.agentId !== filterAgent) return false
    if (filterKind && obs.kind !== filterKind) return false
    return true
  })

  const sorted = useMemo(
    () => [...filtered].sort((a, b) => (a.createdAt?.getTime() ?? 0) - (b.createdAt?.getTime() ?? 0)),
    [filtered],
  )

  if (sorted.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">No observations match the current filters.</p>
    )
  }

  return (
    <div className="space-y-3">
      {sorted.map((obs, i) => {
        const prev = i > 0 ? sorted[i - 1] : undefined
        const agentChanged =
          !!obs.agentId && !!prev?.agentId && prev.agentId !== obs.agentId
        return <ObservationCard key={obs.observationId} observation={obs} agentChanged={agentChanged} />
      })}
    </div>
  )
}

export function useObservationFilters(observations: ObservationViewModel[]) {
  const agents = Array.from(new Set(observations.map((o) => o.agentId).filter(Boolean)))
  const kinds = Array.from(new Set(observations.map((o) => o.kind).filter(Boolean)))
  return { agents, kinds }
}

// ----------------- helpers -----------------

interface ExtractedMetrics {
  tokens?: { input?: number; output?: number; total?: number }
  latencyMs?: number
}

function extractMetrics(payload: JsonValue | undefined): ExtractedMetrics {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return {}
  const p = payload as Record<string, unknown>
  const out: ExtractedMetrics = {}

  const t = (p.tokens ?? p.usage) as Record<string, unknown> | undefined
  if (t && typeof t === 'object') {
    const input = typeof t.input === 'number' ? t.input : typeof t.prompt_tokens === 'number' ? t.prompt_tokens : undefined
    const output = typeof t.output === 'number' ? t.output : typeof t.completion_tokens === 'number' ? t.completion_tokens : undefined
    const total =
      typeof t.total === 'number'
        ? t.total
        : input !== undefined || output !== undefined
          ? (input ?? 0) + (output ?? 0)
          : undefined
    if (input !== undefined || output !== undefined || total !== undefined) {
      out.tokens = { input, output, total }
    }
  }

  const lat =
    p.duration_ms ?? p.durationMs ?? p.latency_ms ?? p.latencyMs ?? p.latency ?? p.elapsed_ms
  if (typeof lat === 'number' && lat > 0) out.latencyMs = lat

  return out
}

function isObject(v: JsonValue): boolean {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)} s`
  return `${(ms / 60_000).toFixed(1)} min`
}

function formatTokens(n: number): string {
  if (n < 1000) return `${n}`
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10_000 ? 1 : 0)}K`
  return `${(n / 1_000_000).toFixed(1)}M`
}