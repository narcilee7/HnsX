/**
 * 类型化的 palette 常量 — 组件层可以用 enum/switch 决定颜色槽位，
 * 避免到处写 magic string。
 */

export const CHART_SLOTS = ['chart-1', 'chart-2', 'chart-3', 'chart-4', 'chart-5'] as const
export type ChartSlot = (typeof CHART_SLOTS)[number]

export const STATUS_TOKENS = ['success', 'warning', 'danger', 'info'] as const
export type StatusToken = (typeof STATUS_TOKENS)[number]

/**
 * 返回语义槽位对应的 CSS 变量名（含 var() 包裹）。
 * 配合 fill-[var(--xxx)] 或者 recharts 的 style={{ fill: cssVar('chart-1') }}。
 */
export function chartVar(slot: ChartSlot): string {
  return `var(--${slot})`
}

export function statusVar(token: StatusToken): string {
  return `var(--${token})`
}

/**
 * 数字系列按索引取 chart 槽位（固定顺序、绝不循环）。
 * 第 9 个系列应折叠到 "Other" / small multiples，而不是新生成颜色。
 */
export function slotFor(index: number): ChartSlot {
  return CHART_SLOTS[index % CHART_SLOTS.length] as ChartSlot
}