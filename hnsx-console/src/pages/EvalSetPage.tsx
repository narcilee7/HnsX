import { useParams, Link, useNavigate } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { DataTable } from '@/components/ui/DataTable'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { Empty } from '@/components/ui/Empty'
import { useEvalSet, useEvalRuns, useRunEval } from '@/hooks/useEvals'
import type { EvalRunViewModel } from '@/api/mappers'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { Play, ArrowLeft } from 'lucide-react'

function formatDuration(ms: number | undefined): string {
  if (ms === undefined || Number.isNaN(ms)) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

export default function EvalSetPage() {
  const { domainId, setId } = useParams<{ domainId: string; setId: string }>()
  const navigate = useNavigate()
  const {
    data: evalSet,
    isLoading: setLoading,
    error: setError,
    refetch: refetchSet,
  } = useEvalSet(domainId, setId)
  const { data: runsData, isLoading: runsLoading, refetch: refetchRuns } = useEvalRuns(domainId, setId, {
    limit: 20,
  })
  const runEval = useRunEval(domainId, setId)

  const runColumns = useMemo<ColumnDef<EvalRunViewModel>[]>(
    () => [
      {
        accessorKey: 'id',
        header: 'Run ID',
        cell: ({ row }) => (
          <Link
            to={`/domains/${domainId}/evals/${setId}/runs/${row.original.id}`}
            className="font-medium hover:underline"
          >
            {row.original.id}
          </Link>
        ),
      },
      {
        accessorKey: 'state',
        header: 'State',
        cell: ({ row }) => (
          <Badge variant={row.original.state === 'completed' ? 'default' : 'secondary'}>
            {row.original.state}
          </Badge>
        ),
      },
      {
        accessorKey: 'score',
        header: 'Score',
        cell: ({ row }) => `${(row.original.score * 100).toFixed(1)}%`,
      },
      {
        accessorKey: 'passed',
        header: 'Passed',
        cell: ({ row }) => `${row.original.passed}/${row.original.total}`,
      },
      {
        accessorKey: 'totalCostUsd',
        header: 'Cost',
        cell: ({ row }) => `$${row.original.totalCostUsd.toFixed(4)}`,
      },
      {
        accessorKey: 'durationMs',
        header: 'Duration',
        cell: ({ row }) => formatDuration(row.original.durationMs),
      },
      {
        accessorKey: 'baselineRunId',
        header: 'Baseline',
        cell: ({ row }) => row.original.baselineRunId || <span className="text-muted-foreground">-</span>,
      },
    ],
    [domainId, setId],
  )

  const handleRun = async () => {
    const res = await runEval.mutateAsync({})
    navigate(`/domains/${domainId}/evals/${setId}/runs/${res.eval_run_id}`)
  }

  if (setLoading) return <Loading />
  if (setError || !evalSet) {
    return <ErrorState description={setError?.message || 'Eval set not found'} onRetry={refetchSet} />
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={`Eval Set: ${evalSet.id}`}
        description={evalSet.description || 'Cases and run reports.'}
        breadcrumbs={[
          { label: 'Evals', href: '/evals' },
          { label: evalSet.id },
        ]}
      >
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={handleRun} disabled={runEval.isPending}>
            <Play className="mr-1.5 h-4 w-4" />
            Run Eval
          </Button>
          <Button variant="outline" size="sm" asChild>
            <Link to="/evals" className="no-underline">
              <ArrowLeft className="mr-1.5 h-4 w-4" />
              Back
            </Link>
          </Button>
        </div>
      </PageHeader>

      <div className="grid gap-4 lg:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Domain</CardTitle>
          </CardHeader>
          <CardContent>
            <Link to={`/domains/${evalSet.domainId}`} className="font-medium hover:underline">
              {evalSet.domainId}
            </Link>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Cases</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-lg font-semibold">{evalSet.cases.length}</div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Cases</CardTitle>
        </CardHeader>
        <CardContent>
          {evalSet.cases.length === 0 ? (
            <Empty description="No cases in this eval set." />
          ) : (
            <div className="divide-y">
              {evalSet.cases.map((c) => (
                <div key={c.id} className="space-y-2 py-4 first:pt-0 last:pb-0">
                  <div className="flex items-center justify-between">
                    <span className="font-medium">{c.name || c.id}</span>
                    <Badge variant="outline">{c.id}</Badge>
                  </div>
                  <div className="grid gap-4 md:grid-cols-2">
                    <div className="rounded-md bg-muted p-3">
                      <div className="mb-1 text-xs font-medium text-muted-foreground">Input</div>
                      <pre className="whitespace-pre-wrap text-xs">{c.input}</pre>
                    </div>
                    <div className="rounded-md bg-muted p-3">
                      <div className="mb-1 text-xs font-medium text-muted-foreground">Expected</div>
                      <pre className="whitespace-pre-wrap text-xs">{c.expect}</pre>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Run History</CardTitle>
          {runsData && (
            <Button variant="ghost" size="sm" onClick={() => refetchRuns()}>
              Refresh
            </Button>
          )}
        </CardHeader>
        <CardContent>
          <DataTable
            columns={runColumns}
            data={runsData?.items || []}
            loading={runsLoading}
          />
        </CardContent>
      </Card>
    </div>
  )
}
