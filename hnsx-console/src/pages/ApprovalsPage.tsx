import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ColumnDef } from '@tanstack/react-table'
import { Button, buttonVariants } from '@/components/ui/button'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable } from '@/components/ui/DataTable'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { Empty } from '@/components/ui/Empty'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useApprovals, useResolveApproval } from '@/hooks/useApprovals'
import type { Approval } from '@/api/approvals'
import { Eye, Check, X, AlertTriangle, ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils'

export default function ApprovalsPage() {
  const [statusFilter, setStatusFilter] = useState('pending')
  const { data, isLoading, error, refetch } = useApprovals({ status: statusFilter || undefined, limit: 50 })
  const { mutate: resolve } = useResolveApproval()
  const [activeApproval, setActiveApproval] = useState<Approval | null>(null)
  const [comment, setComment] = useState('')

  const columns = useMemo<ColumnDef<Approval>[]>(
    () => [
      {
        accessorKey: 'session_id',
        header: 'Session',
        cell: ({ row }) => (
          <Link
            to={`/sessions/${row.original.session_id}`}
            className="font-mono text-xs font-medium hover:underline"
          >
            {row.original.session_id.slice(0, 16)}
          </Link>
        ),
      },
      { accessorKey: 'step_id', header: 'Step',
        cell: ({ row }) => row.original.step_id ? <span className="font-mono text-[10px]">{row.original.step_id.slice(0, 16)}</span> : <span className="text-muted-foreground">—</span>,
      },
      { accessorKey: 'requested_action', header: 'Action',
        cell: ({ row }) => <span className="truncate font-mono text-xs">{row.original.requested_action}</span>,
      },
      {
        accessorKey: 'risk_description',
        header: 'Risk',
        cell: ({ row }) =>
          row.original.risk_description ? (
            <div className="flex items-center gap-1.5 text-xs">
              <AlertTriangle className="h-3 w-3 shrink-0 text-[var(--warning)]" />
              <span className="truncate">{row.original.risk_description}</span>
            </div>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      },
      {
        accessorKey: 'status',
        header: 'Status',
        cell: ({ row }) => <StatusBadge status={row.original.status} />,
      },
      {
        accessorKey: 'created_at',
        header: 'Created',
        cell: ({ row }) => <Timestamp date={new Date(row.original.created_at)} />,
      },
      {
        id: 'actions',
        header: 'Actions',
        cell: ({ row }) => (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => {
                setActiveApproval(row.original)
                setComment('')
              }}
              aria-label="View detail"
              title="View detail"
            >
              <Eye className="h-3.5 w-3.5" />
            </Button>
            {row.original.status === 'pending' && (
              <>
                <Button
                  size="sm"
                  variant="default"
                  onClick={() =>
                    resolve({ id: row.original.id, decision: 'approve' })
                  }
                >
                  <Check className="mr-1 h-3 w-3" />
                  Approve
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() =>
                    resolve({ id: row.original.id, decision: 'reject' })
                  }
                >
                  <X className="mr-1 h-3 w-3" />
                  Reject
                </Button>
              </>
            )}
          </div>
        ),
      },
    ],
    [resolve],
  )

  if (error) {
    return <ErrorState description={error.message} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title="Approvals"
        description="Human-in-the-loop 待审批事项。点击行查看风险详情后决策。"
      >
        <Select value={statusFilter} onValueChange={(v) => setStatusFilter(v || 'pending')}>
          <SelectTrigger className="w-36">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">All</SelectItem>
            <SelectItem value="pending">Pending</SelectItem>
            <SelectItem value="approved">Approved</SelectItem>
            <SelectItem value="rejected">Rejected</SelectItem>
          </SelectContent>
        </Select>
      </PageHeader>

      {!isLoading && (data?.items.length ?? 0) === 0 ? (
        <Empty description={statusFilter === 'pending' ? '当前没有待审批事项。' : '没有匹配的审批记录。'} />
      ) : (
        <DataTable columns={columns} data={data?.items || []} loading={isLoading} />
      )}

      <ApprovalDetailDialog
        approval={activeApproval}
        comment={comment}
        onCommentChange={setComment}
        onClose={() => {
          setActiveApproval(null)
          setComment('')
        }}
        onResolve={(decision) => {
          if (!activeApproval) return
          resolve(
            { id: activeApproval.id, decision, comment: comment || undefined },
            {
              onSuccess: () => {
                setActiveApproval(null)
                setComment('')
              },
            },
          )
        }}
      />
    </div>
  )
}

