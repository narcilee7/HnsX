import { useMemo } from 'react'
import { cn } from '../lib/utils'

export interface TraceMiniBarSpan {
  id: string
  /** 起止时间（毫秒） */
  startMs: number
  endMs: number
  /** 颜色槽位 — 默认 chart-1 */
  variant?: 'chart-1' | 'chart-2' | 'chart-3' | 'chart-4' | 'chart-5' | 'success' | 'warning' | 'danger' | 'info'
  /** 可选 label（hover 显示） */
  label?: string
}

export interface TraceMiniBarProps {
  spans: TraceMiniBarSpan[]
  /** 整体高度 — 默认 8 */
  height?: number
  /** 整体宽度 — 默认 100% */
  width?: number | string
  /** 当前 trace 的总持续时间（毫秒）；不传会自动从 spans 推算 */
  totalMs?: number
  /** 起始时间偏移（毫秒） — 不传则用最小 startMs */
  originMs?: number
  className?: string
}

/**
 * 紧凑多 span 时间线，用于 trace 列表每行的快速对比。
 * 设计原则：
 *   - 单条横向 bar，按 startMs/endMs 比例定位 + 宽度
 *   - 重叠 span 用不同 y 偏移堆叠
 *   - 颜色按 variant 取 chart-1..5 或 status
 *   - hover 显示 label + 耗时
 */
export function TraceMiniBar({
  spans,
  height = 8,
  width = '100%',
  totalMs,
  originMs,
  className,
}: TraceMiniBarProps) {
  const { origin, total, rows } = useMemo(() => {
    if (spans.length === 0) return { origin: 0, total: 1, rows: [] as TraceMiniBarSpan[][] }
    const minStart = Math.min(...spans.map((s) => s.startMs))
    const maxEnd = Math.max(...spans.map((s) => s.endMs))
    const o = originMs ?? minStart
    const t = totalMs ?? Math.max(maxEnd - minStart, 1)
    // 按起止时间做 lane 分配（贪心）
    const sorted = [...spans].sort((a, b) => a.startMs - b.startMs)
    const lanes: Array<{ end: number }> = []
    const placed = sorted.map((span) => {
      let lane = lanes.findIndex((l) => l.end <= span.startMs)
      if (lane === -1) {
        lanes.push({ end: span.endMs })
        lane = lanes.length - 1
      } else {
        lanes[lane]!.end = span.endMs
      }
      return { span, lane }
    })
    const laneCount = Math.max(lanes.length, 1)
    const rowArr: TraceMiniBarSpan[][] = Array.from({ length: laneCount }, () => [])
    for (const { span, lane } of placed) {
      rowArr[lane]!.push(span)
    }
    return { origin: o, total: t, rows: rowArr }
  }, [spans, totalMs, originMs])

  if (spans.length === 0) {
    return (
      <div
        className={cn(
          'flex items-center justify-center rounded bg-[var(--chart-grid)]/40',
          className,
        )}
        style={{ height, width }}
      >
        <span className="text-[10px] text-[var(--chart-text-muted)]">no spans</span>
      </div>
    )
  }

  const laneHeight = Math.max(2, (height - 2) / rows.length)

  return (
    <div
      className={cn('relative w-full overflow-hidden rounded bg-[var(--chart-grid)]/40', className)}
      style={{ height, width }}
      role="img"
      aria-label={`Trace timeline with ${spans.length} spans`}
    >
      {rows.map((row, ri) => (
        <div
          key={`lane-${ri}`}
          className="absolute left-0 right-0"
          style={{ top: 1 + ri * laneHeight, height: laneHeight - 1 }}
        >
          {row.map((span) => {
            const left = ((span.startMs - origin) / total) * 100
            const w = Math.max(((span.endMs - span.startMs) / total) * 100, 0.5)
            const v = span.variant ?? 'chart-1'
            return (
              <div
                key={span.id}
                title={
                  span.label
                    ? `${span.label} · ${span.endMs - span.startMs}ms`
                    : `${span.endMs - span.startMs}ms`
                }
                className="absolute rounded-[1px] transition-opacity hover:opacity-80"
                style={{
                  left: `${left}%`,
                  width: `${w}%`,
                  height: '100%',
                  background: `var(--${v})`,
                }}
              />
            )
          })}
        </div>
      ))}
    </div>
  )
}