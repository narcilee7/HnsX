import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import type { ColumnDef } from '@tanstack/react-table'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable } from '@/components/ui/DataTable'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { useSessions } from '@/hooks/useSessions'
import type { SessionViewModel } from '@/api/mappers'

export default function SessionsPage() {
  const { data, isLoading, error, refetch } = useSessions({ limit: 50 })

  const columns = useMemo<ColumnDef<SessionViewModel>[]>(
    () => [
      {
        accessorKey: 'id',
        header: 'ID',
        cell: ({ row }) => (
          <Link to={`/sessions/${row.original.id}`} className="font-medium hover:underline">
            {row.original.id}
          </Link>
        ),
      },
      { accessorKey: 'domainId', header: 'Domain' },
      {
        accessorKey: 'state',
        header: 'State',
        cell: ({ row }) => <StatusBadge status={row.original.state} />,
      },
      {
        accessorKey: 'startedAt',
        header: 'Started At',
        cell: ({ row }) => <Timestamp date={row.original.startedAt} />,
      },
    ],
    [],
  )

  if (error) {
    return <ErrorState description={error.message} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader title="Sessions" description="Monitor and inspect sessions." />
      <DataTable columns={columns} data={data?.items || []} loading={isLoading} />
    </div>
  )
}
