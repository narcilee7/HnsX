import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { ExternalLink, RefreshCw } from 'lucide-react'
import { cn } from '../lib/utils'

export type GrafanaTimeRange =
  | { kind: 'relative'; value: '5m' | '15m' | '1h' | '6h' | '12h' | '24h' | '7d' | '30d' }
  | { kind: 'absolute'; from: string; to: string }

export interface GrafanaEmbedProps {
  /** Grafana base URL — 例如 http://localhost:3000 */
  baseUrl: string
  /** Dashboard UID（推荐用 UID，比 ID 更稳定） */
  dashboardUid: string
  /** 聚焦到单个 panel；不传则展示整个 dashboard */
  panelId?: number
  /** 时间范围 — 默认 '6h' */
  timeRange?: GrafanaTimeRange
  /** 主题 — 默认 'auto' 跟随宿主 */
  theme?: 'light' | 'dark' | 'auto'
  /** Dashboard 模板变量 */
  vars?: Record<string, string>
  /** 自定义高度 — 默认 600 */
  height?: number | string
  className?: string
  /** 不画 chrome 的"独立 panel"模式 — 默认 true */
  solo?: boolean
  /** 加载失败 / 配置缺失时显示的 fallback */
  fallback?: ReactNode
  /** iframe 标题 — 用于无障碍 */
  title?: string
}

/**
 * Grafana iframe 嵌入组件。
 *
 * 设计原则：
 *   - 永远用 d-solo URL（去掉 dashboard chrome，加载更快）
 *   - theme 通过 URL ?theme= 透传，dark mode 自动同步
 *   - time range 走 Grafana 原生格式 `from=now-6h&to=now`
 *   - sandbox + referrerPolicy + loading="lazy" — 最小权限嵌入
 *   - 配置缺失时不报红，给一个清晰的下一步引导
 */
export function GrafanaEmbed({
  baseUrl,
  dashboardUid,
  panelId,
  timeRange = { kind: 'relative', value: '6h' },
  theme = 'auto',
  vars = {},
  height = 600,
  className,
  solo = true,
  fallback,
  title,
}: GrafanaEmbedProps) {
  const [hostTheme, setHostTheme] = useState<'light' | 'dark'>(() => detectTheme())
  const [reloadKey, setReloadKey] = useState(0)
  const [loaded, setLoaded] = useState(false)

  // 监听宿主主题变化（dark class 切换）
  useEffect(() => {
    if (theme !== 'auto') return
    const observer = new MutationObserver(() => setHostTheme(detectTheme()))
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
    return () => observer.disconnect()
  }, [theme])

  const url = useMemo(() => {
    const params = new URLSearchParams()
    if (panelId !== undefined) params.set('panelId', String(panelId))
    if (solo) params.set('__feature.dashboardSolo', 'true')
    // theme
    const effectiveTheme = theme === 'auto' ? hostTheme : theme
    params.set('theme', effectiveTheme)
    // time range
    if (timeRange.kind === 'relative') {
      params.set('from', `now-${timeRange.value}`)
      params.set('to', 'now')
    } else {
      params.set('from', timeRange.from)
      params.set('to', timeRange.to)
    }
    // vars
    for (const [k, v] of Object.entries(vars)) {
      params.set(`var-${k}`, v)
    }
    const path = solo ? '/d-solo/' : '/d/'
    return `${stripTrailingSlash(baseUrl)}${path}${dashboardUid}?${params.toString()}`
  }, [baseUrl, dashboardUid, panelId, timeRange, theme, hostTheme, vars, solo])

  const showFallback = !baseUrl || !dashboardUid

  return (
    <div
      className={cn(
        'relative overflow-hidden rounded-lg border border-[var(--chart-grid)] bg-[var(--card)]',
        className,
      )}
      style={{ height }}
    >
      {showFallback ? (
        fallback ?? <DefaultFallback />
      ) : (
        <>
          {!loaded && (
            <div className="absolute inset-0 flex items-center justify-center bg-[var(--chart-grid)]/40">
              <div className="flex items-center gap-2 text-sm text-[var(--chart-text-muted)]">
                <RefreshCw className="h-4 w-4 animate-spin" />
                加载 Grafana…
              </div>
            </div>
          )}
          <iframe
            key={reloadKey}
            src={url}
            title={title ?? `Grafana dashboard ${dashboardUid}`}
            loading="lazy"
            referrerPolicy="no-referrer-when-downgrade"
            sandbox="allow-scripts allow-same-origin allow-popups allow-forms"
            allow="fullscreen"
            onLoad={() => setLoaded(true)}
            className={cn('h-full w-full border-0', !loaded && 'opacity-0')}
          />
          {/* 顶部覆盖一条 toolbar：在新窗口打开 + 强制刷新 */}
          <div className="pointer-events-none absolute right-2 top-2 flex gap-1">
            <a
              href={url}
              target="_blank"
              rel="noreferrer"
              className="pointer-events-auto inline-flex h-7 items-center gap-1 rounded-md border border-[var(--chart-grid)] bg-[var(--card)] px-2 text-xs text-[var(--chart-text-secondary)] shadow-sm hover:bg-[var(--chart-grid)]/40"
              aria-label="Open in new tab"
            >
              <ExternalLink className="h-3 w-3" />
              新窗口打开
            </a>
            <button
              type="button"
              onClick={() => {
                setLoaded(false)
                setReloadKey((k) => k + 1)
              }}
              className="pointer-events-auto inline-flex h-7 w-7 items-center justify-center rounded-md border border-[var(--chart-grid)] bg-[var(--card)] text-[var(--chart-text-secondary)] shadow-sm hover:bg-[var(--chart-grid)]/40"
              aria-label="Reload dashboard"
            >
              <RefreshCw className="h-3 w-3" />
            </button>
          </div>
        </>
      )}
    </div>
  )
}

function DefaultFallback() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
      <p className="text-sm font-medium text-[var(--chart-text-primary)]">未配置 Grafana</p>
      <p className="max-w-sm text-xs text-[var(--chart-text-muted)]">
        在 <code className="rounded bg-[var(--chart-grid)]/60 px-1">hnsx-console</code> 的环境变量里设置{' '}
        <code className="rounded bg-[var(--chart-grid)]/60 px-1">VITE_HNSX_GRAFANA_URL</code> 即可启用 Dashboard 嵌入。
      </p>
    </div>
  )
}

function detectTheme(): 'light' | 'dark' {
  if (typeof document === 'undefined') return 'light'
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}

function stripTrailingSlash(s: string): string {
  return s.endsWith('/') ? s.slice(0, -1) : s
}

/**
 * 读取 Grafana base URL。优先环境变量，其次 Settings store 的 override。
 * 在浏览器侧用；不会被打进 SSR。
 */
export function resolveGrafanaBaseUrl(override?: string | null): string | null {
  if (override) return override
  // Vite 注入
  const env =
    (typeof import.meta !== 'undefined' &&
      (import.meta as { env?: Record<string, string | undefined> }).env?.VITE_HNSX_GRAFANA_URL) ||
    undefined
  if (env) return env
  return null
}