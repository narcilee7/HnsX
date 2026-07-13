// Cross-session observability launchpad.
//
// Shows the most recent sessions and traces across all domains with
// filtering by domain + agent. Clicking a session navigates to its
// SessionDetailPage (which streams live observations via SSE).
//
// Phase 5 of the CLI-optional plan: replaces ad-hoc "let me grep the
// logs" workflows with a UI-first debug entry point.
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { useSessions } from '@/hooks/useSessions'
import { useTraces } from '@/hooks/useTraces'

export default function ObservabilityDebugPage() {
  const [domainFilter, setDomainFilter] = useState('')
  const [agentFilter, setAgentFilter] = useState('')

  const sessionParams = useMemo(
    () => ({
      limit: 25,
      ...(domainFilter ? { domain: domainFilter } : {}),
    }),
    [domainFilter],
  )
  const traceParams = useMemo(
    () => ({
      limit: 25,
      ...(domainFilter ? { domain: domainFilter } : {}),
      ...(agentFilter ? { agent: agentFilter } : {}),
    }),
    [domainFilter, agentFilter],
  )

  const sessions = useSessions(sessionParams)
  const traces = useTraces(traceParams)

  return (
    <div className="space-y-4">
      <PageHeader
        title="Live Debug"
        description="Recent sessions and traces across all domains. Click a session for its live observation timeline."
      >
        <div className="flex gap-2">
          <Input
            placeholder="Filter by domain"
            value={domainFilter}
            onChange={(e) => setDomainFilter(e.target.value)}
            className="w-48"
          />
          <Input
            placeholder="Filter by agent"
            value={agentFilter}
            onChange={(e) => setAgentFilter(e.target.value)}
            className="w-48"
          />
        </div>
      </PageHeader>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-baseline justify-between">
              Recent sessions
              <span className="text-xs font-normal text-muted-foreground">
                {sessions.data?.total ?? '…'} total
              </span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            {sessions.isLoading ? (
              <Loading />
            ) : sessions.error ? (
              <ErrorState description={String(sessions.error)} onRetry={() => sessions.refetch()} />
            ) : (sessions.data?.items.length ?? 0) === 0 ? (
              <p className="text-sm text-muted-foreground">
                No sessions{domainFilter ? ` for domain ${domainFilter}` : ''}.
              </p>
            ) : (
              <ul className="divide-y">
                {sessions.data?.items.map((s) => (
                  <li key={s.id} className="flex items-center justify-between py-2">
                    <div className="flex flex-col gap-1">
                      <Link
                        to={`/sessions/${s.id}`}
                        className="font-mono text-sm hover:underline"
                      >
                        {s.id.slice(0, 16)}
                      </Link>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span>{s.domainId}</span>
                        {s.startedAt && (
                          <Timestamp date={s.startedAt} />
                        )}
                      </div>
                    </div>
                    <StatusBadge status={s.state} />
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-baseline justify-between">
              Recent traces
              <span className="text-xs font-normal text-muted-foreground">
                {traces.data?.total ?? '…'} total
              </span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            {traces.isLoading ? (
              <Loading />
            ) : traces.error ? (
              <ErrorState description={String(traces.error)} onRetry={() => traces.refetch()} />
            ) : (traces.data?.items.length ?? 0) === 0 ? (
              <p className="text-sm text-muted-foreground">
                No traces{domainFilter || agentFilter ? ' matching filters' : ''}.
              </p>
            ) : (
              <ul className="divide-y">
                {traces.data?.items.map((t) => (
                  <li key={t.traceId} className="flex items-center justify-between py-2">
                    <div className="flex flex-col gap-1">
                      <Link
                        to={`/traces/${t.traceId}`}
                        className="font-mono text-sm hover:underline"
                      >
                        {t.traceId.slice(0, 16)}
                      </Link>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span>{t.domainId}</span>
                        <span>{t.sessionId.slice(0, 12)}</span>
                        {t.startedAt && (
                          <Timestamp date={t.startedAt} />
                        )}
                      </div>
                    </div>
                    <div className="flex flex-col items-end text-xs text-muted-foreground">
                      <span>{t.observationCount} obs</span>
                      <span>${t.totalCostUsd.toFixed(3)}</span>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>

      <p className="text-xs text-muted-foreground">
        Tip: domain/agent filters apply to both panels. Click a session for the live SSE observation stream.
      </p>
      <div>
        <Button asChild variant="outline" size="sm">
          <Link to="/observability">Back to observability overview</Link>
        </Button>
      </div>
    </div>
  )
}