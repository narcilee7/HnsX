import { Sparklines, SparklinesArea, SparklinesLine, SparklinesSpots } from 'react-sparklines'
import { cn } from '../lib/utils'

export interface SparklineProps {
  /** 等间距时间序列（任意数值即可） */
  data: number[]
  /** 颜色槽位 — 默认 chart-1 */
  variant?: 'chart-1' | 'chart-2' | 'chart-3' | 'chart-4' | 'chart-5' | 'success' | 'warning' | 'danger' | 'info'
  /** 折线 / 面积 — 默认 area */
  kind?: 'area' | 'line'
  /** 高度（像素） — 默认 36 */
  height?: number
  /** 宽度 — 默认 100% */
  width?: number | string
  /** 端点圆点 — 默认 false */
  endDot?: boolean
  /** margin 内边距 — 默认 [2, 2, 2, 2]，sparkline 一般不画坐标轴所以不需要太多 */
  margin?: number | [number, number, number, number]
  className?: string
  /** aria-label */
  'aria-label'?: string
}

const CSS_VAR = (variant: NonNullable<SparklineProps['variant']>) => `var(--${variant})`
const CSS_VAR_SOFT = (variant: NonNullable<SparklineProps['variant']>) => `var(--${variant}-soft)`

/**
 * 紧凑时间序列预览。基于 react-sparklines，**不拉 recharts ResponsiveContainer**。
 * 设计原则：
 *   - 永远不画坐标轴
 *   - 永远不画 legend（单系列）
 *   - 颜色通过 CSS var 直接绑定，确保主题切换零成本
 */
export function Sparkline({
  data,
  variant = 'chart-1',
  kind = 'area',
  height = 36,
  width = '100%',
  endDot = false,
  margin = 2,
  className,
  'aria-label': ariaLabel,
}: SparklineProps) {
  const stroke = CSS_VAR(variant)
  const fill = CSS_VAR_SOFT(variant)

  return (
    <div
      className={cn('w-full overflow-hidden', className)}
      style={{ height, width }}
      role="img"
      aria-label={ariaLabel ?? `Sparkline ${variant}`}
    >
      <Sparklines data={data} height={height} margin={margin} style={{ width: '100%' }}>
        {kind === 'area' && (
          <SparklinesArea
            style={{
              fill: fill,
              stroke: stroke,
              strokeWidth: 1.75,
            }}
          />
        )}
        {kind === 'line' && (
          <SparklinesLine
            style={{
              stroke: stroke,
              strokeWidth: 1.75,
              fill: 'none',
            }}
          />
        )}
        {endDot && (
          <SparklinesSpots
            size={2.5}
            style={{
              fill: stroke,
              stroke: stroke,
              strokeWidth: 0,
            }}
          />
        )}
      </Sparklines>
    </div>
  )
}