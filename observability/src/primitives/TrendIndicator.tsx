import { ArrowDown, ArrowRight, ArrowUp } from 'lucide-react'
import { cn } from '../lib/utils'

export type TrendDirection = 'up' | 'down' | 'flat'

export interface TrendIndicatorProps {
  /** 当前周期值 */
  value: number
  /** 上一周期值（用来算 delta） */
  previous?: number
  /** 直接传 delta 百分比（覆盖由 previous 计算的结果） */
  delta?: number
  /** 当 delta 为正时是否为"好事" — true 表示 success / false 表示 danger */
  goodWhen?: 'up' | 'down'
  /** 格式化函数，默认按 percent 1 位小数 */
  format?: (n: number) => string
  /** 反转颜色语义（默认 false） */
  invertColor?: boolean
  className?: string
}

/**
 * 用于 KPI 卡的同比/环比小标识。
 * 颜色规则：
 *   - 方向（up/down/flat）由 delta 符号决定
 *   - 语义（success/danger）由 invertColor + goodWhen 共同决定
 */
export function TrendIndicator({
  value,
  previous,
  delta,
  goodWhen = 'up',
  format = (n: number) => `${n >= 0 ? '+' : ''}${n.toFixed(1)}%`,
  invertColor = false,
  className,
}: TrendIndicatorProps) {
  let pct: number
  if (typeof delta === 'number') {
    pct = delta
  } else if (typeof previous === 'number' && previous !== 0) {
    pct = ((value - previous) / Math.abs(previous)) * 100
  } else {
    pct = 0
  }

  const direction: TrendDirection = Math.abs(pct) < 0.05 ? 'flat' : pct > 0 ? 'up' : 'down'

  const isPositive = direction === 'up'
  const goodWhenUp = goodWhen === 'up'
  // 默认：up=good (goodWhen='up')；invertColor=true 时反转（用于"错误率下降是好事"）
  const naturallyGood = isPositive === goodWhenUp
  const actuallyGood = invertColor ? !naturallyGood : naturallyGood

  let tone: 'good' | 'bad' | 'neutral'
  if (direction === 'flat') {
    tone = 'neutral'
  } else {
    tone = actuallyGood ? 'good' : 'bad'
  }

  const toneClass =
    tone === 'good'
      ? 'text-[var(--success-text)] bg-[var(--success-soft)]'
      : tone === 'bad'
        ? 'text-[var(--danger-text)] bg-[var(--danger-soft)]'
        : 'text-[var(--chart-text-muted)] bg-[var(--chart-grid)]'

  const Icon = direction === 'up' ? ArrowUp : direction === 'down' ? ArrowDown : ArrowRight

  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium tabular-nums',
        toneClass,
        className,
      )}
      aria-label={`Trend ${format(pct)}`}
    >
      <Icon className="h-3 w-3" />
      {format(pct)}
    </span>
  )
}