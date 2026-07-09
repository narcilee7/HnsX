import type { CSSProperties } from 'react'
import { cn } from '../lib/utils'

export interface ChartSkeletonProps {
  /** 高度 — 默认 240 */
  height?: number | string
  /** 预设形状 */
  shape?: 'line' | 'bars' | 'donut' | 'heatmap' | 'flow'
  className?: string
  style?: CSSProperties
}

/**
 * 图表骨架屏。每个图表形态都给一个静态预览，避免加载时整片空白跳动。
 * 用 animate-pulse + 不同 opacities 模拟"内容正在进来"的感觉。
 */
export function ChartSkeleton({
  height = 240,
  shape = 'line',
  className,
  style,
}: ChartSkeletonProps) {
  return (
    <div
      role="status"
      aria-busy="true"
      aria-live="polite"
      className={cn('flex w-full items-end gap-1.5 overflow-hidden', className)}
      style={{ height, ...style }}
    >
      {shape === 'line' && <LineSkeleton />}
      {shape === 'bars' && <BarsSkeleton />}
      {shape === 'donut' && <DonutSkeleton />}
      {shape === 'heatmap' && <HeatmapSkeleton />}
      {shape === 'flow' && <FlowSkeleton />}
      <span className="sr-only">Loading chart…</span>
    </div>
  )
}

export interface ChartEmptyProps {
  message?: string
  description?: string
  height?: number | string
  className?: string
}

/**
 * 图表空态。无数据时不画 0，也不画坐标轴——给一个安静的中心文案。
 */
export function ChartEmpty({
  message = '暂无数据',
  description = '等待事件触发后会在这里显示。',
  height = 240,
  className,
}: ChartEmptyProps) {
  return (
    <div
      role="status"
      className={cn(
        'flex w-full flex-col items-center justify-center gap-1 text-center',
        className,
      )}
      style={{ height }}
    >
      <p className="text-sm text-[var(--chart-text-secondary)]">{message}</p>
      <p className="text-xs text-[var(--chart-text-muted)]">{description}</p>
    </div>
  )
}

// ---------- 各 shape 实现 ----------

function LineSkeleton() {
  // 用 9 根柱状模拟折线起伏
  const heights = [0.4, 0.55, 0.35, 0.7, 0.5, 0.85, 0.6, 0.75, 0.45]
  return (
    <>
      {heights.map((h, i) => (
        <div
          key={`sk-line-${i}`}
          className="flex-1 rounded-sm bg-[var(--chart-grid)] animate-pulse"
          style={{ height: `${h * 100}%`, opacity: 0.4 + h * 0.4 }}
        />
      ))}
    </>
  )
}

function BarsSkeleton() {
  const heights = [0.5, 0.7, 0.4, 0.85, 0.3, 0.65, 0.55]
  return (
    <>
      {heights.map((h, i) => (
        <div
          key={`sk-bar-${i}`}
          className="flex-1 rounded-sm bg-[var(--chart-grid)] animate-pulse"
          style={{ height: `${h * 100}%`, opacity: 0.4 + h * 0.4 }}
        />
      ))}
    </>
  )
}

function DonutSkeleton() {
  return (
    <div className="flex h-full w-full items-center justify-center">
      <div
        className="animate-pulse rounded-full border-[16px] border-[var(--chart-grid)]"
        style={{ width: 140, height: 140 }}
      />
    </div>
  )
}

function HeatmapSkeleton() {
  const rows = 7
  const cols = 26
  return (
    <div
      className="grid h-full w-full gap-[2px]"
      style={{ gridTemplateRows: `repeat(${rows}, 1fr)`, gridTemplateColumns: `repeat(${cols}, 1fr)` }}
    >
      {Array.from({ length: rows * cols }).map((_, i) => (
        <div
          key={`sk-hm-${i}`}
          className="animate-pulse rounded-[2px] bg-[var(--chart-grid)]"
          style={{ opacity: ((i * 13) % 7) / 10 + 0.2 }}
        />
      ))}
    </div>
  )
}

function FlowSkeleton() {
  return (
    <div className="flex h-full w-full flex-col gap-2">
      {Array.from({ length: 5 }).map((_, i) => (
        <div key={`sk-flow-${i}`} className="flex items-center gap-3">
          <div className="h-3 w-24 animate-pulse rounded bg-[var(--chart-grid)]" />
          <div
            className="h-7 flex-1 animate-pulse rounded bg-[var(--chart-grid)]"
            style={{ opacity: 0.3 + (i % 3) * 0.2 }}
          />
          <div className="h-3 w-12 animate-pulse rounded bg-[var(--chart-grid)]" />
        </div>
      ))}
    </div>
  )
}