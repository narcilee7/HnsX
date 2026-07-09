import { useMemo } from 'react'
import { cn } from '../lib/utils'
import { formatCompact, formatNumber } from '../lib/utils'
import { ChartFrame } from './ChartFrame'

export interface AgentHandoff {
  source: string
  target: string
  count: number
  avgMs?: number
}

export interface AgentFlowDiagramProps {
  data: AgentHandoff[]
  height?: number | string
  className?: string
  hideHeader?: boolean
  loading?: boolean
}

/**
 * Agent 之间 hand-off 流向图。设计原则：
 *   - 行 = source agent，分段条 = target agent 占比
 *   - 横向条形图（horizontal stacked bar），不用 sankey
 *     —— sankey 在小数据下信息密度低、视觉噪声大
 *   - 每行右侧补一个数字汇总（总 hand-off 数 / 平均耗时）
 *   - 颜色固定按 source 在 agents 列表里的索引取 chart-1..5
 */
export function AgentFlowDiagram({
  data,
  height = 320,
  className,
  hideHeader = false,
  loading = false,
}: AgentFlowDiagramProps) {
  const { agents, rows, maxTotal } = useMemo(() => {
    const sourceSet = new Set<string>()
    const targetSet = new Set<string>()
    for (const h of data) {
      sourceSet.add(h.source)
      targetSet.add(h.target)
    }
    const all = Array.from(new Set([...sourceSet, ...targetSet]))

    const grouped = new Map<string, AgentHandoff[]>()
    for (const h of data) {
      if (!grouped.has(h.source)) grouped.set(h.source, [])
      grouped.get(h.source)!.push(h)
    }

    const rowsLocal = Array.from(grouped.entries()).map(([source, items]) => {
      const total = items.reduce((acc, h) => acc + h.count, 0)
      const avgMs =
        items.reduce((acc, h) => acc + (h.avgMs ?? 0) * h.count, 0) / Math.max(total, 1)
      return { source, total, avgMs, items }
    })

    const maxTotalLocal = rowsLocal.reduce((acc, r) => Math.max(acc, r.total), 0)
    return { agents: all, rows: rowsLocal, maxTotal: maxTotalLocal }
  }, [data])

  const slotFor = (name: string) => {
    const idx = agents.indexOf(name)
    const slot = ((idx % 5) + 1) as 1 | 2 | 3 | 4 | 5
    return `var(--chart-${slot})`
  }

  return (
    <ChartFrame
      title={hideHeader ? undefined : 'Agent 流向'}
      description={hideHeader ? undefined : '各 source agent 的 hand-off 分布（颜色按 target 索引）'}
      height={height}
      className={className}
      loading={loading}
      skeletonShape="flow"
      empty={!loading && data.length === 0}
      emptyMessage="暂无 Agent hand-off 数据"
      emptyDescription="需要 Agent 之间产生 hand-off 关系后才会出现流向。"
    >
      {rows.length === 0 ? (
        <div className="flex h-full items-center justify-center text-sm text-[var(--chart-text-muted)]">
          暂无 hand-off 数据
        </div>
      ) : (
        <div className="flex h-full flex-col gap-2 overflow-auto pr-2">
          {rows.map((row) => (
            <div key={`flow-${row.source}`} className="grid grid-cols-[140px_1fr_auto] items-center gap-3">
              <div className="flex items-center gap-2 text-xs">
                <span
                  className="h-2.5 w-2.5 shrink-0 rounded-sm"
                  style={{ background: slotFor(row.source) }}
                  aria-hidden
                />
                <span className="truncate font-medium text-[var(--chart-text-primary)]" title={row.source}>
                  {row.source}
                </span>
              </div>
              <div className="flex h-7 overflow-hidden rounded-md bg-[var(--chart-grid)]/40">
                {row.items.map((item) => {
                  const pct = maxTotal > 0 ? (item.count / maxTotal) * 100 : 0
                  if (pct <= 0) return null
                  return (
                    <div
                      key={`seg-${row.source}-${item.target}`}
                      className={cn(
                        'flex items-center justify-end gap-1 px-1.5 text-[10px] tabular-nums text-white',
                        'transition-opacity hover:opacity-90',
                      )}
                      style={{
                        width: `${pct}%`,
                        background: slotFor(item.target),
                        minWidth: 24,
                      }}
                      title={`${row.source} → ${item.target}: ${item.count}`}
                    >
                      {pct > 8 && <span className="truncate">{item.target}</span>}
                    </div>
                  )
                })}
              </div>
              <div className="flex flex-col items-end text-[11px] tabular-nums leading-tight">
                <span className="text-[var(--chart-text-primary)]">{formatNumber(row.total)}</span>
                {Number.isFinite(row.avgMs) && row.avgMs > 0 && (
                  <span className="text-[var(--chart-text-muted)]">avg {formatCompact(row.avgMs)} ms</span>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </ChartFrame>
  )
}