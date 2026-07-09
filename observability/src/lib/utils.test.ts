import { describe, expect, it } from 'vitest'
import {
  formatCompact,
  formatCostUsd,
  formatDurationMs,
  formatNumber,
  formatPercent,
  formatTokens,
} from './utils'

describe('formatNumber', () => {
  it('returns em-dash for null/undefined/NaN', () => {
    expect(formatNumber(null)).toBe('—')
    expect(formatNumber(undefined)).toBe('—')
    expect(formatNumber(NaN)).toBe('—')
  })

  it('respects custom Intl options', () => {
    expect(formatNumber(1234.5, { maximumFractionDigits: 1 })).toMatch(/1,?234\.5/)
  })
})

describe('formatCompact', () => {
  it('formats small numbers without suffix', () => {
    expect(formatCompact(42)).toMatch(/^42$/)
  })

  it('formats thousands as K', () => {
    expect(formatCompact(1500)).toMatch(/1\.5K/)
  })

  it('formats millions as M', () => {
    expect(formatCompact(2_500_000)).toMatch(/2\.5M/)
  })

  it('handles null gracefully', () => {
    expect(formatCompact(null)).toBe('—')
    expect(formatCompact(NaN)).toBe('—')
  })
})

describe('formatPercent', () => {
  it('formats with default 1 decimal', () => {
    expect(formatPercent(0.123)).toBe('12.3%')
  })

  it('respects custom digit count', () => {
    expect(formatPercent(0.12345, 2)).toBe('12.35%')
    expect(formatPercent(0.5, 0)).toBe('50%')
  })

  it('handles null gracefully', () => {
    expect(formatPercent(null)).toBe('—')
  })
})

describe('formatDurationMs', () => {
  it('formats milliseconds under 1s', () => {
    expect(formatDurationMs(42)).toBe('42 ms')
    expect(formatDurationMs(999)).toBe('999 ms')
  })

  it('formats seconds', () => {
    expect(formatDurationMs(1500)).toBe('1.50 s')
  })

  it('formats minutes', () => {
    expect(formatDurationMs(120_000)).toBe('2.0 min')
  })

  it('formats hours', () => {
    expect(formatDurationMs(3_600_000)).toBe('1.0 h')
  })

  it('handles null gracefully', () => {
    expect(formatDurationMs(null)).toBe('—')
    expect(formatDurationMs(NaN)).toBe('—')
  })
})

describe('formatCostUsd', () => {
  it('formats very small values with 4 decimals', () => {
    expect(formatCostUsd(0.00237)).toBe('$0.0024')
  })

  it('formats sub-dollar values with 3 decimals', () => {
    expect(formatCostUsd(0.42)).toBe('$0.420')
  })

  it('formats dollar values with 2 decimals', () => {
    expect(formatCostUsd(12.5)).toBe('$12.50')
  })

  it('handles null gracefully', () => {
    expect(formatCostUsd(null)).toBe('—')
  })
})

describe('formatTokens', () => {
  it('appends tok suffix', () => {
    expect(formatTokens(500)).toMatch(/tok$/)
  })

  it('uses compact format for large values', () => {
    expect(formatTokens(1500)).toMatch(/1\.5K.*tok/)
  })

  it('handles null gracefully', () => {
    expect(formatTokens(null)).toBe('—')
  })
})