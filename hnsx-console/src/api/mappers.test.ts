import { describe, it, expect } from 'vitest'
import {
  toDate,
  formatDate,
  toNumber,
  toInt,
  parseJsonField,
  mapTraceSummaryFromJson,
  mapDomainSummaryFromJson,
  extractAgentRefs,
} from './mappers'

describe('toDate', () => {
  it('returns null for empty values', () => {
    expect(toDate(undefined)).toBeNull()
    expect(toDate(null)).toBeNull()
    expect(toDate('')).toBeNull()
  })

  it('parses millisecond timestamps', () => {
    const date = toDate(1_700_000_000_000)
    expect(date).toBeInstanceOf(Date)
    expect(date?.getFullYear()).toBe(2023)
  })

  it('parses bigint timestamps', () => {
    const date = toDate(1_700_000_000_000n)
    expect(date).toBeInstanceOf(Date)
    expect(date?.getFullYear()).toBe(2023)
  })

  it('parses ISO strings', () => {
    const date = toDate('2024-06-01T12:00:00.000Z')
    expect(date?.toISOString()).toBe('2024-06-01T12:00:00.000Z')
  })
})

describe('formatDate', () => {
  it('formats dates as expected', () => {
    expect(formatDate(new Date('2024-06-01T12:00:00.000Z'))).toMatch(/2024-06-01/)
  })

  it('returns dash for null', () => {
    expect(formatDate(null)).toBe('-')
  })
})

describe('toNumber', () => {
  it('returns finite numbers', () => {
    expect(toNumber(3.14)).toBe(3.14)
    expect(toNumber('2.5')).toBe(2.5)
  })

  it('returns 0 for invalid input', () => {
    expect(toNumber(NaN)).toBe(0)
    expect(toNumber('abc')).toBe(0)
    expect(toNumber(null)).toBe(0)
    expect(toNumber(undefined)).toBe(0)
  })
})

describe('toInt', () => {
  it('truncates floats', () => {
    expect(toInt(3.9)).toBe(3)
    expect(toInt(-1.2)).toBe(-1)
  })
})

describe('parseJsonField', () => {
  it('parses valid JSON strings', () => {
    expect(parseJsonField('{"a":1}')).toEqual({ a: 1 })
  })

  it('returns empty object for empty input', () => {
    expect(parseJsonField('')).toEqual({})
    expect(parseJsonField(null)).toEqual({})
  })

  it('returns raw string for invalid JSON', () => {
    expect(parseJsonField('not json')).toBe('not json')
  })
})

describe('mapTraceSummaryFromJson', () => {
  it('maps snake_case fields with defaults', () => {
    const summary = mapTraceSummaryFromJson({
      trace_id: 't1',
      session_id: 's1',
      domain_id: 'd1',
      domain_version: 'v1',
      status: 'completed',
      started_at: '2024-06-01T12:00:00.000Z',
      observation_count: 10,
      total_cost_usd: 0.123,
      prompt_tokens: 100,
      completion_tokens: 50,
      agent_invocations: 2,
      tool_invocations: 3,
    })

    expect(summary.traceId).toBe('t1')
    expect(summary.sessionId).toBe('s1')
    expect(summary.status).toBe('completed')
    expect(summary.observationCount).toBe(10)
    expect(summary.totalCostUsd).toBe(0.123)
    expect(summary.promptTokens).toBe(100)
    expect(summary.startedAt?.toISOString()).toBe('2024-06-01T12:00:00.000Z')
  })

  it('defaults numeric fields to 0', () => {
    const summary = mapTraceSummaryFromJson({
      trace_id: 't1',
      session_id: 's1',
      domain_id: 'd1',
      domain_version: 'v1',
    })

    expect(summary.observationCount).toBe(0)
    expect(summary.totalCostUsd).toBe(0)
    expect(summary.promptTokens).toBe(0)
    expect(summary.status).toBe('unknown')
  })
})

describe('mapDomainSummaryFromJson', () => {
  it('maps fields and defaults status', () => {
    const summary = mapDomainSummaryFromJson({
      id: 'd1',
      version: 'v1',
      description: 'test domain',
    })

    expect(summary.id).toBe('d1')
    expect(summary.status).toBe('active')
    expect(summary.createdAt).toBeNull()
    expect(summary.updatedAt).toBeNull()
  })
})

describe('extractAgentRefs', () => {
  it('collects unique agent ids', () => {
    const refs = extractAgentRefs([
      { agent_id: 'a1' },
      { agentId: 'a2' },
      { agent_id: 'a1' },
      'ignored',
    ])
    expect(refs).toEqual(['a1', 'a2'])
  })
})
