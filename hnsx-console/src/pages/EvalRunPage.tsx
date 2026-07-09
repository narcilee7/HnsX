import { useParams, Link } from 'react-router-dom'
import { useMemo } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { DataTable } from '@/components/ui/DataTable'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { Empty } from '@/components/ui/Empty'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useEvalRun, useEvalRuns, useEvalSet } from '@/hooks/useEvals'
import type { JsonValue } from '@bufbuild/protobuf'
import { LineChart, Line, ResponsiveContainer, Tooltip, XAxis, YAxis, CartesianGrid, ReferenceLine } from 'recharts'
import { ArrowLeft, CheckCircle2, XCircle, GitCompare, Minus } from 'lucide-react'

interface CaseRow {
  id: string
  passed: boolean
  score: number
  actual: string
  details: string
  baselinePassed?: boolean
  baselineScore?: number
  diff?: 'improved' | 'regressed' | 'unchanged' | 'new'
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
  const { data: runsList } = useEvalRuns(domainId, setId, { limit: 50 })

  /* ----- score trend data ----- */
  const trendData = useMemo(() => {
    if (!runsList?.items) return []
    return [...runsList.items]
      .sort((a, b) => a.id.localeCompare(b.id))
      .map((r) => ({
        runId: r.id,
        score: Math.round(r.score * 1000) / 10, // % with 1 digit
        passed: r.passed,
        total: r.total,
        baseline: r.id === run?.baselineRunId,
        current: r.id === runId,
      }))
  }, [runsList, run?.baselineRunId, runId])

  const baselineScore = useMemo(() => {
    if (!run?.baselineRunId) return undefined
    return trendData.find((d) => d.runId === run.baselineRunId)?.score
  }, [run?.baselineRunId, trendData])

  /* ----- baseline data ----- */
  const baselineData = useMemo(() => {
    if (!run?.baselineRunId || !runsList?.items) return null
    const baseline = runsList.items.find((r) => r.id === run.baselineRunId)
    if (!baseline) return null
    const map = new Map<string, { passed: boolean; score: number }>()
    for (const c of baseline.cases as Array<Record<string, unknown>>) {
      map.set(String(c.caseId ?? c.case_id ?? ''), {
        passed: Boolean(c.passed),
        score: Number(c.score ?? 0),
      })
    }
    return map
  }, [run?.baselineRunId, runsList])

