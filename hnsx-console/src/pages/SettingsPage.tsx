import { useState } from 'react'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Timestamp } from '@/components/ui/Timestamp'
import { Loading } from '@/components/ui/Loading'
import { Empty } from '@/components/ui/Empty'
import { ErrorState } from '@/components/ui/Error'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  useSecrets,
  useCreateSecret,
  useDeleteSecret,
  usePolicies,
  useRuntimes,
} from '@/hooks/useSettings'
import type { Secret } from '@/api/settings'
import { KeyRound, Plus, Trash2, ShieldCheck, Server, AlertCircle } from 'lucide-react'

export default function SettingsPage() {
  return (
    <div className="space-y-4">
      <PageHeader
        title="Settings"
        description="管理 Secret、Policy 和 Runtime worker 的注册信息。"
      />
      <Tabs defaultValue="secrets">
        <TabsList>
          <TabsTrigger value="secrets">
            <KeyRound className="mr-1.5 h-4 w-4" />
            Secrets
          </TabsTrigger>
          <TabsTrigger value="policies">
            <ShieldCheck className="mr-1.5 h-4 w-4" />
            Policies
          </TabsTrigger>
          <TabsTrigger value="runtimes">
            <Server className="mr-1.5 h-4 w-4" />
            Runtimes
          </TabsTrigger>
        </TabsList>
        <TabsContent value="secrets">
          <SecretsTab />
        </TabsContent>
        <TabsContent value="policies">
          <PoliciesTab />
        </TabsContent>
        <TabsContent value="runtimes">
          <RuntimesTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}

/* -------------------------------------------------------------------------- */
/*                                   Secrets                                   */
/* -------------------------------------------------------------------------- */

function SecretsTab() {
  const { data, isLoading, error, refetch } = useSecrets()
  const createSecret = useCreateSecret()
  const deleteSecret = useDeleteSecret()
  const [createOpen, setCreateOpen] = useState(false)
  const [draftId, setDraftId] = useState('')
  const [draftValue, setDraftValue] = useState('')
  const [draftProvider, setDraftProvider] = useState('env')

  const handleCreate = async () => {
    await createSecret.mutateAsync({
      id: draftId,
      value: draftValue,
      provider: draftProvider,
    })
    setCreateOpen(false)
    setDraftId('')
    setDraftValue('')
    setDraftProvider('env')
  }

  if (error) return <ErrorState description={error.message} onRetry={refetch} />

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <div>
            <CardTitle className="text-base">Secret Registry</CardTitle>
            <p className="mt-1 text-xs text-muted-foreground">
              集中存储 model API key / DB password / OAuth token。值仅在创建时提交，列表只显示 fingerprint。
            </p>
          </div>
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="mr-1.5 h-4 w-4" />
            New Secret
          </Button>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Loading />
          ) : !data?.items.length ? (
            <Empty description="还没有 Secret。点击 New Secret 添加 model API key、DB 凭证等。" />
          ) : (
            <SecretTable
              items={data.items}
              onDelete={(id) => {
                if (confirm(`Delete secret "${id}"?`)) deleteSecret.mutate(id)
              }}
            />
          )}
        </CardContent>
      </Card>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Create Secret</DialogTitle>
            <DialogDescription>
              ID 之后不能改。值会加密入库，列表只显示 fingerprint。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="secret-id">ID</Label>
              <Input
                id="secret-id"
                placeholder="openai-api-key"
                value={draftId}
                onChange={(e) => setDraftId(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="secret-provider">Provider</Label>
              <select
                id="secret-provider"
                value={draftProvider}
                onChange={(e) => setDraftProvider(e.target.value)}
                className="h-9 w-full rounded-md border border-input bg-transparent px-3 text-sm"
              >
                <option value="env">env</option>
                <option value="vault">vault</option>
                <option value="aws-sm">aws-sm</option>
              </select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="secret-value">Value</Label>
              <textarea
                id="secret-value"
                placeholder="secret value..."
                value={draftValue}
                onChange={(e) => setDraftValue(e.target.value)}
                className="min-h-[80px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm font-mono"
              />
            </div>
            <div className="flex items-start gap-2 rounded-md border border-[var(--warning)]/30 bg-[var(--warning-soft)] p-2 text-xs text-[var(--warning-text)]">
              <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>Phase 1 stub：后端 CreateSecret 返回 ADAPTER_NOT_IMPLEMENTED。Phase 2 上线加密存储。</span>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleCreate}
              disabled={!draftId || !draftValue || createSecret.isPending}
            >
              {createSecret.isPending ? 'Creating…' : 'Create'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function SecretTable({
  items,
  onDelete,
}: {
  items: Secret[]
  onDelete: (id: string) => void
}) {
  return (
    <div className="divide-y">
      {items.map((s) => (
        <div key={s.id} className="flex items-center justify-between py-3 first:pt-0 last:pb-0">
          <div className="min-w-0 space-y-1">
            <div className="flex items-center gap-2">
              <span className="font-mono text-sm font-medium">{s.id}</span>
              {s.scope && (
                <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                  {s.scope}
                </span>
              )}
            </div>
            <div className="flex items-center gap-3 text-xs text-muted-foreground">
              {s.provider && <span>provider: {s.provider}</span>}
              {s.fingerprint && (
                <span className="font-mono">fp: {s.fingerprint}</span>
              )}
              {s.updated_at && <Timestamp date={new Date(s.updated_at)} />}
            </div>
          </div>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => onDelete(s.id)}
            aria-label={`Delete ${s.id}`}
          >
            <Trash2 className="h-4 w-4 text-destructive" />
          </Button>
        </div>
      ))}
    </div>
  )
}

/* -------------------------------------------------------------------------- */
/*                                  Policies                                   */
/* -------------------------------------------------------------------------- */

function PoliciesTab() {
  const { data, isLoading, error, refetch } = usePolicies()

  if (error) return <ErrorState description={error.message} onRetry={refetch} />

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Policy Registry</CardTitle>
        <p className="mt-1 text-xs text-muted-foreground">
          Budget / Permission / Guardrail 规则。Phase 1 只读，规则通过 DomainSpec YAML 注入。
        </p>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Loading />
        ) : !data?.items.length ? (
          <Empty description="目前没有 Policy。规则随 DomainSpec 注入，每个 Domain 独立管理。" />
        ) : (
          <div className="divide-y">
            {data.items.map((p) => (
              <div key={p.id} className="flex items-start justify-between gap-4 py-3 first:pt-0 last:pb-0">
                <div className="min-w-0 space-y-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm font-medium">{p.id}</span>
                    {p.scope && (
                      <span className="rounded bg-muted px-1.5 py-0.5 text-[10px]">
                        {p.scope}
                      </span>
                    )}
                    {p.effect && (
                      <span
                        className={
                          p.effect === 'deny'
                            ? 'rounded bg-[var(--danger-soft)] px-1.5 py-0.5 text-[10px] text-[var(--danger-text)]'
                            : p.effect === 'allow'
                              ? 'rounded bg-[var(--success-soft)] px-1.5 py-0.5 text-[10px] text-[var(--success-text)]'
                              : 'rounded bg-[var(--warning-soft)] px-1.5 py-0.5 text-[10px] text-[var(--warning-text)]'
                        }
                      >
                        {p.effect}
                      </span>
                    )}
                  </div>
                  {p.description && (
                    <p className="text-xs text-muted-foreground">{p.description}</p>
                  )}
                  {p.rule && (
                    <pre className="mt-1 max-w-2xl overflow-x-auto rounded bg-muted px-2 py-1 text-[11px]">
                      {p.rule}
                    </pre>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

/* -------------------------------------------------------------------------- */
/*                                  Runtimes                                   */
/* -------------------------------------------------------------------------- */

function RuntimesTab() {
  const { data, isLoading, error, refetch } = useRuntimes()

  if (error) return <ErrorState description={error.message} onRetry={refetch} />

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Runtime Workers</CardTitle>
        <p className="mt-1 text-xs text-muted-foreground">
          Session 执行所在的 worker。心跳每 10s 刷新一次。
        </p>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Loading />
        ) : !data?.items.length ? (
          <Empty description="还没有 Runtime worker 注册。" />
        ) : (
          <div className="overflow-hidden rounded-lg border">
            <table className="w-full text-sm">
              <thead className="bg-muted/50 text-xs text-muted-foreground">
                <tr>
                  <th className="px-3 py-2 text-left font-medium">Runtime ID</th>
                  <th className="px-3 py-2 text-left font-medium">Host</th>
                  <th className="px-3 py-2 text-left font-medium">Region</th>
                  <th className="px-3 py-2 text-left font-medium">Capabilities</th>
                  <th className="px-3 py-2 text-left font-medium">Status</th>
                  <th className="px-3 py-2 text-right font-medium">Slots</th>
                  <th className="px-3 py-2 text-right font-medium">Last Heartbeat</th>
                </tr>
              </thead>
              <tbody className="divide-y">
                {data.items.map((rt) => (
                  <tr key={rt.runtime_id} className="hover:bg-muted/30">
                    <td className="px-3 py-2">
                      <div className="font-mono text-xs">{rt.runtime_id}</div>
                      {rt.version ? (
                        <div className="text-[10px] text-muted-foreground">v{rt.version}</div>
                      ) : null}
                    </td>
                    <td className="px-3 py-2 text-xs">
                      {rt.hostname ?? '—'}
                      {rt.pid ? (
                        <span className="ml-1 text-muted-foreground">pid {rt.pid}</span>
                      ) : null}
                    </td>
                    <td className="px-3 py-2 text-xs">{rt.region ?? '—'}</td>
                    <td className="px-3 py-2 text-xs">
                      {rt.capabilities?.length ? (
                        <div className="flex flex-wrap gap-1">
                          {rt.capabilities.slice(0, 4).map((c) => (
                            <span
                              key={c}
                              className="inline-flex items-center rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-medium"
                            >
                              {c}
                            </span>
                          ))}
                          {rt.capabilities.length > 4 ? (
                            <span className="text-[10px] text-muted-foreground">
                              +{rt.capabilities.length - 4}
                            </span>
                          ) : null}
                        </div>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td className="px-3 py-2">
                      <StatusBadge status={rt.status ?? 'unknown'} />
                    </td>
                    <td className="px-3 py-2 text-right text-xs tabular-nums">
                      {rt.active_sessions ?? 0}
                      {rt.capacity ? <span className="text-muted-foreground">/{rt.capacity}</span> : null}
                    </td>
                    <td className="px-3 py-2 text-right text-xs">
                      {rt.last_heartbeat_at ? (
                        <Timestamp date={new Date(rt.last_heartbeat_at)} />
                      ) : (
                        '—'
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}