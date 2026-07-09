import { useMemo } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable } from '@/components/ui/DataTable'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { useAuditLogs } from '@/hooks/useAudit'
import type { AuditLogViewModel } from '@/api/mappers'

export default function AuditPage() {
  const { data, isLoading, error, refetch } = useAuditLogs({ limit: 50 })

  const columns = useMemo<ColumnDef<AuditLogViewModel>[]>(
    () => [
      {
        accessorKey: 'timestamp',
        header: 'Timestamp',
        cell: ({ row }) => <Timestamp date={row.original.timestamp} />,
      },
      { accessorKey: 'action', header: 'Action' },
      { accessorKey: 'actor', header: 'Actor' },
      { accessorKey: 'resource', header: 'Resource' },
      { accessorKey: 'decision', header: 'Decision' },
      { accessorKey: 'reason', header: 'Reason' },
    ],
    [],
  )

  if (error) {
    return <ErrorState description={error.message} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader title="Audit Log" description="Immutable security and operations log." />
      <DataTable columns={columns} data={data?.items || []} loading={isLoading} />
    </div>
  )
}