  /* ----- case rows w/ diff ----- */
  const caseRows: CaseRow[] = useMemo(() => {
    if (!run) return []
    return run.cases.map((c) => {
      const record = c as Record<string, unknown>
      const id = String(record.caseId ?? record.case_id ?? '')
      const passed = Boolean(record.passed)
      const score = Number(record.score ?? 0)
      const baseline = baselineData?.get(id)
      let diff: CaseRow['diff'] = undefined
      if (baselineData) {
        if (!baseline) diff = 'new'
        else if (passed && !baseline.passed) diff = 'improved'
        else if (!passed && baseline.passed) diff = 'regressed'
        else diff = Math.abs(score - baseline.score) > 0.01 ? 'improved' : 'unchanged'
      }
      return {
        id,
        passed,
        score,
        actual: String(record.actual ?? ''),
        details: String(record.details ?? ''),
        baselinePassed: baseline?.passed,
        baselineScore: baseline?.score,
        diff,
      }
    })
  }, [run, baselineData])

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
            <span className="inline-flex items-center gap-1 text-sm font-medium text-[var(--success-text)]">
              <CheckCircle2 className="h-4 w-4" />
              Passed
            </span>
          ) : (
            <span className="inline-flex items-center gap-1 text-sm font-medium text-[var(--danger-text)]">
              <XCircle className="h-4 w-4" />
              Failed
            </span>
          ),
      },
      {
        accessorKey: 'score',
        header: 'Score',
        cell: ({ row }) => (
          <span className="tabular-nums">{(row.original.score * 100).toFixed(1)}%</span>
        ),
      },
      ...(baselineData
        ? [
            {
              id: 'diff',
              header: 'Diff vs Baseline',
              cell: ({ row }: { row: { original: CaseRow } }) => {
                const r = row.original
                if (r.diff === undefined) return null
                const icon =
                  r.diff === 'improved' ? (
                    <CheckCircle2 className="h-3.5 w-3.5 text-[var(--success)]" />
                  ) : r.diff === 'regressed' ? (
                    <XCircle className="h-3.5 w-3.5 text-[var(--danger)]" />
                  ) : r.diff === 'new' ? (
                    <span className="text-[10px] uppercase tracking-wider text-[var(--info-text)]">NEW</span>
                  ) : (
                    <Minus className="h-3.5 w-3.5 text-[var(--chart-text-muted)]" />
                  )
                const label =
                  r.diff === 'unchanged'
                    ? '—'
                    : r.diff === 'new'
                      ? '新增'
                      : `${r.diff === 'improved' ? '+' : '-'}${(
                          ((r.score - (r.baselineScore ?? 0)) * 100
                        ).toFixed(1))}%`
                const tone =
                  r.diff === 'improved'
                    ? 'text-[var(--success-text)]'
                    : r.diff === 'regressed'
                      ? 'text-[var(--danger-text)]'
                      : 'text-[var(--chart-text-secondary)]'
                return (
                  <span className={`inline-flex items-center gap-1 text-xs tabular-nums ${tone}`}>
                    {icon}
                    {label}
                  </span>
                )
              },
            } as ColumnDef<CaseRow>,
          ]
        : []),
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
    [evalSet, baselineData],
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

      {/* Score trend */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-sm">
            <GitCompare className="h-4 w-4" />
            Score Trend · {trendData.length} runs
          </CardTitle>
        </CardHeader>
        <CardContent>
          {trendData.length === 0 ? (
            <p className="text-sm text-muted-foreground">No other runs to compare.</p>
          ) : (
            <ResponsiveContainer width="100%" height={180}>
              <LineChart data={trendData} margin={{ top: 8, right: 12, bottom: 4, left: 4 }}>
                <CartesianGrid stroke="var(--chart-grid)" strokeDasharray="2 4" vertical={false} />
                <XAxis
                  dataKey="runId"
                  tick={{ fill: 'var(--chart-text-muted)', fontSize: 10 }}
                  tickLine={false}
                  axisLine={{ stroke: 'var(--chart-baseline)' }}
                  tickFormatter={(v) => String(v).slice(0, 8)}
                />
                <YAxis
                  domain={[0, 100]}
                  tick={{ fill: 'var(--chart-text-muted)', fontSize: 10 }}
                  tickLine={false}
                  axisLine={false}
                  tickFormatter={(v) => `${v}%`}
                  width={48}
                />
                <Tooltip
                  content={({ active, payload }) => {
                    if (!active || !payload?.length) return null
                    const p = payload[0]?.payload as { runId: string; score: number; passed: number; total: number; baseline?: boolean; current?: boolean }
                    return (
                      <div className="rounded-md border border-[var(--chart-tooltip-border)] bg-[var(--chart-tooltip-bg)] px-3 py-2 text-xs shadow-lg">
                        <div className="font-medium">{p.runId}</div>
                        <div className="text-[var(--chart-text-secondary)]">
                          score: <span className="tabular-nums">{p.score.toFixed(1)}%</span>
                        </div>
                        <div className="text-[var(--chart-text-secondary)]">
                          passed: {p.passed}/{p.total}
                        </div>
                        {p.baseline && <div className="text-[var(--info-text)]">baseline</div>}
                        {p.current && <div className="text-[var(--chart-1)]">current</div>}
                      </div>
                    )
                  }}
                />
                {baselineScore !== undefined && (
                  <ReferenceLine
                    y={baselineScore}
                    stroke="var(--info)"
                    strokeDasharray="4 4"
                    label={{ value: 'baseline', fill: 'var(--info-text)', fontSize: 10, position: 'right' }}
                  />
                )}
                <Line
                  type="monotone"
                  dataKey="score"
                  stroke="var(--chart-1)"
                  strokeWidth={1.75}
                  dot={(p) => {
                    const point = p as { payload?: { current?: boolean; baseline?: boolean } }
                    const isCurrent = point.payload?.current
                    const isBaseline = point.payload?.baseline
                    return (
                      <circle
                        key={`dot-${p.cx}-${p.cy}`}
                        cx={p.cx}
                        cy={p.cy}
                        r={isCurrent || isBaseline ? 4 : 2.5}
                        fill={isCurrent ? 'var(--chart-1)' : isBaseline ? 'var(--info)' : 'var(--chart-1)'}
                        stroke="var(--card)"
                        strokeWidth={isCurrent || isBaseline ? 2 : 0}
                      />
                    )
                  }}
                  isAnimationActive={false}
                />
              </LineChart>
            </ResponsiveContainer>
          )}
        </CardContent>
      </Card>

      {/* Stats row */}
      <div className="grid gap-4 lg:grid-cols-5 md:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Score</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold tabular-nums">{(run.score * 100).toFixed(1)}%</div>
            {baselineScore !== undefined && (
              <p className="mt-1 text-xs text-muted-foreground tabular-nums">
                baseline {baselineScore.toFixed(1)}%
                {' · '}
                <span
                  className={
                    run.score * 100 - baselineScore >= 0
                      ? 'text-[var(--success-text)]'
                      : 'text-[var(--danger-text)]'
                  }
                >
                  {run.score * 100 - baselineScore >= 0 ? '+' : ''}
                  {(run.score * 100 - baselineScore).toFixed(1)}%
                </span>
              </p>
            )}
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
            <div className="text-2xl font-semibold tabular-nums">
              {run.total > 0 ? ((run.passed / run.total) * 100).toFixed(1) : 0}%
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Cost</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold tabular-nums">${run.totalCostUsd.toFixed(4)}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Duration</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-semibold tabular-nums">{formatDuration(run.durationMs)}</div>
          </CardContent>
        </Card>
      </div>

      {/* Run Details */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-base">Run Details</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 text-sm md:grid-cols-2">
          <div className="flex justify-between">
            <span className="text-muted-foreground">Baseline Run</span>
            <Select
              value={run.baselineRunId || '_none'}
              onValueChange={() => {
                /* baseline change would re-call runEval with baseline_run_id; UI hook not yet wired */
              }}
            >
              <SelectTrigger className="h-7 w-auto min-w-[12rem] text-xs">
                <SelectValue placeholder="No baseline" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="_none">No baseline</SelectItem>
                {runsList?.items
                  .filter((r) => r.id !== run.id)
                  .map((r) => (
                    <SelectItem key={r.id} value={r.id}>
                      {r.id.slice(0, 24)}… ({(r.score * 100).toFixed(0)}%)
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Domain</span>
            <Link to={`/domains/${run.domainId}`} className="font-medium hover:underline">
              {run.domainId}
            </Link>
          </div>
        </CardContent>
      </Card>

      {/* Case Results */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-base">Case Results</CardTitle>
          {baselineData && (
            <span className="text-xs text-muted-foreground">
              与 baseline{' '}
              <span className="font-mono">{run.baselineRunId?.slice(0, 12)}</span> 对比
            </span>
          )}
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