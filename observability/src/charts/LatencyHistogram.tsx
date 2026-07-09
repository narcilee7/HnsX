import { Bar, BarChart, CartesianGrid, Cell, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { cn } from '../lib/utils'
import { formatNumber } from '../lib/utils'
import { ChartFrame } from './ChartFrame'

export interface LatencyBucket {
  /** 区间标签 — 例如 '0-100ms', '100-300ms', '300ms-1s', '1s+' */
  label: string
  /** 落入该区间的请求/事件数 */
  count: number
  /** 平均值（毫秒）— 可选，用于颜色映射 */
  avgMs?: number
}

export interface LatencyHistogramProps {
  data: LatencyBucket[]
  height?: number | string
  className?: string
  hideHeader?: boolean
  loading?: boolean
}

/**
 * 延迟分布直方图。设计原则：
 *   - X 轴顺序从左到右是慢→快（或反过来），**绝不**按 count 排序
 *   - 柱子按 p95 阈值上色（warning / danger），其余用 chart-1
 *   - 单系列 → 不显示图例（条形图本身就识别性强）
 *   - 永远不显示坐标数值（数值标签放在 tooltip 里）
 */
export function LatencyHistogram({
  data,
  height = 260,
  className,
  hideHeader = false,
  loading = false,
}: LatencyHistogramProps) {
  const isEmpty = data.length === 0
  // p95 阈值：粗略用 (sum * 0.95) 累积定位
  const total = data.reduce((acc, d) => acc + d.count, 0)
  const p95Index = (() => {
    let acc = 0
    for (let i = 0; i < data.length; i++) {
      acc += data[i]!.count
      if (acc / Math.max(total, 1) >= 0.95) return i
    }
    return data.length - 1
  })()

  return (
    <ChartFrame
      title={hideHeader ? undefined : '延迟分布'}
      description={hideHeader ? undefined : 'Step / Turn / Tool 调用按耗时分布'}
      height={height}
      className={className}
      loading={loading}
      skeletonShape="bars"
      empty={!loading && isEmpty}
      emptyMessage="暂无延迟数据"
      emptyDescription="需要至少一条 Step 或 Tool 调用的耗时记录才会出现分布。"
    >
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={data} margin={{ top: 8, right: 12, bottom: 4, left: 4 }}>
          <CartesianGrid vertical={false} stroke="var(--chart-grid)" strokeDasharray="2 4" />
          <XAxis
            dataKey="label"
            tick={{ fill: 'var(--chart-text-muted)', fontSize: 11 }}
            tickLine={false}
            axisLine={{ stroke: 'var(--chart-baseline)' }}
          />
          <YAxis
            tick={{ fill: 'var(--chart-text-muted)', fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(v) => formatNumber(v as number, { notation: 'compact' })}
            width={48}
          />
          <Tooltip
            cursor={{ fill: 'var(--chart-grid)', opacity: 0.6 }}
            content={<LatencyTooltip />}
          />
          <Bar dataKey="count" radius={[4, 4, 0, 0]} isAnimationActive={false}>
            {data.map((_entry, index) => {
              let color = 'var(--chart-1)'
              if (index === p95Index) color = 'var(--warning)'
              if (index > p95Index) color = 'var(--danger)'
              return <Cell key={`cell-${index}`} fill={color} />
            })}
          </Bar>
        </BarChart>
      </ResponsiveContainer>
    </ChartFrame>
  )
}

function LatencyTooltip({
  active,
  payload,
  label,
}: {
  active?: boolean
  payload?: Array<{ value?: number; payload?: LatencyBucket }>
  label?: string
}) {
  if (!active || !payload || payload.length === 0) return null
  const point = payload[0]?.payload
  return (
    <div
      className={cn(
        'rounded-md border px-3 py-2 text-xs shadow-lg',
        'border-[var(--chart-tooltip-border)] bg-[var(--chart-tooltip-bg)]',
      )}
    >
      <div className="mb-1 font-medium text-[var(--chart-text-primary)]">{label}</div>
      <div className="flex items-center gap-3 tabular-nums">
        <span className="text-[var(--chart-text-secondary)]">次数</span>
        <span className="ml-auto text-[var(--chart-text-primary)]">{formatNumber(point?.count)}</span>
      </div>
      {typeof point?.avgMs === 'number' && (
        <div className="flex items-center gap-3 tabular-nums">
          <span className="text-[var(--chart-text-secondary)]">均值</span>
          <span className="ml-auto text-[var(--chart-text-primary)]">{point.avgMs.toFixed(1)} ms</span>
        </div>
      )}
    </div>
  )
}