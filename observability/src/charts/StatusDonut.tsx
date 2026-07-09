import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from 'recharts'
import { cn } from '../lib/utils'
import { formatCompact, formatNumber } from '../lib/utils'
import { ChartFrame } from './ChartFrame'

export interface StatusDonutSlice {
  label: string
  value: number
  /** 槽位 — 'success' | 'warning' | 'danger' | 'info' | 'chart-1..5' */
  variant?: 'success' | 'warning' | 'danger' | 'info' | 'chart-1' | 'chart-2' | 'chart-3' | 'chart-4' | 'chart-5'
}

export interface StatusDonutProps {
  data: StatusDonutSlice[]
  /** 中心标题 */
  centerLabel?: string
  /** 中心数值 — 不传则显示总和 */
  centerValue?: number
  height?: number | string
  className?: string
  hideHeader?: boolean
  loading?: boolean
}

const variantVar = (variant: StatusDonutSlice['variant']) => {
  switch (variant) {
    case 'success':
      return 'var(--success)'
    case 'warning':
      return 'var(--warning)'
    case 'danger':
      return 'var(--danger)'
    case 'info':
      return 'var(--info)'
    case 'chart-1':
      return 'var(--chart-1)'
    case 'chart-2':
      return 'var(--chart-2)'
    case 'chart-3':
      return 'var(--chart-3)'
    case 'chart-4':
      return 'var(--chart-4)'
    case 'chart-5':
      return 'var(--chart-5)'
    default:
      return 'var(--chart-5)'
  }
}

/**
 * 状态/分类占比环形图。设计原则：
 *   - 永远内圈留白 → 放总数或主指标
 *   - 扇区按数值降序排，**绝不**按 variant 排
 *   - tooltip 显示百分比 + 绝对值 + variant 名
 *   - 图例跟数据同列，避免右侧再开一栏
 */
export function StatusDonut({
  data,
  centerLabel,
  centerValue,
  height = 260,
  className,
  hideHeader = false,
  loading = false,
}: StatusDonutProps) {
  const sorted = [...data].sort((a, b) => b.value - a.value)
  const total = sorted.reduce((acc, d) => acc + d.value, 0)
  const isEmpty = data.length === 0 || total === 0

  return (
    <ChartFrame
      title={hideHeader ? undefined : '状态占比'}
      description={hideHeader ? undefined : 'Session / Step 的状态分布'}
      height={height}
      className={className}
      loading={loading}
      skeletonShape="donut"
      empty={!loading && isEmpty}
      emptyMessage="暂无状态分布"
      emptyDescription="需要至少一条 Session 记录后才会出现扇区。"
    >
      <div className="flex h-full items-center gap-4">
        <div className="relative h-full flex-1">
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Tooltip content={<DonutTooltip total={total} />} />
              <Pie
                data={sorted}
                dataKey="value"
                nameKey="label"
                innerRadius="62%"
                outerRadius="88%"
                paddingAngle={2}
                stroke="var(--card)"
                strokeWidth={2}
                isAnimationActive={false}
              >
                {sorted.map((entry, idx) => (
                  <Cell key={`donut-${idx}`} fill={variantVar(entry.variant)} />
                ))}
              </Pie>
            </PieChart>
          </ResponsiveContainer>
          <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
            <div className="text-[10px] uppercase tracking-wider text-[var(--chart-text-muted)]">
              {centerLabel ?? 'Total'}
            </div>
            <div className="text-xl font-semibold tabular-nums text-[var(--chart-text-primary)]">
              {formatCompact(centerValue ?? total)}
            </div>
          </div>
        </div>
        <ul className="flex flex-col gap-1.5 text-xs">
          {sorted.map((entry, idx) => (
            <li key={`legend-${idx}`} className="flex items-center gap-2 tabular-nums">
              <span
                className="h-2.5 w-2.5 rounded-sm"
                style={{ background: variantVar(entry.variant) }}
                aria-hidden
              />
              <span className="text-[var(--chart-text-secondary)]">{entry.label}</span>
              <span className="ml-auto text-[var(--chart-text-primary)]">{formatCompact(entry.value)}</span>
              <span className="w-10 text-right text-[var(--chart-text-muted)]">
                {total > 0 ? `${((entry.value / total) * 100).toFixed(1)}%` : '—'}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </ChartFrame>
  )
}

function DonutTooltip({
  active,
  payload,
  total,
}: {
  active?: boolean
  payload?: Array<{ payload?: StatusDonutSlice; value?: number }>
  total: number
}) {
  if (!active || !payload || payload.length === 0) return null
  const slice = payload[0]?.payload
  const value = payload[0]?.value ?? 0
  const pct = total > 0 ? (value / total) * 100 : 0
  return (
    <div
      className={cn(
        'rounded-md border px-3 py-2 text-xs shadow-lg',
        'border-[var(--chart-tooltip-border)] bg-[var(--chart-tooltip-bg)]',
      )}
    >
      <div className="font-medium text-[var(--chart-text-primary)]">{slice?.label}</div>
      <div className="mt-1 flex items-center gap-3 tabular-nums">
        <span className="text-[var(--chart-text-secondary)]">数值</span>
        <span className="ml-auto text-[var(--chart-text-primary)]">{formatNumber(value)}</span>
      </div>
      <div className="flex items-center gap-3 tabular-nums">
        <span className="text-[var(--chart-text-secondary)]">占比</span>
        <span className="ml-auto text-[var(--chart-text-primary)]">{pct.toFixed(1)}%</span>
      </div>
    </div>
  )
}