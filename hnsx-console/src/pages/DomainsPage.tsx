import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import type { ColumnDef } from '@tanstack/react-table'
import { Button } from '@/components/ui/button'
import { Plus } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable } from '@/components/ui/DataTable'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { useDomains } from '@/hooks/useDomains'
import type { DomainSummary } from '@/api/mappers'

export default function DomainsPage() {
  const { data, isLoading, error, refetch } = useDomains({ limit: 50 })

  const columns = useMemo<ColumnDef<DomainSummary>[]>(
    () => [
      {
        accessorKey: 'id',
        header: 'ID',
        cell: ({ row }) => (
          <Link to={`/domains/${row.original.id}`} className="font-medium hover:underline">
            {row.original.id}
          </Link>
        ),
      },
      { accessorKey: 'version', header: 'Version' },
      { accessorKey: 'description', header: 'Description' },
      {
        accessorKey: 'status',
        header: 'Status',
        cell: ({ row }) => <StatusBadge status={row.original.status} />,
      },
      {
        accessorKey: 'updatedAt',
        header: 'Last Updated',
        cell: ({ row }) => <Timestamp date={row.original.updatedAt} />,
      },
    ],
    [],
  )

  if (error) {
    return <ErrorState description={error.message} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader title="Domains" description="Manage harness domain definitions.">
        <Button>
          <Plus className="mr-2 h-4 w-4" /> Register Domain
        </Button>
      </PageHeader>
      <DataTable columns={columns} data={data?.items || []} loading={isLoading} />
    </div>
  )
}
