import { formatDistanceToNow } from 'date-fns'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatDate } from '@/api/mappers'

interface TimestampProps {
  date: Date | string | number | null | undefined
  className?: string
}

export function Timestamp({ date, className }: TimestampProps) {
  const parsed = typeof date === 'string' || typeof date === 'number' ? new Date(date) : date
  if (!parsed || Number.isNaN(parsed.getTime())) {
    return <span className={className}>-</span>
  }
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger>
          <span className={className}>{formatDistanceToNow(parsed, { addSuffix: true })}</span>
        </TooltipTrigger>
        <TooltipContent>
          <p>{formatDate(parsed)}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}
