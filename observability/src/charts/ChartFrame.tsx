import type { CSSProperties, ReactNode } from 'react'
import { cn } from '../lib/utils'
import { ChartEmpty, ChartSkeleton, type ChartSkeletonProps } from './ChartStates'

export interface ChartFrameProps {
  title?: ReactNode
  description?: ReactNode
  /** 右上角控件区 — 通常放 time-range / filter */
  actions?: ReactNode
  /** 图表本身 */
  children: ReactNode
  /** 自定义高度（像素） */
  height?: number | string
  className?: string
  style?: CSSProperties
  /** 加载态 — 显示骨架屏替代 children */
  loading?: boolean
  /** 加载时骨架的形状 */
  skeletonShape?: ChartSkeletonProps['shape']
  /** 空态 — 显示空态文案替代 children */
  empty?: boolean
  /** 空态文案 */
  emptyMessage?: string
  emptyDescription?: string
}

/**
 * 图表外壳：统一标题 / 副标题 / actions / 高度 / loading / empty。
 * 所有 chart 组件都套这个壳，保证视觉一致性。
 */
export function ChartFrame({
  title,
  description,
  actions,
  children,
  height = 280,
  className,
  style,
  loading = false,
  skeletonShape = 'line',
  empty = false,
  emptyMessage,
  emptyDescription,
}: ChartFrameProps) {
  return (
    <div
      className={cn(
        'rounded-lg border border-[var(--chart-grid)] bg-[var(--card)] p-4 shadow-sm',
        className,
      )}
      style={style}
    >
      {(title || description || actions) && (
        <div className="mb-3 flex items-start justify-between gap-3">
          <div className="space-y-0.5">
            {title && (
              <h3 className="text-sm font-semibold text-[var(--chart-text-primary)]">{title}</h3>
            )}
            {description && (
              <p className="text-xs text-[var(--chart-text-muted)]">{description}</p>
            )}
          </div>
          {actions && <div className="flex items-center gap-2">{actions}</div>}
        </div>
      )}
      <div style={{ width: '100%', height }}>
        {loading ? (
          <ChartSkeleton shape={skeletonShape} height={height} />
        ) : empty ? (
          <ChartEmpty message={emptyMessage} description={emptyDescription} height={height} />
        ) : (
          children
        )}
      </div>
    </div>
  )
}