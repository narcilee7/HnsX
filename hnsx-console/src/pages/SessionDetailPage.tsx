import { useParams, Link } from 'react-router-dom'
import { useState, useMemo } from 'react'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/PageHeader'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { useSession, useSessionEvents } from '@/hooks/useSessions'
import {
  ObservationTimeline,
  useObservationFilters,
} from '@/components/session/ObservationTimeline'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { CostBadge } from '@/components/ui/CostBadge'
import { ArrowLeft, Check, X } from 'lucide-react'

export default function SessionDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: session, isLoading, error, refetch } = useSession(id)
  const { observations, state, connected } = useSessionEvents(id)
  const [agentFilter, setAgentFilter] = useState('')
  const [kindFilter, setKindFilter] = useState('')

  const { agents, kinds } = useObservationFilters(observations)

  const durationMs = useMemo(() => {
    if (!session?.startedAt || !session?.completedAt) return undefined
    return session.completedAt.getTime() - session.startedAt.getTime()
  }, [session])

  const statCards: { label: string; value: React.ReactNode }[] = [
    {
      label: 'Domain',
      value: (
        <Link to={`/domains/${session?.domainId}`} className="font-medium hover:underline">
          {session?.domainId}
        </Link>
      ),
    },
    { label: 'State', value: <StatusBadge status={state || session?.state || ''} /> },
    { label: 'Started At', value: <Timestamp date={session?.startedAt} /> },
    { label: 'Duration', value: <span className="text-sm font-medium">{durationMs ? `${durationMs}ms` : '-'}</span> },
    { label: 'Cost', value: <CostBadge cost={undefined} /> },
    { label: 'Agent Calls', value: <span className="text-sm font-medium">-</span> },
    { label: 'Tool Calls', value: <span className="text-sm font-medium">-</span> },
  ]

  if (isLoading) return <Loading />
  if (error || !session) {
    return <ErrorState description={error?.message || 'Session not found'} onRetry={refetch} />
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={`Session ${session.id}`}
        breadcrumbs={[
          { label: 'Sessions', href: '/sessions' },
          { label: session.id },
        ]}
      >
        <div className="flex items-center gap-2">
          {(state || session.state) === 'paused' && (
            <>
              <button className={cn(buttonVariants({ variant: 'default' }))}>
                <Check className="mr-2 h-4 w-4" /> Approve
              </button>
              <button className={cn(buttonVariants({ variant: 'outline' }))}>
                <X className="mr-2 h-4 w-4" /> Reject
              </button>
            </>
          )}
          <Link
            to="/sessions"
            className={cn(buttonVariants({ variant: 'outline' }), 'no-underline')}
          >
            <ArrowLeft className="mr-2 h-4 w-4" /> Back
          </Link>
        </div>
      </PageHeader>

      <div className="grid gap-4 lg:grid-cols-4 md:grid-cols-2">
        {statCards.map((stat) => (
          <Card key={stat.label}>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">{stat.label}</CardTitle>
            </CardHeader>
            <CardContent>{stat.value}</CardContent>
          </Card>
        ))}
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Live</CardTitle>
          </CardHeader>
          <CardContent>
            {connected ? (
              <span className="inline-flex items-center gap-1.5 text-sm font-medium text-green-600">
                <span className="h-2 w-2 animate-pulse rounded-full bg-green-600" />
                Connected
              </span>
            ) : (
              <span className="inline-flex items-center gap-1.5 text-sm text-muted-foreground">
                <span className="h-2 w-2 rounded-full bg-muted-foreground" />
                Disconnected
              </span>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Trace Timeline</CardTitle>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <Label htmlFor="agent-filter" className="text-sm">Agent</Label>
              <Select value={agentFilter} onValueChange={(v) => setAgentFilter(v || '')}>
                <SelectTrigger id="agent-filter" className="w-36">
                  <SelectValue placeholder="All agents" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="">All agents</SelectItem>
                  {agents.map((agent) => (
                    <SelectItem key={agent} value={agent}>{agent}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <Label htmlFor="kind-filter" className="text-sm">Kind</Label>
              <Select value={kindFilter} onValueChange={(v) => setKindFilter(v || '')}>
                <SelectTrigger id="kind-filter" className="w-36">
                  <SelectValue placeholder="All kinds" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="">All kinds</SelectItem>
                  {kinds.map((kind) => (
                    <SelectItem key={kind} value={kind}>{kind}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-2">
          <ObservationTimeline
            observations={observations}
            filterAgent={agentFilter || undefined}
            filterKind={kindFilter || undefined}
          />
        </CardContent>
      </Card>
    </div>
  )
}
