// Per-domain debug panel — shows recent sessions for the given domain.
//
// Phase 6 of the CLI-optional plan. For now this is a launchpad: it
// lists recent sessions filtered by domain and lets the user click
// through to a full SessionDetailPage for live observation streaming.
// A future iteration could embed an in-place ObservationTimeline per
// session, but that requires plumbing per-domain SSE which is out of
// scope here.
import { Link } from 'react-router-dom'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { Loading } from '@/components/ui/Loading'
import { ErrorState } from '@/components/ui/Error'
import { useSessions } from '@/hooks/useSessions'

export function DebugPanel({ domainId }: { domainId: string }) {
  const { data, isLoading, error, refetch } = useSessions({ domain: domainId, limit: 15 })

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-baseline justify-between">
          Recent sessions
          <span className="text-xs font-normal text-muted-foreground">
            {data?.total ?? '…'} total for {domainId}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Loading />
        ) : error ? (
          <ErrorState description={String(error)} onRetry={() => refetch()} />
        ) : (data?.items.length ?? 0) === 0 ? (
          <p className="text-sm text-muted-foreground">
            No sessions for this domain yet. Trigger one via{' '}
            <Link to="/domains" className="underline">Domains → Run</Link>.
          </p>
        ) : (
          <ul className="divide-y">
            {data?.items.map((s) => (
              <li key={s.id} className="flex items-center justify-between py-2">
                <div className="flex flex-col gap-1">
                  <Link
                    to={`/sessions/${s.id}`}
                    className="font-mono text-sm hover:underline"
                  >
                    {s.id.slice(0, 16)}
                  </Link>
                  {s.startedAt && (
                    <Timestamp date={s.startedAt} className="text-xs text-muted-foreground" />
                  )}
                </div>
                <StatusBadge status={s.state} />
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}