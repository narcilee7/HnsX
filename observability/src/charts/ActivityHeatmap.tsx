import { useMemo } from 'react'
import CalendarHeatmap from 'react-calendar-heatmap'
import 'react-calendar-heatmap/dist/styles.css'
import { ChartFrame } from './ChartFrame'
import { cn } from '../lib/utils'

export interface ActivityHeatmapCell {
  /** ISO date — 'YYYY-MM-DD' */
  date: string
  /** 数值 — 例如当天 session 数 / token 数 / 错误数 */
  value: number
}

export interface ActivityHeatmapProps {
  data: ActivityHeatmapCell[]
  /** 显示多少列（约几周）— 默认 26 */
  cols?: number
  /** 颜色变体 — 默认 chart-1 */
  variant?: 'chart-1' | 'chart-2' | 'chart-3' | 'chart-4' | 'chart-5' | 'success' | 'warning' | 'danger' | 'info'
  height?: number | string
  className?: string
  hideHeader?: boolean
  /** 单位文本 — 例如 "sessions" / "tokens" */
  unit?: string
  /** 取数后端 — 默认 percentile 95 */
  cap?: 'p95' | 'p99' | 'max'
  /** 加载态 — 显示骨架屏 */
  loading?: boolean
}

const BIN_COUNT = 4

/**
 * 基于 react-calendar-heatmap 的活动热力图。设计原则：
 *   - 永远用离散档位（4 档），不用连续渐变
 *   - 颜色通过 inline fill 直绑 var(--${variant}) + color-mix，确保主题切换
 *   - 永远不画坐标 label（GitHub 同款静默设计）
 */
export function ActivityHeatmap({
  data,
  cols = 26,
  variant = 'chart-1',
  height,
  className,
  hideHeader = false,
  unit = 'sessions',
  cap = 'p95',
  loading = false,
}: ActivityHeatmapProps) {
  const { startDate, endDate, scale } = useMemo(() => {
    const last = data.length > 0 ? new Date(data[data.length - 1]!.date) : new Date()
    const start = new Date(last)
    start.setDate(start.getDate() - cols * 7 + 1)
    const values = data.map((d) => d.value).filter((v) => v > 0).sort((a, b) => a - b)
    const at = (p: number) => values[Math.min(values.length - 1, Math.floor(values.length * p))] ?? 1
    const p95 = cap === 'p95' ? at(0.95) : cap === 'p99' ? at(0.99) : values[values.length - 1] ?? 1
    // 4 个 bin 阈值：>= p95, >= p66, >= p33, > 0
    return {
      startDate: start,
      endDate: last,
      scale: { p33: at(0.33), p66: at(0.66), p1: p95 },
    }
  }, [data, cols, cap])

  const colorFor = (value: number): string => {
    if (value <= 0) return 'var(--chart-grid)'
    if (value >= scale.p1) return `var(--${variant})`
    if (value >= scale.p66) return `color-mix(in oklch, var(--${variant}) 75%, transparent)`
    if (value >= scale.p33) return `color-mix(in oklch, var(--${variant}) 50%, transparent)`
    return `color-mix(in oklch, var(--${variant}) 25%, transparent)`
  }

  // react-calendar-heatmap 的 classForValue 必须返回 string（用于内置 CSS）。
  // 我们用 rectRender 完全接管 fill，因此 classForValue 只返回占位 class。
  const classForValue = (value: ActivityHeatmapCell | undefined) => {
    if (!value || value.value <= 0) return 'react-calendar-heatmap__empty'
    return `react-calendar-heatmap__bin-${binOf(value.value, scale)}`
  }

  const tooltipFor = (value: ActivityHeatmapCell | undefined) =>
    value ? `${value.date} · ${value.value} ${unit}` : ''

  return (
    <ChartFrame
      title={hideHeader ? undefined : '活动热力图'}
      description={hideHeader ? undefined : `每日 ${unit} 强度（按分位数离散分 ${BIN_COUNT} 档）`}
      height={height}
      loading={loading}
      skeletonShape="heatmap"
      empty={!loading && data.length === 0}
      emptyMessage="暂无活动数据"
      emptyDescription={`积累 ${cols} 周的 ${unit} 后会渲染热力图。`}
      className={cn(
        // 把 react-calendar-heatmap 的内置 rect 边框/hover 收编进 Morandi
        '[&_.react-calendar-heatmap]:w-full',
        '[&_.react-calendar-heatmap__month-label]:fill-[var(--chart-text-muted)] [&_.react-calendar-heatmap__month-label]:text-[10px]',
        '[&_.react-calendar-heatmap__weekday-label]:fill-[var(--chart-text-muted)] [&_.react-calendar-heatmap__weekday-label]:text-[10px]',
        '[&_.react-calendar-heatmap__day]:stroke-[var(--card)] [&_.react-calendar-heatmap__day]:stroke-[1px]',
        '[&_.react-calendar-heatmap__day:hover]:stroke-[var(--chart-text-primary)]',
        className,
      )}
    >
      <CalendarHeatmap
        startDate={startDate}
        endDate={endDate}
        // 库内部约定 count 字段；在边界处映射
        values={data.map((d) => ({ date: d.date, count: d.value }))}
        classForValue={(v) =>
          classForValue(v ? { date: v.date, value: v.count ?? 0 } : undefined)
        }
        gutterSize={2}
        // rectRender 接管 fill，直接绑 CSS var，绕过库内置 color scale
        rectRender={(props: Record<string, unknown>, value: { date: string; count?: number } | undefined) => {
          const cellValue = value?.count ?? 0
          const cell: ActivityHeatmapCell | undefined = value
            ? { date: value.date, value: cellValue }
            : undefined
          return (
            <rect
              {...props}
              fill={colorFor(cellValue)}
              rx={2}
              ry={2}
              data-date={value?.date}
              data-value={cellValue}
            >
              <title>{tooltipFor(cell)}</title>
            </rect>
          )
        }}
      />
      <div className="mt-2 flex items-center justify-end gap-2 text-[10px] text-[var(--chart-text-muted)]">
        <span>少</span>
        {[0, 25, 50, 75, 100].map((step) => (
          <span
            key={`legend-${step}`}
            className="h-3 w-3 rounded-[3px]"
            style={{
              background:
                step === 0
                  ? 'var(--chart-grid)'
                  : step === 100
                    ? `var(--${variant})`
                    : `color-mix(in oklch, var(--${variant}) ${step}%, transparent)`,
            }}
          />
        ))}
        <span>多</span>
      </div>
    </ChartFrame>
  )
}

function binOf(value: number, scale: { p33: number; p66: number; p1: number }): number {
  if (value <= 0) return 0
  if (value >= scale.p1) return 4
  if (value >= scale.p66) return 3
  if (value >= scale.p33) return 2
  return 1
}