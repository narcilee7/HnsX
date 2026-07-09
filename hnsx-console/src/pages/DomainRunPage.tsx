import { useEffect, useMemo, useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import Editor from '@monaco-editor/react'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { PageHeader } from '@/components/ui/PageHeader'
import { Loading } from '@/components/ui/Loading'
import { ErrorState } from '@/components/ui/Error'
import { useCreateSession, useSessions } from '@/hooks/useSessions'
import { useDomain } from '@/hooks/useDomains'
import { cn } from '@/lib/utils'
import { ArrowLeft, FileCode2, History, Sparkles, AlertCircle, CheckCircle2, ChevronRight } from 'lucide-react'

const TEMPLATES: { id: string; label: string; description: string; payload: Record<string, unknown> }[] = [
  {
    id: 'basic',
    label: 'Basic Question',
    description: '单轮简单问答',
    payload: { question: 'Why was I billed twice this month?' },
  },
  {
    id: 'with-meta',
    label: 'Question + Metadata',
    description: '带 user_id / channel 等上下文',
    payload: {
      question: 'How do I reset my password?',
      user_id: 'usr_001',
      channel: 'email',
      priority: 'normal',
    },
  },
  {
    id: 'multi-turn',
    label: 'Multi-turn',
    description: '历史消息 + 当前问题',
    payload: {
      messages: [
        { role: 'user', content: 'My internet is slow.' },
        { role: 'assistant', content: 'Have you tried restarting your router?' },
        { role: 'user', content: 'Yes, but it is still slow.' },
      ],
    },
  },
  {
    id: 'tool-trigger',
    label: 'Tool-required',
    description: '需要工具调用的复杂任务',
    payload: {
      task: 'search_recent_orders',
      params: { customer_id: 'cust_42', since: '2026-07-01' },
      require_tools: ['sql_query', 'http_request'],
    },
  },
]

const RECENT_KEY_PREFIX = 'hnsx.run.recent.'

export default function DomainRunPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: domain, isLoading: domainLoading, error: domainError } = useDomain(id)
  const { data: sessions } = useSessions({ domain: id, limit: 5 })
  const createSession = useCreateSession()
  const isPending = createSession.isPending

  const [trigger, setTrigger] = useState<string>('{\n  \n}')
  const [parseError, setParseError] = useState<string | null>(null)

  // localStorage-based per-domain recent trigger history
  const recents = useMemo<string[]>(() => {
    if (typeof window === 'undefined' || !id) return []
    try {
      const raw = window.localStorage.getItem(RECENT_KEY_PREFIX + id)
      return raw ? (JSON.parse(raw) as string[]) : []
    } catch {
      return []
    }
  }, [id])

  // Validate JSON on every change
  useEffect(() => {
    try {
      if (trigger.trim() === '') {
        setParseError(null)
        return
      }
      JSON.parse(trigger)
      setParseError(null)
    } catch (e) {
      setParseError((e as Error).message)
    }
  }, [trigger])

  const applyTemplate = (tpl: (typeof TEMPLATES)[number]) => {
    setTrigger(JSON.stringify(tpl.payload, null, 2))
  }

  const pushRecent = (raw: string) => {
    if (typeof window === 'undefined' || !id) return
    try {
      const next = [raw, ...recents.filter((r) => r !== raw)].slice(0, 5)
      window.localStorage.setItem(RECENT_KEY_PREFIX + id, JSON.stringify(next))
    } catch {
      // ignore quota errors
    }
  }

  const handleRun = () => {
    let parsed: Record<string, unknown>
    try {
      parsed = JSON.parse(trigger)
    } catch {
      return
    }
    createSession.mutate(
      { domain_id: id || '', trigger: parsed },
      {
        onSuccess: (res) => {
          pushRecent(trigger)
          navigate(`/sessions/${res.id}`)
        },
      },
    )
  }

  if (domainLoading) return <Loading />
  if (domainError || !domain) {
    return <ErrorState description={domainError?.message || 'Domain not found'} onRetry={() => navigate('/domains')} />
  }

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <PageHeader
        title="Run Session"
        description={`为 domain ${domain.id} @ ${domain.version} 触发一个新 session。`}
      >
        <Link to={`/domains/${domain.id}`} className={cn(buttonVariants({ variant: 'outline' }), 'no-underline')}>
          <ArrowLeft className="mr-1.5 h-4 w-4" />
          Back to Domain
        </Link>
      </PageHeader>

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Session Trigger</CardTitle>
              <CardDescription>
                JSON 格式的 trigger payload。Harness 的 orchestrator 会按 DomainSpec 解析。
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="space-y-1.5">
                <Label htmlFor="trigger" className="flex items-center gap-1.5 text-xs">
                  <FileCode2 className="h-3.5 w-3.5" />
                  Trigger Payload (JSON)
                </Label>
                <div className="overflow-hidden rounded-md border">
                  <Editor
                    height="260px"
                    defaultLanguage="json"
                    value={trigger}
                    onChange={(v) => setTrigger(v ?? '')}
                    theme="vs-light"
                    options={{
                      minimap: { enabled: false },
                      fontSize: 12,
                      lineNumbers: 'on',
                      scrollBeyondLastLine: false,
                      wordWrap: 'on',
                      tabSize: 2,
                      automaticLayout: true,
                    }}
                  />
                </div>
                {parseError ? (
                  <div className="flex items-start gap-1.5 rounded-md border border-[var(--danger)]/30 bg-[var(--danger-soft)] p-2 text-xs text-[var(--danger-text)]">
                    <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span className="font-mono">{parseError}</span>
                  </div>
                ) : trigger.trim() && (
                  <div className="flex items-center gap-1.5 text-xs text-[var(--success-text)]">
                    <CheckCircle2 className="h-3.5 w-3.5" />
                    Valid JSON
                  </div>
                )}
              </div>

              <div className="flex items-center justify-between gap-2 pt-2">
                <span className="text-xs text-muted-foreground">
                  Domain: <span className="font-mono">{domain.id}</span> @ <Badge variant="outline">{domain.version}</Badge>
                </span>
                <Button
                  onClick={handleRun}
                  disabled={isPending || !!parseError}
                  size="lg"
                >
                  {isPending ? 'Starting…' : 'Run Session'}
                </Button>
              </div>
            </CardContent>
          </Card>

          {/* Trigger history */}
          {recents.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  <History className="h-4 w-4" />
                  Recent Triggers
                </CardTitle>
                <CardDescription>本机最近 5 次（localStorage 持久化）</CardDescription>
              </CardHeader>
              <CardContent className="space-y-1">
                {recents.map((raw, i) => (
                  <button
                    key={`recent-${i}`}
                    onClick={() => setTrigger(raw)}
                    className="w-full rounded-md border bg-card px-3 py-2 text-left transition-colors hover:bg-accent"
                  >
                    <pre className="line-clamp-2 font-mono text-xs text-muted-foreground">{raw}</pre>
                  </button>
                ))}
              </CardContent>
            </Card>
          )}
        </div>

        {/* Sidebar: templates + last session */}
        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Sparkles className="h-4 w-4 text-[var(--chart-4)]" />
                Templates
              </CardTitle>
              <CardDescription>点击快速填充</CardDescription>
            </CardHeader>
            <CardContent className="space-y-1">
              {TEMPLATES.map((tpl) => (
                <button
                  key={tpl.id}
                  onClick={() => applyTemplate(tpl)}
                  className="group flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent"
                >
                  <div className="min-w-0">
                    <p className="text-sm font-medium text-[var(--chart-text-primary)]">{tpl.label}</p>
                    <p className="truncate text-xs text-muted-foreground">{tpl.description}</p>
                  </div>
                  <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                </button>
              ))}
            </CardContent>
          </Card>

          {sessions?.items && sessions.items.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Last Session</CardTitle>
                <CardDescription>本 domain 最近一次</CardDescription>
              </CardHeader>
              <CardContent>
                <Link
                  to={`/sessions/${sessions.items[0]!.id}`}
                  className="block rounded-md border bg-card p-2 transition-colors hover:bg-accent"
                >
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs">{sessions.items[0]!.id.slice(0, 14)}</span>
                    <Badge variant="outline">{sessions.items[0]!.state}</Badge>
                  </div>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {sessions.items[0]!.startedAt?.toLocaleString()}
                  </p>
                </Link>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}