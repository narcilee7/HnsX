import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import type { ColumnDef } from '@tanstack/react-table'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable } from '@/components/ui/DataTable'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { Button, buttonVariants } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import {
  useSessions,
  useCreateSession,
  useCancelSession,
  usePauseSession,
  useResumeSession,
} from '@/hooks/useSessions'
import { useDomains } from '@/hooks/useDomains'
import type { SessionViewModel } from '@/api/mappers'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Play, Pause, RotateCcw, XCircle, Search, Radio } from 'lucide-react'

const STATE_OPTIONS = [
  { value: 'running', label: 'Running', tone: 'info' as const },
  { value: 'completed', label: 'Completed', tone: 'success' as const },
  { value: 'failed', label: 'Failed', tone: 'danger' as const },
  { value: 'paused', label: 'Paused', tone: 'warning' as const },
]

export default function SessionsPage() {
  const navigate = useNavigate()
  const [stateFilter, setStateFilter] = useState('')
  const [domainFilter, setDomainFilter] = useState('')
  const [searchInput, setSearchInput] = useState('')
  const [searchApplied, setSearchApplied] = useState('')

  const { data: domainsData } = useDomains({ limit: 200 })
  const { data, isLoading, error, refetch, isFetching } = useSessions({
    domain: domainFilter || undefined,
    state: stateFilter || undefined,
    limit: 50,
  })
  const createSession = useCreateSession()
  const cancelSession = useCancelSession()
  const pauseSession = usePauseSession()
  const resumeSession = useResumeSession()

  const filteredItems = useMemo(() => {
    if (!searchApplied) return data?.items ?? []
    const needle = searchApplied.toLowerCase()
    return (data?.items ?? []).filter(
      (s) =>
        s.id.toLowerCase().includes(needle) ||
        s.domainId.toLowerCase().includes(needle),
    )
  }, [data?.items, searchApplied])

  const columns = useMemo<ColumnDef<SessionViewModel>[]>(
    () => [
      {
        accessorKey: 'id',
        header: 'ID',
        cell: ({ row }) => (
          <Link to={`/sessions/${row.original.id}`} className="font-mono text-xs font-medium hover:underline">
            {row.original.id.slice(0, 16)}
          </Link>
        ),
      },
      {
        accessorKey: 'domainId',
        header: 'Domain',
        cell: ({ row }) => (
          <Link to={`/domains/${row.original.domainId}`} className="text-sm hover:underline">
            {row.original.domainId}
          </Link>
        ),
      },
      {
        accessorKey: 'state',
        header: 'State',
        cell: ({ row }) => (
          <div className="flex items-center gap-1.5">
            <StatusBadge status={row.original.state} />
            {row.original.state === 'running' && (
              <Radio className="h-3 w-3 animate-pulse text-[var(--info)]" />
            )}
          </div>
        ),
      },
      {
        accessorKey: 'startedAt',
        header: 'Started',
        cell: ({ row }) => <Timestamp date={row.original.startedAt} />,
      },
      {
        accessorKey: 'completedAt',
        header: 'Duration',
        cell: ({ row }) => {
          const s = row.original.startedAt?.getTime()
          const c = row.original.completedAt?.getTime()
          if (!s) return <span className="text-muted-foreground">—</span>
          const end = c ?? Date.now()
          const ms = end - s
          return <span className="text-xs tabular-nums">{formatDuration(ms)}</span>
        },
      },
      {
        id: 'actions',
        header: 'Actions',
        cell: ({ row }) => (
          <div className="flex items-center gap-1">
            {row.original.state === 'running' ? (
              <>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => pauseSession.mutate({ id: row.original.id })}
                  aria-label="Pause session"
                  title="Pause"
                >
                  <Pause className="h-3.5 w-3.5 text-[var(--warning)]" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => cancelSession.mutate(row.original.id)}
                  aria-label="Cancel session"
                  title="Cancel"
                >
                  <XCircle className="h-3.5 w-3.5 text-[var(--danger)]" />
                </Button>
              </>
            ) : row.original.state === 'paused' ? (
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => resumeSession.mutate(row.original.id)}
                aria-label="Resume session"
                title="Resume"
              >
                <Play className="h-3.5 w-3.5 text-[var(--success)]" />
              </Button>
            ) : (
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() =>
                  createSession.mutate(
                    { domain_id: row.original.domainId },
                    {
                      onSuccess: (res) => navigate(`/sessions/${res.id}`),
                    },
                  )
                }
                aria-label="Rerun session"
                title="Rerun"
                disabled={createSession.isPending}
              >
                <RotateCcw className="h-3.5 w-3.5" />
              </Button>
            )}
            <Link
              to={`/sessions/${row.original.id}`}
              className={cn(buttonVariants({ variant: 'ghost', size: 'icon-sm' }), 'no-underline')}
              aria-label="Open session"
              title="Open"
            >
              <Play className="h-3.5 w-3.5" />
            </Link>
          </div>
        ),
      },
    ],
    [cancelSession, createSession, navigate],
  )

  if (error) {
    return <ErrorState description={error.message} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title="Sessions"
        description="实时监控 Session 执行。5 秒自动刷新。"
      >
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Radio className={`h-3 w-3 ${isFetching ? 'animate-pulse text-[var(--info)]' : 'text-muted-foreground'}`} />
          {isFetching ? 'refreshing…' : `${data?.total ?? 0} sessions`}
        </div>
      </PageHeader>

      <div className="rounded-lg border bg-card p-4">
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          <div className="space-y-1.5">
            <label className="text-xs text-muted-foreground">Domain</label>
            <Select value={domainFilter} onValueChange={(v) => setDomainFilter(v || '')}>
              <SelectTrigger>
                <SelectValue placeholder="All domains" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="">All domains</SelectItem>
                {domainsData?.items.map((d) => (
                  <SelectItem key={d.id} value={d.id}>{d.id}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs text-muted-foreground">State</label>
            <Select value={stateFilter} onValueChange={(v) => setStateFilter(v || '')}>
              <SelectTrigger>
                <SelectValue placeholder="All states" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="">All states</SelectItem>
                {STATE_OPTIONS.map((s) => (
                  <SelectItem key={s.value} value={s.value}>{s.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5 lg:col-span-2">
            <label className="text-xs text-muted-foreground">Search</label>
            <div className="flex gap-2">
              <div className="relative flex-1">
                <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <Input
                  className="pl-8"
                  placeholder="Session ID 或 Domain"
                  value={searchInput}
                  onChange={(e) => setSearchInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') setSearchApplied(searchInput)
                  }}
                />
              </div>
              <Button size="sm" variant="outline" onClick={() => setSearchApplied(searchInput)}>
                Search
              </Button>
            </div>
          </div>
        </div>
      </div>

      <DataTable columns={columns} data={filteredItems} loading={isLoading} />
    </div>
  )
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`
  if (ms < 3_600_000) return `${(ms / 60_000).toFixed(1)}m`
  return `${(ms / 3_600_000).toFixed(1)}h`
}