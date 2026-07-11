import { useMemo } from 'react'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { History } from 'lucide-react'

const STORAGE_KEY_PREFIX = 'hnsx.workspace.triggers.'

export interface TriggerRecord {
  label: string
  payload: Record<string, unknown>
  at: string
}

export function useTriggerHistory(domainId: string | undefined) {
  return useMemo<TriggerRecord[]>(() => {
    if (typeof window === 'undefined' || !domainId) return []
    try {
      const raw = window.localStorage.getItem(STORAGE_KEY_PREFIX + domainId)
      return raw ? (JSON.parse(raw) as TriggerRecord[]) : []
    } catch {
      return []
    }
  }, [domainId])
}

export function pushTriggerHistory(
  domainId: string | undefined,
  payload: Record<string, unknown>,
  label?: string,
) {
  if (typeof window === 'undefined' || !domainId) return
  try {
    const firstText =
      typeof payload.question === 'string'
        ? payload.question
        : typeof payload.task === 'string'
          ? payload.task
          : JSON.stringify(payload).slice(0, 80)
    const record: TriggerRecord = {
      label: label || firstText || 'Untitled',
      payload,
      at: new Date().toISOString(),
    }
    const raw = window.localStorage.getItem(STORAGE_KEY_PREFIX + domainId)
    const prev = raw ? (JSON.parse(raw) as TriggerRecord[]) : []
    const next = [record, ...prev.filter((r) => r.at !== record.at)]
      .filter((r, idx, self) => self.findIndex((x) => x.at === r.at) === idx)
      .slice(0, 5)
    window.localStorage.setItem(STORAGE_KEY_PREFIX + domainId, JSON.stringify(next))
  } catch {
    // ignore quota errors
  }
}

interface TriggerHistoryProps {
  domainId: string | undefined
  onSelect: (payload: Record<string, unknown>) => void
}

export function TriggerHistory({ domainId, onSelect }: TriggerHistoryProps) {
  const items = useTriggerHistory(domainId)

  if (items.length === 0) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <History className="h-4 w-4" />
          Recent Triggers
        </CardTitle>
        <CardDescription>本机最近 5 次触发记录</CardDescription>
      </CardHeader>
      <CardContent className="space-y-2">
        {items.map((item) => (
          <button
            key={item.at}
            onClick={() => onSelect(item.payload)}
            className="w-full rounded-md border bg-card px-3 py-2 text-left transition-colors hover:bg-accent"
          >
            <p className="truncate text-sm font-medium">{item.label}</p>
            <p className="text-xs text-muted-foreground">
              {new Date(item.at).toLocaleString()}
            </p>
          </button>
        ))}
      </CardContent>
    </Card>
  )
}
