import { useMemo, useState } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable } from '@/components/ui/DataTable'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useAuditLogs } from '@/hooks/useAudit'
import type { AuditLogViewModel } from '@/api/mappers'
import { Search, Download, FileJson, RotateCcw } from 'lucide-react'

export default function AuditPage() {
  // UI draft state
  const [action, setAction] = useState('')
  const [decision, setDecision] = useState('')
  const [actor, setActor] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')

  // applied filters
  const [applied, setApplied] = useState({
    action: '',
    decision: '',
    actor: '',
    from: '',
    to: '',
  })

  const { data, isLoading, error, refetch } = useAuditLogs({
    action: applied.action || undefined,
    decision: applied.decision || undefined,
    actor: applied.actor || undefined,
    from: applied.from || undefined,
    to: applied.to || undefined,
    limit: 200, // bump for export
  })

  const columns = useMemo<ColumnDef<AuditLogViewModel>[]>(
    () => [
      {
        accessorKey: 'timestamp',
        header: 'Timestamp',
        cell: ({ row }) => <Timestamp date={row.original.timestamp} />,
      },
      { accessorKey: 'action', header: 'Action' },
      { accessorKey: 'actor', header: 'Actor' },
      {
        accessorKey: 'resource',
        header: 'Resource',
        cell: ({ row }) => row.original.resource ? (
          <span className="font-mono text-xs">{row.original.resource}</span>
        ) : <span className="text-muted-foreground">—</span>,
      },
      {
        accessorKey: 'decision',
        header: 'Decision',
        cell: ({ row }) => {
          const d = row.original.decision
          if (!d) return <span className="text-muted-foreground">—</span>
          const tone =
            d === 'allow' || d === 'approved'
              ? 'bg-[var(--success-soft)] text-[var(--success-text)]'
              : d === 'deny' || d === 'rejected'
                ? 'bg-[var(--danger-soft)] text-[var(--danger-text)]'
                : 'bg-[var(--info-soft)] text-[var(--info-text)]'
          return <span className={`rounded px-1.5 py-0.5 text-[10px] font-medium ${tone}`}>{d}</span>
        },
      },
      {
        accessorKey: 'reason',
        header: 'Reason',
        cell: ({ row }) => row.original.reason ? (
          <span className="line-clamp-2 max-w-md text-xs text-muted-foreground" title={row.original.reason}>
            {row.original.reason}
          </span>
        ) : <span className="text-muted-foreground">—</span>,
      },
    ],
    [],
  )

  const handleSearch = () => {
    setApplied({ action, decision, actor, from, to })
  }

  const handleReset = () => {
    setAction('')
    setDecision('')
    setActor('')
    setFrom('')
    setTo('')
    setApplied({ action: '', decision: '', actor: '', from: '', to: '' })
  }

  const exportCsv = () => {
    const rows = data?.items ?? []
    const headers = ['timestamp', 'action', 'actor', 'resource', 'decision', 'reason', 'session_id', 'domain_id']
    const escape = (v: unknown) => {
      const s = v === null || v === undefined ? '' : String(v)
      return `"${s.replace(/"/g, '""')}"`
    }
    const csv = [
      headers.join(','),
      ...rows.map((r) =>
        [
          r.timestamp?.toISOString() ?? '',
          r.action,
          r.actor,
          r.resource,
          r.decision,
          r.reason,
          r.sessionId,
          r.domainId,
        ]
          .map(escape)
          .join(','),
      ),
    ].join('\n')
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
    downloadBlob(blob, `audit-${Date.now()}.csv`)
  }

  const exportJson = () => {
    const rows = (data?.items ?? []).map((r) => ({
      ...r,
      timestamp: r.timestamp?.toISOString() ?? null,
    }))
    const json = JSON.stringify(rows, null, 2)
    const blob = new Blob([json], { type: 'application/json' })
    downloadBlob(blob, `audit-${Date.now()}.json`)
  }

  if (error) return <ErrorState description={error.message} onRetry={refetch} />

  return (
    <div className="space-y-4">
      <PageHeader
        title="Audit Log"
        description="不可变安全 / 运维日志。支持按 actor/action/decision 过滤 + CSV / JSON 导出。"
      >
        <Button size="sm" variant="outline" onClick={exportCsv} disabled={!data?.items.length}>
          <Download className="mr-1.5 h-3.5 w-3.5" />
          CSV
        </Button>
        <Button size="sm" variant="outline" onClick={exportJson} disabled={!data?.items.length}>
          <FileJson className="mr-1.5 h-3.5 w-3.5" />
          JSON
        </Button>
      </PageHeader>

      <div className="rounded-lg border bg-card p-4">
        <div className="grid gap-4 md:grid-cols-3 lg:grid-cols-5">
          <div className="space-y-1.5">
            <Label htmlFor="audit-action" className="text-xs">Action</Label>
            <Input
              id="audit-action"
              placeholder="session.create"
              value={action}
              onChange={(e) => setAction(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="audit-decision" className="text-xs">Decision</Label>
            <Select value={decision || '_all'} onValueChange={(v) => setDecision(v === '_all' || v === null ? '' : v)}>
              <SelectTrigger id="audit-decision">
                <SelectValue placeholder="All" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="_all">All</SelectItem>
                <SelectItem value="allow">allow</SelectItem>
                <SelectItem value="deny">deny</SelectItem>
                <SelectItem value="approved">approved</SelectItem>
                <SelectItem value="rejected">rejected</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="audit-actor" className="text-xs">Actor</Label>
            <Input
              id="audit-actor"
              placeholder="user / system:control-plane"
              value={actor}
              onChange={(e) => setActor(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="audit-from" className="text-xs">From</Label>
            <Input
              id="audit-from"
              type="datetime-local"
              value={from}
              onChange={(e) => setFrom(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="audit-to" className="text-xs">To</Label>
            <Input
              id="audit-to"
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
          <Button size="sm" onClick={handleSearch}>
            <Search className="mr-1.5 h-4 w-4" />
            Search
          </Button>
        </div>
      </div>

      <DataTable columns={columns} data={data?.items || []} loading={isLoading} />
    </div>
  )
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}