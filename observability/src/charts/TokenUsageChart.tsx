import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { cn } from '../lib/utils'
import { formatCompact, formatNumber } from '../lib/utils'
import { ChartFrame } from './ChartFrame'

export interface TokenUsagePoint {
  /** X 轴标签 — 例如 '10:00' 或 '07-09' */
  label: string
  /** 输入 token */
  input?: number
  /** 输出 token */
  output?: number
  /** 总 token（可选；如果没传 input/output，会回退到这里） */
  total?: number
}

export interface TokenUsageChartProps {
  data: TokenUsagePoint[]
  height?: number | string
  className?: string
  /** 隐藏标题 — playground / 嵌入场景 */
  hideHeader?: boolean
  /** 显示输入/输出拆解 — 默认 true */
  stacked?: boolean
  /** 加载态 — 显示骨架屏 */
  loading?: boolean
}

/**
 * Token 用量趋势图。设计原则：
 *   - 双系列 stacked area（input/output），用 chart-1 / chart-2
 *   - X 轴细标签 + grid hairline
 *   - Tooltip 三行（input / output / total）
 *   - 永远单 Y 轴，绝不双 Y 轴
 */
export function TokenUsageChart({
  data,
  height = 280,
  className,
  hideHeader = false,
  stacked = true,
  loading = false,
}: TokenUsageChartProps) {
  const hasSplit = data.some((d) => typeof d.input === 'number' || typeof d.output === 'number')
  const isEmpty = data.length === 0

  return (
    <ChartFrame
      title={hideHeader ? undefined : 'Token 用量趋势'}
      description={hideHeader ? undefined : '按时间窗口聚合的 input / output token 数'}
      height={height}
      className={className}
      loading={loading}
      skeletonShape="line"
      empty={!loading && isEmpty}
      emptyMessage="尚无 token 用量数据"
      emptyDescription="Session 触发后，按时间窗口聚合的 token 消耗会在这里显示。"
    >
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={data} margin={{ top: 8, right: 12, bottom: 4, left: 4 }}>
          <defs>
            <linearGradient id="tok-input-grad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--chart-3)" stopOpacity={0.55} />
              <stop offset="100%" stopColor="var(--chart-3)" stopOpacity={0.05} />
            </linearGradient>
            <linearGradient id="tok-output-grad" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--chart-1)" stopOpacity={0.6} />
              <stop offset="100%" stopColor="var(--chart-1)" stopOpacity={0.05} />
            </linearGradient>
          </defs>
          <CartesianGrid
            vertical={false}
            stroke="var(--chart-grid)"
            strokeDasharray="2 4"
          />
          <XAxis
            dataKey="label"
            tick={{ fill: 'var(--chart-text-muted)', fontSize: 11 }}
            tickLine={false}
            axisLine={{ stroke: 'var(--chart-baseline)' }}
            interval="preserveStartEnd"
            minTickGap={24}
          />
          <YAxis
            tick={{ fill: 'var(--chart-text-muted)', fontSize: 11 }}
            tickLine={false}
            axisLine={false}
            tickFormatter={(v) => formatCompact(v as number)}
            width={48}
          />
          <Tooltip
            cursor={{ stroke: 'var(--chart-baseline)', strokeDasharray: '2 4' }}
            content={<TokenTooltip stacked={stacked} />}
          />
          {hasSplit ? (
            <>
              <Area
                type="monotone"
                dataKey="input"
                stackId="tokens"
                stroke="var(--chart-3)"
                strokeWidth={1.75}
                fill="url(#tok-input-grad)"
                isAnimationActive={false}
              />
              <Area
                type="monotone"
                dataKey="output"
                stackId="tokens"
                stroke="var(--chart-1)"
                strokeWidth={1.75}
                fill="url(#tok-output-grad)"
                isAnimationActive={false}
              />
            </>
          ) : (
            <Area
              type="monotone"
              dataKey="total"
              stroke="var(--chart-1)"
              strokeWidth={1.75}
              fill="url(#tok-output-grad)"
              isAnimationActive={false}
            />
          )}
        </AreaChart>
      </ResponsiveContainer>
    </ChartFrame>
  )
}

function TokenTooltip({
  active,
  payload,
  label,
  stacked,
}: {
  active?: boolean
  payload?: Array<{ name?: string; value?: number; dataKey?: string; color?: string; payload?: TokenUsagePoint }>
  label?: string
  stacked: boolean
}) {
  if (!active || !payload || payload.length === 0) return null
  const point = (payload[0]?.payload ?? {}) as TokenUsagePoint
  const input = point.input ?? 0
  const output = point.output ?? point.total ?? 0

  return (
    <div
      className={cn(
        'rounded-md border px-3 py-2 text-xs shadow-lg',
        'border-[var(--chart-tooltip-border)] bg-[var(--chart-tooltip-bg)]',
      )}
    >
      <div className="mb-1 font-medium text-[var(--chart-text-primary)]">{label}</div>
      {stacked && (
        <div className="flex items-center gap-2 tabular-nums">
          <span className="h-2 w-2 rounded-full" style={{ background: 'var(--chart-3)' }} />
          <span className="text-[var(--chart-text-secondary)]">input</span>
          <span className="ml-auto text-[var(--chart-text-primary)]">{formatNumber(input)}</span>
        </div>
      )}
      <div className="flex items-center gap-2 tabular-nums">
        <span className="h-2 w-2 rounded-full" style={{ background: 'var(--chart-1)' }} />
        <span className="text-[var(--chart-text-secondary)]">output</span>
        <span className="ml-auto text-[var(--chart-text-primary)]">{formatNumber(output)}</span>
      </div>
      {stacked && (
        <div className="mt-1 flex items-center gap-2 border-t border-[var(--chart-grid)] pt-1 tabular-nums">
          <span className="text-[var(--chart-text-secondary)]">total</span>
          <span className="ml-auto font-medium text-[var(--chart-text-primary)]">
            {formatNumber(input + output)}
          </span>
        </div>
      )}
    </div>
  )
}