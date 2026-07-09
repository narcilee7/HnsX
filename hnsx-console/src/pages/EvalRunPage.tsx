import { useParams, Link } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { DataTable } from '@/components/ui/DataTable'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { Empty } from '@/components/ui/Empty'
import { useEvalRun, useEvalSet } from '@/hooks/useEvals'
import type { JsonValue } from '@bufbuild/protobuf'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { ArrowLeft, CheckCircle2, XCircle } from 'lucide-react'

interface CaseRow {
  id: string
  passed: boolean
  score: number
  actual: string
  details: string
}

function formatDuration(ms: number | undefined): string {
  if (ms === undefined || Number.isNaN(ms)) return '-'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

function renderJson(value: JsonValue): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

export default function EvalRunPage() {
  const { domainId, setId, runId } = useParams<{
    domainId: string
    setId: string
    runId: string
  }>()
  const {
    data: run,
    isLoading: runLoading,
    error: runError,
    refetch,
  } = useEvalRun(domainId, setId, runId)
  const { data: evalSet } = useEvalSet(domainId, setId)

  const caseRows: CaseRow[] = useMemo(() => {
    if (!run) return []
    return run.cases.map((c) => {
      const record = c as Record<string, unknown>
      return {
        id: String(record.caseId ?? record.case_id ?? ''),
        passed: Boolean(record.passed),
        score: Number(record.score ?? 0),
        actual: String(record.actual ?? ''),
        details: String(record.details ?? ''),
      }
    })
  }, [run])

  const columns = useMemo<ColumnDef<CaseRow>[]>(
    () => [
      {
        accessorKey: 'id',
        header: 'Case',
        cell: ({ row }) => {
          const caseName = evalSet?.cases.find((c) => c.id === row.original.id)?.name
          return <span className="font-medium">{caseName || row.original.id}</span>
        },
      },
      {
        accessorKey: 'passed',
        header: 'Result',
        cell: ({ row }) =>
          row.original.passed ? (
            <span className="inline-flex items-center gap-1 text-sm font-medium text-green-600">
              <CheckCircle2 className="h-4 w-4" />
              Passed
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 text-sm font-medium text-destructive">
              <XCircle className="h-4 w-4" />
              Failed
            </span>
          ),
      },
      {
        accessorKey: 'score',
        header: 'Score',
        cell: ({ row }) => `${(row.original.score * 100).toFixed(1)}%`,
      },
      {
        accessorKey: 'actual',
        header: 'Actual',
        cell: ({ row }) => (
          <pre className="max-w-xs truncate text-xs">{renderJson(row.original.actual as JsonValue)}</pre>
        ),
      },
      {
        accessorKey: 'details',
        header: 'Details',
        cell: ({ row }) => (
          <pre className="max-w-xs truncate text-xs text-muted-foreground">
            {row.original.details || '-'}
          </pre>
        ),
      },
    ],
    [evalSet],
  )

  if (runLoading) return <Loading />
  if (runError || !run) {
    return <ErrorState description={runError?.message || 'Eval run not found'} onRetry={refetch} />
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={`Eval Run: ${run.id}`}
        description={`Report for eval set ${run.setId}.`}
        breadcrumbs={[
          { label: 'Evals', href: '/evals' },
          { label: run.setId, href: `/domains/${domainId}/evals/${setId}` },
          { label: run.id },
        ]}
      >
        <Button variant="outline" size="sm" asChild>
          <Link
            to={`/domains/${domainId}/evals/${setId}`}
            className="no-underline"
          >
            <ArrowLeft className="mr-1.5 h-4 w-4" />
            Back
          </Link>
        </Button>
      </PageHeader>

      <div className="grid gap-4 lg:grid-cols-5 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Score</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold">{(run.score * 100).toFixed(1)}%</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Passed</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold">
              {run.passed}<span className="text-base font-normal text-muted-foreground">/{run.total}</span>
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Pass Rate</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold">
              {run.total > 0 ? ((run.passed / run.total) * 100).toFixed(1) : 0}%
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Cost</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold">${run.totalCostUsd.toFixed(4)}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Duration</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold">{formatDuration(run.durationMs)}</div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Run Details</CardTitle>
          <Badge variant={run.state === 'completed' ? 'default' : 'secondary'}>{run.state}</Badge>
        </CardHeader>        
        <CardContent className="grid gap-4 text-sm md:grid-cols-2">
          <div className="flex justify-between">
            <span className="text-muted-foreground">Baseline Run</span>
            <span>
              {run.baselineRunId ? (
                <Link
                  to={`/domains/${domainId}/evals/${setId}/runs/${run.baselineRunId}`}
                  className="font-medium hover:underline"
                >
                  {run.baselineRunId}
                </Link>
              ) : (
                <span className="text-muted-foreground">No baseline configured</span>
              )}
            </span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Domain</span>
            <Link to={`/domains/${run.domainId}`} className="font-medium hover:underline">
              {run.domainId}
            </Link>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Case Results</CardTitle>
        </CardHeader>
        <CardContent>
          {caseRows.length === 0 ? (
            <Empty description="No case results available." />
          ) : (
            <DataTable columns={columns} data={caseRows} />
          )}
        </CardContent>
      </Card>
    </div>
  )
}
