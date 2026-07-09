import { useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Timestamp } from '@/components/ui/Timestamp'
import type { ObservationViewModel } from '@/api/mappers'

interface ObservationCardProps {
  observation: ObservationViewModel
  depth?: number
}

const kindVariantMap: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  text: 'default',
  message: 'default',
  user: 'secondary',
  assistant: 'default',
  tool_call: 'secondary',
  tool_result: 'secondary',
  error: 'destructive',
  cost: 'outline',
  state: 'outline',
  thinking: 'outline',
}

export function ObservationCard({ observation, depth = 0 }: ObservationCardProps) {
  const [expanded, setExpanded] = useState(true)
  const variant = kindVariantMap[observation.kind] || 'outline'

  return (
    <Card
      className={cn(
        'overflow-hidden border-l-4',
        depth === 0 ? 'border-l-primary' : 'border-l-muted',
      )}
      style={{ marginLeft: depth * 16 }}
    >
      <CardHeader className="flex flex-row items-center justify-between gap-2 p-3">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon-xs" onClick={() => setExpanded((v) => !v)}>
            {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          </Button>
          <Badge variant={variant}>{observation.kind}</Badge>
          {observation.agentId && <span className="text-sm text-muted-foreground">{observation.agentId}</span>}
          {observation.role && <Badge variant="outline">{observation.role}</Badge>}
          <Timestamp date={observation.createdAt} />
        </div>
        {observation.stepId && <span className="text-xs text-muted-foreground">{observation.stepId}</span>}
      </CardHeader>
      {expanded && (
        <CardContent className="space-y-2 p-3 pt-0">
          {observation.payload && Object.keys(observation.payload).length > 0 && (
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground">Payload</p>
              <pre className="max-h-48 overflow-auto rounded-md bg-muted p-2 text-xs">
                {JSON.stringify(observation.payload, null, 2)}
              </pre>
            </div>
          )}
          {observation.metadata && Object.keys(observation.metadata).length > 0 && (
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground">Metadata</p>
              <pre className="max-h-32 overflow-auto rounded-md bg-muted p-2 text-xs">
                {JSON.stringify(observation.metadata, null, 2)}
              </pre>
            </div>
          )}
        </CardContent>
      )}
    </Card>
  )
}

interface ObservationTimelineProps {
  observations: ObservationViewModel[]
  filterAgent?: string
  filterKind?: string
}

export function ObservationTimeline({ observations, filterAgent, filterKind }: ObservationTimelineProps) {
  const filtered = observations.filter((obs) => {
    if (filterAgent && obs.agentId !== filterAgent) return false
    if (filterKind && obs.kind !== filterKind) return false
    return true
  })

  const sorted = [...filtered].sort((a, b) => {
    const ta = a.createdAt?.getTime() ?? 0
    const tb = b.createdAt?.getTime() ?? 0
    return ta - tb
  })

  const grouped = sorted.reduce<Record<string, ObservationViewModel[]>>((acc, obs) => {
    const key = obs.parentId || obs.stepId || '_root'
    if (!acc[key]) acc[key] = []
    acc[key].push(obs)
    return acc
  }, {})

  const rootObservations = grouped['_root'] || sorted

  return (
    <div className="space-y-3">
      {rootObservations.length === 0 ? (
        <p className="text-sm text-muted-foreground">No observations match the current filters.</p>
      ) : (
        rootObservations.map((obs) => (
          <ObservationCard key={obs.observationId} observation={obs} />
        ))
      )}
    </div>
  )
}

export function useObservationFilters(observations: ObservationViewModel[]) {
  const agents = Array.from(new Set(observations.map((o) => o.agentId).filter(Boolean)))
  const kinds = Array.from(new Set(observations.map((o) => o.kind).filter(Boolean)))
  return { agents, kinds }
}
