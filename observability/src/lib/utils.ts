import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}

/**
 * 数字格式：默认 locale-aware；支持 compact 模式（1.2K / 3.4M）。
 */
export function formatNumber(value: number | null | undefined, opts: Intl.NumberFormatOptions = {}): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return new Intl.NumberFormat(undefined, opts).format(value)
}

export function formatCompact(value: number | null | undefined): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(value)
}

export function formatPercent(value: number | null | undefined, digits = 1): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '—'
  return new Intl.NumberFormat(undefined, {
    style: 'percent',
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(value)
}

export function formatDurationMs(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || Number.isNaN(ms)) return '—'
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)} s`
  if (ms < 3_600_000) return `${(ms / 60_000).toFixed(1)} min`
  return `${(ms / 3_600_000).toFixed(1)} h`
}

export function formatCostUsd(usd: number | null | undefined): string {
  if (usd === null || usd === undefined || Number.isNaN(usd)) return '—'
  if (usd < 0.01) return `$${usd.toFixed(4)}`
  if (usd < 1) return `$${usd.toFixed(3)}`
  return `$${usd.toFixed(2)}`
}

export function formatTokens(tokens: number | null | undefined): string {
  if (tokens === null || tokens === undefined || Number.isNaN(tokens)) return '—'
  return `${formatCompact(tokens)} tok`
}