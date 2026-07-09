import { useMemo } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable } from '@/components/ui/DataTable'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { useApprovals, useResolveApproval } from '@/hooks/useApprovals'
import type { Approval } from '@/api/approvals'

export default function ApprovalsPage() {
  const { data, isLoading, error, refetch } = useApprovals({ status: 'pending', limit: 50 })
  const { mutate: resolve } = useResolveApproval()

  const columns = useMemo<ColumnDef<Approval>[]>(
    () => [
      { accessorKey: 'session_id', header: 'Session' },
      { accessorKey: 'step_id', header: 'Step' },
      { accessorKey: 'requested_action', header: 'Action' },
      { accessorKey: 'risk_description', header: 'Risk' },
      {
        accessorKey: 'status',
        header: 'Status',
        cell: ({ row }) => <StatusBadge status={row.original.status} />,
      },
      {
        accessorKey: 'created_at',
        header: 'Created',
        cell: ({ row }) => <Timestamp date={row.original.created_at} />,
      },
      {
        id: 'actions',
        header: 'Actions',
        cell: ({ row }) =>
          row.original.status === 'pending' ? (
            <div className="flex gap-2">
              <Button size="sm" onClick={() => resolve({ id: row.original.id, decision: 'approve' })}>
                Approve
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => resolve({ id: row.original.id, decision: 'reject' })}
              >
                Reject
              </Button>
            </div>
          ) : null,
      },
    ],
    [resolve],
  )

  if (error) {
    return <ErrorState description={error.message} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader title="Approvals" description="Review and resolve human-in-the-loop requests." />
      <DataTable columns={columns} data={data?.items || []} loading={isLoading} />
    </div>
  )
}
