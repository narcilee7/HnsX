import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ColumnDef } from '@tanstack/react-table'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable } from '@/components/ui/DataTable'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useTraces } from '@/hooks/useTraces'
import { useDomains } from '@/hooks/useDomains'
import type { TraceSummaryViewModel } from '@/api/mappers'
import { TraceMiniBar } from '@hnsx/observability'
import { Search, RotateCcw } from 'lucide-react'

function formatDuration(ms: number | undefined): string {
  if (ms === undefined || Number.isNaN(ms)) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

/**
 * TraceSummary rows don't carry observations (the server keeps the list
 * endpoint aggregate-only to keep paging cheap). Render the mini bar as
 * a single neutral segment carrying the total duration so the column
 * still has visual weight. Detail pages build real spans from /traces/:id.
 */
function buildMiniSpans(sum: TraceSummaryViewModel): { id: string; startMs: number; endMs: number; variant: 'chart-1' | 'chart-2' | 'chart-3' | 'chart-4' | 'chart-5' | 'success' | 'warning' | 'danger' | 'info' }[] {
  const totalMs = sum.durationMs ?? 0
  if (totalMs <= 0) return []
  return [
    { id: 'seg-total', startMs: 0, endMs: totalMs, variant: sum.status === 'completed' ? 'chart-1' : 'chart-3' },
  ]
}

export default function TracesPage() {
  const [domain, setDomain] = useState('')
  const [session, setSession] = useState('')
  const [agent, setAgent] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')

  const [appliedFilters, setAppliedFilters] = useState({
    domain: '',
    session: '',
    agent: '',
    from: '',
    to: '',
  })

  const { data: domainsData } = useDomains({ limit: 200 })
  const { data, isLoading, error, refetch } = useTraces({
    domain: appliedFilters.domain || undefined,
    session: appliedFilters.session || undefined,
    agent: appliedFilters.agent || undefined,
    from: appliedFilters.from || undefined,
    to: appliedFilters.to || undefined,
    limit: 50,
  })

  const columns = useMemo<ColumnDef<TraceSummaryViewModel>[]>(
    () => [
      {
        accessorKey: 'traceId',
        header: 'Trace ID',
        cell: ({ row }) => (
          <Link to={`/traces/${row.original.traceId}`} className="font-medium hover:underline">
            {row.original.traceId}
          </Link>
        ),
      },
      {
        accessorKey: 'sessionId',
        header: 'Session',
        cell: ({ row }) => (
          <Link
            to={`/sessions/${row.original.sessionId}`}
            className="text-sm text-muted-foreground hover:underline"
          >
            {row.original.sessionId}
          </Link>
        ),
      },
      { accessorKey: 'domainId', header: 'Domain' },
      {
        accessorKey: 'status',
        header: 'Status',
        cell: ({ row }) => <StatusBadge status={row.original.status} />,
      },
      {
        accessorKey: 'startedAt',
        header: 'Started At',
        cell: ({ row }) => <Timestamp date={row.original.startedAt} />,
      },
      {
        accessorKey: 'durationMs',
        header: 'Duration',
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <TraceMiniBar
              spans={buildMiniSpans(row.original)}
              height={10}
              width={120}
              totalMs={row.original.durationMs}
            />
            <span className="text-xs tabular-nums">{formatDuration(row.original.durationMs)}</span>
          </div>
        ),
      },
      {
        accessorKey: 'observationCount',
        header: 'Observations',
        cell: ({ row }) => (
          <span className="text-sm tabular-nums">{row.original.observationCount}</span>
        ),
      },
      {
        accessorKey: 'agentInvocations',
        header: 'Agent / Tool',
        cell: ({ row }) => (
          <span className="text-sm tabular-nums">
            {row.original.agentInvocations} / {row.original.toolInvocations}
          </span>
        ),
      },
      {
        accessorKey: 'totalCostUsd',
        header: 'Cost (USD)',
        cell: ({ row }) => (
          <span className="text-sm tabular-nums">${row.original.totalCostUsd.toFixed(4)}</span>
        ),
      },
    ],
    [],
  )

  const handleSearch = () => {
    setAppliedFilters({ domain, session, agent, from, to })
  }

  const handleReset = () => {
    setDomain('')
    setSession('')
    setAgent('')
    setFrom('')
    setTo('')
    setAppliedFilters({ domain: '', session: '', agent: '', from: '', to: '' })
  }

  const filtersDirty =
    domain !== appliedFilters.domain ||
    session !== appliedFilters.session ||
    agent !== appliedFilters.agent ||
    from !== appliedFilters.from ||
    to !== appliedFilters.to

  if (error) {
    return <ErrorState description={error.message} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader title="Traces" description="Query and inspect traces across sessions." />

      <div className="rounded-lg border bg-card p-4">
        <div className="grid gap-4 md:grid-cols-3 lg:grid-cols-5">
          <div className="space-y-1.5">
            <Label htmlFor="domain-filter" className="text-xs">
              Domain
            </Label>
            <Select value={domain} onValueChange={(v) => setDomain(v || '')}>
              <SelectTrigger id="domain-filter">
                <SelectValue placeholder="All domains" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="">All domains</SelectItem>
                {domainsData?.items.map((d) => (
                  <SelectItem key={d.id} value={d.id}>
                    {d.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="session-filter" className="text-xs">
              Session
            </Label>
            <Input
              id="session-filter"
              placeholder="Session ID"
              value={session}
              onChange={(e) => setSession(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="agent-filter" className="text-xs">
              Agent
            </Label>
            <Input
              id="agent-filter"
              placeholder="Agent ID"
              value={agent}
              onChange={(e) => setAgent(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="from-filter" className="text-xs">
              From
            </Label>
            <Input
              id="from-filter"
              type="datetime-local"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="to-filter" className="text-xs">
              To
            </Label>
            <Input
              id="to-filter"
              type="datetime-local"
              value={to}
              onChange={(e) => setTo(e.target.value)}
            />
          </div>
        </div>

        <div className="mt-4 flex items-center justify-end gap-2">
          <Button variant="outline" size="sm" onClick={handleReset}>
            <RotateCcw className="mr-1.5 h-4 w-4" />
            Reset
          </Button>
          <Button size="sm" onClick={handleSearch} disabled={!filtersDirty}>
            <Search className="mr-1.5 h-4 w-4" />
            Search
          </Button>
        </div>
      </div>

      <DataTable columns={columns} data={data?.items || []} loading={isLoading} />
    </div>
  )
}
