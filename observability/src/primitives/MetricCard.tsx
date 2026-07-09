import type { ReactNode } from 'react'
import { cn } from '../lib/utils'

export interface MetricCardProps {
  /** 标题 — 用语义化短语，不是 "Total Sessions"，而是 "今日 Session 数" */
  label: string
  /** 主数值，可以是 string/number/ReactNode，便于放自定义内容 */
  value: ReactNode
  /** 主数值的格式化函数（仅当 value 是 number 时生效） */
  formatValue?: (n: number) => string
  /** 单位 — 例如 "USD"、"ms"、"req/s" */
  unit?: string
  /** 副标题/上下文 — 例如 "今日" / "对比昨日" */
  caption?: ReactNode
  /** 右上角装饰区 — 通常放 TrendIndicator */
  trend?: ReactNode
  /** 底部装饰区 — 通常放 Sparkline */
  footer?: ReactNode
  /** 点击行为 — 不传则不可点击 */
  onClick?: () => void
  className?: string
}

/**
 * 通用 KPI 卡。
 * 设计原则：
 *   - 信息密度优先（一卡一指标）
 *   - 左侧色条标识当前 metric 的语义槽位（默认 chart-1）
 *   - 自包含 markup：不依赖宿主 Card 组件，保证包内独立可用
 */
export function MetricCard({
  label,
  value,
  formatValue,
  unit,
  caption,
  trend,
  footer,
  onClick,
  className,
}: MetricCardProps) {
  const isClickable = typeof onClick === 'function'
  const renderedValue =
    typeof value === 'number' && formatValue ? (
      <span className="tabular-nums">{formatValue(value)}</span>
    ) : (
      value
    )

  return (
    <div
      onClick={onClick}
      className={cn(
        'relative overflow-hidden rounded-lg border border-[var(--chart-grid)] bg-[var(--card)] p-4 shadow-sm',
        'border-l-4 border-l-[var(--chart-1)]',
        'transition-all duration-200',
        isClickable && 'cursor-pointer hover:-translate-y-0.5 hover:bg-[var(--chart-grid)]/40 hover:shadow-md',
        className,
      )}
    >
      <div className="flex flex-row items-start justify-between gap-2 pb-2">
        <div className="space-y-0.5">
          <h3 className="text-sm font-medium text-[var(--chart-text-secondary)]">{label}</h3>
          {caption && <p className="text-xs text-[var(--chart-text-muted)]">{caption}</p>}
        </div>
        {trend}
      </div>
      <div className="space-y-3">
        <div className="flex items-baseline gap-1.5">
          <div className="text-2xl font-semibold tabular-nums text-[var(--chart-text-primary)]">
            {renderedValue}
          </div>
          {unit && <span className="text-sm text-[var(--chart-text-muted)]">{unit}</span>}
        </div>
        {footer}
      </div>
    </div>
  )
}