import { Badge } from '@/components/ui/badge'

export type Status = 'active' | 'inactive' | 'draft' | 'running' | 'completed' | 'failed' | 'paused' | 'cancelled' | 'pending' | 'approved' | 'rejected' | string

const statusVariantMap: Record<string, 'default' | 'secondary' | 'destructive' | 'outline'> = {
  active: 'default',
  completed: 'default',
  approved: 'default',
  running: 'secondary',
  pending: 'secondary',
  paused: 'secondary',
  draft: 'outline',
  inactive: 'outline',
  cancelled: 'outline',
  failed: 'destructive',
  rejected: 'destructive',
}

interface StatusBadgeProps {
  status: Status
  className?: string
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const variant = statusVariantMap[status] || 'outline'
  return (
    <Badge variant={variant} className={className}>
      {status}
    </Badge>
  )
}
