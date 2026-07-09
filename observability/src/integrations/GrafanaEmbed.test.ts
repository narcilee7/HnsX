import { describe, expect, it } from 'vitest'
import { buildGrafanaUrl, resolveGrafanaBaseUrl } from './GrafanaEmbed'

const baseOpts = {
  baseUrl: 'http://localhost:3000',
  dashboardUid: 'hnsx-overview',
  timeRange: { kind: 'relative' as const, value: '6h' as const },
  theme: 'auto' as const,
  hostTheme: 'light' as const,
  vars: {},
  solo: true,
}

describe('buildGrafanaUrl', () => {
  it('uses d-solo path by default', () => {
    const url = buildGrafanaUrl(baseOpts)
    expect(url).toMatch(/^http:\/\/localhost:3000\/d-solo\/hnsx-overview\?/)
  })

  it('uses /d/ when solo=false', () => {
    const url = buildGrafanaUrl({ ...baseOpts, solo: false })
    expect(url).toMatch(/^http:\/\/localhost:3000\/d\/hnsx-overview\?/)
  })

  it('strips trailing slash from baseUrl', () => {
    const url = buildGrafanaUrl({ ...baseOpts, baseUrl: 'http://localhost:3000/' })
    expect(url).toMatch(/^http:\/\/localhost:3000\/d-solo\//)
    expect(url).not.toMatch(/3000\/\/d-solo/)
  })

  it('includes panelId param', () => {
    const url = buildGrafanaUrl({ ...baseOpts, panelId: 4 })
    expect(url).toContain('panelId=4')
  })

  it('omits panelId when undefined', () => {
    const url = buildGrafanaUrl(baseOpts)
    expect(url).not.toContain('panelId')
  })

  it('translates relative time range to from=now-X&to=now', () => {
    const url = buildGrafanaUrl({
      ...baseOpts,
      timeRange: { kind: 'relative', value: '24h' },
    })
    expect(url).toContain('from=now-24h')
    expect(url).toContain('to=now')
  })

  it('passes through absolute time range', () => {
    const url = buildGrafanaUrl({
      ...baseOpts,
      timeRange: { kind: 'absolute', from: '2026-07-01T00:00:00Z', to: '2026-07-09T23:59:59Z' },
    })
    expect(url).toContain('from=2026-07-01T00%3A00%3A00Z')
    expect(url).toContain('to=2026-07-09T23%3A59%3A59Z')
  })

  it('uses explicit theme when not auto', () => {
    const url = buildGrafanaUrl({ ...baseOpts, theme: 'dark', hostTheme: 'light' })
    expect(url).toContain('theme=dark')
  })

  it('falls back to hostTheme when theme=auto', () => {
    const url = buildGrafanaUrl({ ...baseOpts, theme: 'auto', hostTheme: 'dark' })
    expect(url).toContain('theme=dark')
  })

  it('prefixes vars with var-', () => {
    const url = buildGrafanaUrl({
      ...baseOpts,
      vars: { domain: 'customer-service', env: 'prod' },
    })
    expect(url).toContain('var-domain=customer-service')
    expect(url).toContain('var-env=prod')
  })

  it('sets solo feature flag only when solo=true', () => {
    const withSolo = buildGrafanaUrl({ ...baseOpts, solo: true })
    const withoutSolo = buildGrafanaUrl({ ...baseOpts, solo: false })
    expect(withSolo).toContain('__feature.dashboardSolo=true')
    expect(withoutSolo).not.toContain('__feature.dashboardSolo')
  })
})

describe('resolveGrafanaBaseUrl', () => {
  it('returns override when provided', () => {
    expect(resolveGrafanaBaseUrl('https://grafana.example.com')).toBe('https://grafana.example.com')
  })

  it('returns null when override is null and no env', () => {
    expect(resolveGrafanaBaseUrl(null)).toBeNull()
  })
})