interface ApprovalDetailDialogProps {
  approval: Approval | null
  comment: string
  onCommentChange: (v: string) => void
  onClose: () => void
  onResolve: (decision: 'approve' | 'reject') => void
}

function ApprovalDetailDialog({
  approval,
  comment,
  onCommentChange,
  onClose,
  onResolve,
}: ApprovalDetailDialogProps) {
  const open = approval !== null
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        {approval && (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-[var(--warning)]" />
                审批详情
              </DialogTitle>
              <DialogDescription>
                ID <span className="font-mono">{approval.id}</span>
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4 py-2">
              {/* Risk banner */}
              {approval.risk_description && (
                <div className="flex items-start gap-2 rounded-md border border-[var(--warning)]/40 bg-[var(--warning-soft)] p-3 text-sm text-[var(--warning-text)]">
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                  <div>
                    <p className="font-medium">风险提示</p>
                    <p className="mt-0.5 text-[var(--chart-text-secondary)]">{approval.risk_description}</p>
                  </div>
                </div>
              )}

              {/* Action detail */}
              <div className="space-y-1">
                <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">Requested Action</p>
                <pre className="overflow-x-auto rounded-md bg-muted p-3 text-xs">
                  {approval.requested_action}
                </pre>
              </div>

              {/* Meta grid */}
              <div className="grid grid-cols-2 gap-3 text-sm">
                <Field label="Session">
                  <Link
                    to={`/sessions/${approval.session_id}`}
                    className="inline-flex items-center gap-1 font-mono text-xs hover:underline"
                  >
                    {approval.session_id}
                    <ExternalLink className="h-3 w-3" />
                  </Link>
                </Field>
                <Field label="Step">
                  <span className="font-mono text-xs">
                    {approval.step_id ?? <span className="text-muted-foreground">—</span>}
                  </span>
                </Field>
                <Field label="Status">
                  <StatusBadge status={approval.status} />
                </Field>
                <Field label="Created">
                  <Timestamp date={new Date(approval.created_at)} />
                </Field>
                {approval.resolver && (
                  <Field label="Resolver">
                    <span className="text-xs">{approval.resolver}</span>
                  </Field>
                )}
                {approval.resolved_at && (
                  <Field label="Resolved">
                    <Timestamp date={new Date(approval.resolved_at)} />
                  </Field>
                )}
              </div>

              {/* Comment input */}
              {approval.status === 'pending' && (
                <div className="space-y-1.5">
                  <label className="text-xs font-medium text-muted-foreground">Comment (optional)</label>
                  <textarea
                    value={comment}
                    onChange={(e) => onCommentChange(e.target.value)}
                    placeholder="审批意见 / 拒绝原因"
                    className="min-h-[60px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm"
                  />
                </div>
              )}
            </div>

            <DialogFooter>
              {approval.status === 'pending' ? (
                <>
                  <Button variant="outline" onClick={onClose}>
                    Cancel
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => onResolve('reject')}
                    className={cn(buttonVariants({ variant: 'outline' }))}
                  >
                    <X className="mr-1 h-3.5 w-3.5" />
                    Reject
                  </Button>
                  <Button onClick={() => onResolve('approve')}>
                    <Check className="mr-1 h-3.5 w-3.5" />
                    Approve
                  </Button>
                </>
              ) : (
                <Button variant="outline" onClick={onClose}>
                  Close
                </Button>
              )}
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-0.5">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div>{children}</div>
    </div>
  )
}