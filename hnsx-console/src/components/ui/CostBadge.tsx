import { Badge } from '@/components/ui/badge'

interface CostBadgeProps {
  cost?: number
  className?: string
}

export function CostBadge({ cost, className }: CostBadgeProps) {
  if (cost === undefined || cost === null) {
    return <span className={className}>-</span>
  }
  return (
    <Badge variant="outline" className={className}>
      ${cost.toFixed(4)}
    </Badge>
  )
}
