import { useParams, Link } from 'react-router-dom'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/PageHeader'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { useTrace } from '@/hooks/useTraces'
import { ArrowLeft } from 'lucide-react'

export default function TraceDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: trace, isLoading, error, refetch } = useTrace(id)

  if (isLoading) return <Loading />
  if (error || !trace) {
    return <ErrorState description={error?.message || 'Trace not found'} onRetry={refetch} />
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={`Trace ${trace.traceId}`}
        breadcrumbs={[
          { label: 'Traces', href: '/traces' },
          { label: trace.traceId },
        ]}
      >
        <Link
          to={`/sessions/${trace.sessionId}`}
          className={cn(buttonVariants({ variant: 'outline' }), 'no-underline')}
        >
          <ArrowLeft className="mr-2 h-4 w-4" /> Session
        </Link>
      </PageHeader>

      <Card>
        <CardHeader>
          <CardTitle>Observations</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {trace.observations.length === 0 ? (
            <p className="text-sm text-muted-foreground">No observations.</p>
          ) : (
            trace.observations.map((obs, idx) => (
              <pre key={idx} className="rounded-md bg-muted p-2 text-xs">
                {JSON.stringify(obs, null, 2)}
              </pre>
            ))
          )}
        </CardContent>
      </Card>
    </div>
  )
}
