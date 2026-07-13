import { lazy, Suspense, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import type { ColumnDef } from '@tanstack/react-table'
import { Button } from '@/components/ui/button'
import { Plus } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { DataTable } from '@/components/ui/DataTable'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { useDomains, useCreateDomainYaml } from '@/hooks/useDomains'
import type { DomainSummary } from '@/api/mappers'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from 'sonner'

const DEFAULT_DOMAIN_YAML = `id: new-domain
version: "1.0.0"
description: ""
harness:
  agents:
    assistant:
      id: assistant
      provider: noop
      adapter:
        kind: noop
      system_prompt: "You are a helpful assistant."
  prompts:
    default:
      id: default
      type: system
      template: "You are a helpful assistant."
  sandbox:
    policy: none
  session:
    mode: single
    agent: assistant
`

export default function DomainsPage() {
  const { data, isLoading, error, refetch } = useDomains({ limit: 50 })
  const create = useCreateDomainYaml()
  const [open, setOpen] = useState(false)
  const [yaml, setYaml] = useState(DEFAULT_DOMAIN_YAML)

  const Editor = lazy(() => import('@monaco-editor/react'))

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

  const handleRegister = () => {
    create.mutate(yaml, {
      onSuccess: () => {
        setOpen(false)
        setYaml(DEFAULT_DOMAIN_YAML)
        refetch()
        toast.success('Domain registered')
      },
    })
  }

  if (error) {
    return <ErrorState description={error.message} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader title="Domains" description="Manage harness domain definitions.">
        <Dialog open={open} onOpenChange={setOpen}>
          <Button onClick={() => setOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Register Domain
          </Button>
          <DialogContent className="sm:max-w-2xl">
            <DialogHeader>
              <DialogTitle>Register Domain</DialogTitle>
              <DialogDescription>
                Paste the domain YAML in the Monaco editor below. <strong>Commit</strong>
                sends it to the server, which validates and persists it. This replaces
                the CLI's <code>hnsx domain apply</code>.
              </DialogDescription>
            </DialogHeader>
            <Suspense
              fallback={
                <div className="min-h-[320px] rounded-md border bg-muted/30 p-3 font-mono text-xs text-muted-foreground">
                  loading editor…
                </div>
              }
            >
              <Editor
                height="320px"
                defaultLanguage="yaml"
                theme="vs-light"
                value={yaml}
                onChange={(v) => setYaml(v ?? '')}
                options={{
                  minimap: { enabled: false },
                  fontSize: 12,
                  tabSize: 2,
                  insertSpaces: true,
                }}
              />
            </Suspense>
            <DialogFooter>
              <Button variant="outline" onClick={() => setOpen(false)}>Cancel</Button>
              <Button onClick={handleRegister} disabled={create.isPending || !yaml.trim()}>
                {create.isPending ? 'Registering…' : 'Commit'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </PageHeader>
      <DataTable columns={columns} data={data?.items || []} loading={isLoading} />
    </div>
  )
}
