import { useState } from 'react'
import { GrafanaEmbed, resolveGrafanaBaseUrl } from '@hnsx/observability'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSettingsStore } from '@/stores/settingsStore'
import { LocalObservabilityDashboard } from '@/components/observability/LocalObservabilityDashboard'
import { Activity, Settings2, ExternalLink, BarChart3 } from 'lucide-react'
import { cn } from '@/lib/utils'

const DEFAULT_DASHBOARDS = [
  { uid: 'hnsx-overview', label: 'Overview · Session / Cost / Tokens' },
  { uid: 'hnsx-runtime', label: 'Runtime · Latency / Agent hand-off' },
  { uid: 'hnsx-policy', label: 'Policy · Budget / Permission violations' },
]

type Range = '15m' | '1h' | '6h' | '24h' | '7d'
type View = 'grafana' | 'local'

export default function ObservabilityPage() {
  const grafanaUrlOverride = useSettingsStore((s) => s.grafanaUrlOverride)
  const setGrafanaUrlOverride = useSettingsStore((s) => s.setGrafanaUrlOverride)
  const grafanaBase = resolveGrafanaBaseUrl(grafanaUrlOverride)
  const [view, setView] = useState<View>(grafanaBase ? 'grafana' : 'local')
  const [dashboardUid, setDashboardUid] = useState(DEFAULT_DASHBOARDS[0]!.uid)
  const [range, setRange] = useState<Range>('6h')
  const [showSettings, setShowSettings] = useState(false)

  return (
    <div className="space-y-4">
      <PageHeader
        title="Observability"
        description="通过 Grafana 查看 Session / Agent / Cost / Policy 多维度指标。"
      >
        {/* view switcher */}
        <div className="inline-flex h-8 rounded-md border border-[var(--chart-grid)] bg-[var(--card)] p-0.5">
          <ViewTab active={view === 'grafana'} onClick={() => setView('grafana')} icon={<ExternalLink className="h-3.5 w-3.5" />}>
            Grafana
          </ViewTab>
          <ViewTab active={view === 'local'} onClick={() => setView('local')} icon={<BarChart3 className="h-3.5 w-3.5" />}>
            本地视图
          </ViewTab>
        </div>

        {view === 'grafana' && (
          <>
            <select
              value={dashboardUid}
              onChange={(e) => setDashboardUid(e.target.value)}
              className="h-8 rounded-md border border-[var(--chart-grid)] bg-[var(--card)] px-2 text-xs"
              aria-label="Select dashboard"
            >
              {DEFAULT_DASHBOARDS.map((d) => (
                <option key={d.uid} value={d.uid}>
                  {d.label}
                </option>
              ))}
            </select>
            <select
              value={range}
              onChange={(e) => setRange(e.target.value as Range)}
              className="h-8 rounded-md border border-[var(--chart-grid)] bg-[var(--card)] px-2 text-xs"
              aria-label="Time range"
            >
              <option value="15m">最近 15m</option>
              <option value="1h">最近 1h</option>
              <option value="6h">最近 6h</option>
              <option value="24h">最近 24h</option>
              <option value="7d">最近 7d</option>
            </select>
          </>
        )}

        <Button
          size="sm"
          variant="outline"
          onClick={() => setShowSettings((v) => !v)}
          aria-pressed={showSettings}
        >
          <Settings2 className="mr-1.5 h-4 w-4" />
          配置
        </Button>
      </PageHeader>

      {showSettings && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Grafana 配置</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="grafana-url" className="text-xs">
                Grafana Base URL
              </Label>
              <Input
                id="grafana-url"
                placeholder="http://localhost:3000"
                value={grafanaUrlOverride ?? ''}
                onChange={(e) => setGrafanaUrlOverride(e.target.value || null)}
              />
              <p className="text-xs text-muted-foreground">
                也可以通过环境变量{' '}
                <code className="rounded bg-muted px-1 text-[10px]">VITE_HNSX_GRAFANA_URL</code> 配置。
              </p>
            </div>
          </CardContent>
        </Card>
      )}

      {view === 'grafana' ? (
        grafanaBase ? (
          <GrafanaEmbed
            baseUrl={grafanaBase}
            dashboardUid={dashboardUid}
            timeRange={{ kind: 'relative', value: range }}
            theme="auto"
            height={620}
            title={`Grafana ${dashboardUid}`}
          />
        ) : (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-sm">
                <Activity className="h-4 w-4" />
                未配置 Grafana
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm text-muted-foreground">
              <p>
                设置 <code className="rounded bg-muted px-1">VITE_HNSX_GRAFANA_URL</code> 或在
                <em> 配置 </em>面板填入 Grafana Base URL（默认期望 <code className="rounded bg-muted px-1">http://localhost:3000</code>）。
              </p>
              <p>
                仪表盘 UID 需在 Grafana 中预创建（<code className="rounded bg-muted px-1">hnsx-overview</code> /
                <code className="rounded bg-muted px-1">hnsx-runtime</code> /
                <code className="rounded bg-muted px-1">hnsx-policy</code>）。
              </p>
              <p className="pt-2">
                或者切到「本地视图」tab，使用内置 Dashboard。
              </p>
            </CardContent>
          </Card>
        )
      ) : (
        <LocalObservabilityDashboard />
      )}
    </div>
  )
}

function ViewTab({
  active,
  onClick,
  icon,
  children,
}: {
  active: boolean
  onClick: () => void
  icon: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'inline-flex h-7 items-center gap-1.5 rounded px-2.5 text-xs font-medium transition-colors',
        active
          ? 'bg-[var(--chart-grid)] text-[var(--chart-text-primary)]'
          : 'text-[var(--chart-text-muted)] hover:text-[var(--chart-text-primary)]',
      )}
      aria-pressed={active}
    >
      {icon}
      {children}
    </button>
  )
}