import { useEffect, useMemo, useState } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useForm, FormProvider } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { PageHeader } from '@/components/ui/PageHeader'
import { Loading } from '@/components/ui/Loading'
import { ErrorState } from '@/components/ui/Error'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { DomainNav } from '@/components/domain/DomainNav'
import {
  JsonSchemaForm,
  buildZodSchema,
  buildDefaultValues,
} from '@/components/domain/JsonSchemaForm'
import {
  TriggerHistory,
  pushTriggerHistory,
} from '@/components/domain/TriggerHistory'
import { useDomainYaml, useDomainSchema } from '@/hooks/useDomains'
import {
  useCreateSession,
  useSessions,
  useSession,
  useSessionEvents,
} from '@/hooks/useSessions'
import { cn } from '@/lib/utils'
import { toast } from 'sonner'
import {
  Send,
  Sparkles,
  FileText,
  ArrowRight,
  Clock,
  Activity,
} from 'lucide-react'
import { load } from 'js-yaml'

const TEMPLATES: { id: string; label: string; description: string; payload: Record<string, unknown> }[] = [
  {
    id: 'basic',
    label: 'Basic Question',
    description: '单轮简单问答',
    payload: { question: 'Why was I billed twice this month?' },
  },
  {
    id: 'refund',
    label: 'Refund Request',
    description: '触发 billing + 审批路径',
    payload: {
      question: 'I want a refund for my last charge.',
      user_id: 'usr_001',
      channel: 'email',
    },
  },
  {
    id: 'technical',
    label: 'Technical Issue',
    description: '路由到 technical specialist',
    payload: {
      question: 'My internet is very slow today.',
      user_id: 'usr_002',
      channel: 'chat',
    },
  },
  {
    id: 'multi-turn',
    label: 'Multi-turn',
    description: '带历史上下文',
    payload: {
      question: 'Yes, but it is still slow.',
      user_id: 'usr_003',
      channel: 'phone',
    },
  },
]

interface DomainInfo {
  id: string
  version: string
  description: string
}

export default function DomainWorkspacePage() {
  const { id } = useParams<{ id: string }>()
  const {
    data: yamlText,
    isLoading: yamlLoading,
    error: yamlError,
  } = useDomainYaml(id)
  const { data: schema, isLoading: schemaLoading } = useDomainSchema(id)
  const createSession = useCreateSession()
  const { data: recentSessions } = useSessions({ domain: id, limit: 5 })
  const [activeSessionId, setActiveSessionId] = useState<string | null>(null)
  const { data: activeSession } = useSession(activeSessionId ?? undefined)
  const { observations, state: liveState, connected } = useSessionEvents(
    activeSessionId ?? undefined,
  )

  const domain = useMemo<DomainInfo | null>(() => {
    if (!yamlText) return null
    try {
      const obj = load(yamlText) as Record<string, unknown>
      return {
        id: String(obj.id),
        version: String(obj.version),
        description: String(obj.description || ''),
      }
    } catch {
      return null
    }
  }, [yamlText])

  const hasSchema =
    schema?.trigger_schema && typeof schema.trigger_schema === 'object'
  const zodSchema = useMemo(
    () => buildZodSchema(hasSchema ? schema?.trigger_schema : { type: 'object', properties: {} }),
    [schema, hasSchema],
  )
  const defaultValues = useMemo(
    () => buildDefaultValues(hasSchema ? schema?.trigger_schema : { type: 'object', properties: {} }),
    [schema, hasSchema],
  )

  const form = useForm<Record<string, unknown>>({
    resolver: zodResolver(zodSchema),
    defaultValues,
    mode: 'onChange',
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const [rawJson, setRawJson] = useState('{}')
  const [rawError, setRawError] = useState<string | null>(null)

  const handleSubmitForm = (values: Record<string, unknown>) => {
    createSession.mutate(
      { domain_id: id || '', trigger: values },
      {
        onSuccess: (res) => {
          setActiveSessionId(res.id)
          pushTriggerHistory(id, values)
          toast.success(`Session ${res.id.slice(0, 8)} started`)
        },
      },
    )
  }

  const handleRawSubmit = () => {
    let parsed: Record<string, unknown>
    try {
      parsed = JSON.parse(rawJson)
      setRawError(null)
    } catch (e) {
      setRawError((e as Error).message)
      return
    }
    handleSubmitForm(parsed)
  }

  const applyTemplate = (payload: Record<string, unknown>) => {
    if (hasSchema) {
      form.reset(payload)
    } else {
      setRawJson(JSON.stringify(payload, null, 2))
    }
  }

  const handleHistorySelect = (payload: Record<string, unknown>) => {
    if (hasSchema) {
      form.reset(payload)
    } else {
      setRawJson(JSON.stringify(payload, null, 2))
    }
  }

  const durationMs = useMemo(() => {
    if (!activeSession?.startedAt || !activeSession?.completedAt) return undefined
    return activeSession.completedAt.getTime() - activeSession.startedAt.getTime()
  }, [activeSession])

  if (yamlLoading || schemaLoading) return <Loading />
  if (yamlError || !domain) {
    return (
      <ErrorState
        description={yamlError?.message || 'Domain not found'}
        onRetry={() => window.location.reload()}
      />
    )
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title={domain.id}
        description={domain.description}
        breadcrumbs={[
          { label: 'Domains', href: '/domains' },
          { label: domain.id },
        ]}
      >
        <Link
          to={`/domains/${id}/run`}
          className={cn(buttonVariants({ variant: 'outline' }), 'no-underline')}
        >
          Advanced Run
        </Link>
      </PageHeader>

      <DomainNav domainId={domain.id} />

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Sparkles className="h-4 w-4 text-[var(--chart-4)]" />
                Trigger
              </CardTitle>
              <CardDescription>
                {hasSchema
                  ? '根据 Domain 定义的 trigger_schema 自动生成的输入表单。'
                  : '当前 Domain 未声明 trigger_schema，使用原始 JSON 触发。'}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="flex flex-wrap gap-2">
                {TEMPLATES.map((tpl) => (
                  <Button
                    key={tpl.id}
                    variant="outline"
                    size="sm"
                    onClick={() => applyTemplate(tpl.payload)}
                  >
                    {tpl.label}
                  </Button>
                ))}
              </div>

              {hasSchema ? (
                <FormProvider {...form}>
                  <form
                    onSubmit={form.handleSubmit(handleSubmitForm)}
                    className="space-y-4"
                  >
                    <JsonSchemaForm schema={schema?.trigger_schema} />
                    <Button
                      type="submit"
                      disabled={createSession.isPending}
                      className="w-full sm:w-auto"
                    >
                      <Send className="mr-2 h-4 w-4" />
                      {createSession.isPending ? 'Starting…' : 'Run Session'}
                    </Button>
                  </form>
                </FormProvider>
              ) : (
                <div className="space-y-3">
                  <div className="space-y-1.5">
                    <Label htmlFor="raw-trigger">Trigger Payload (JSON)</Label>
                    <Textarea
                      id="raw-trigger"
                      value={rawJson}
                      onChange={(e) => setRawJson(e.target.value)}
                      className="min-h-[160px] font-mono text-xs"
                    />
                    {rawError && (
                      <p className="text-xs text-[var(--danger-text)]">{rawError}</p>
                    )}
                  </div>
                  <Button
                    onClick={handleRawSubmit}
                    disabled={createSession.isPending}
                  >
                    <Send className="mr-2 h-4 w-4" />
                    {createSession.isPending ? 'Starting…' : 'Run Session'}
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          {activeSessionId && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  <Activity className="h-4 w-4 text-[var(--chart-4)]" />
                  Result
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <div className="flex flex-wrap items-center gap-3">
                  <Badge variant="outline" className="font-mono">
                    {activeSessionId.slice(0, 14)}
                  </Badge>
                  <StatusBadge status={liveState || activeSession?.state || ''} />
                  {connected && (
                    <span className="inline-flex items-center gap-1 text-xs text-[var(--success-text)]">
                      <span className="relative flex h-2 w-2">
                        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--success)] opacity-75" />
                        <span className="relative inline-flex h-2 w-2 rounded-full bg-[var(--success)]" />
                      </span>
                      live
                    </span>
                  )}
                  {durationMs !== undefined && (
                    <span className="flex items-center gap-1 text-xs text-muted-foreground">
                      <Clock className="h-3 w-3" />
                      {durationMs >= 1000
                        ? `${(durationMs / 1000).toFixed(2)}s`
                        : `${durationMs}ms`}
                    </span>
                  )}
                </div>

                <div className="rounded-md border bg-muted/30 p-3">
                  <p className="text-sm font-medium">Final Output</p>
                  <pre className="mt-1 whitespace-pre-wrap text-xs text-muted-foreground">
                    {activeSession?.result || 'Waiting for output…'}
                  </pre>
                </div>

                <div className="grid gap-2 text-xs text-muted-foreground sm:grid-cols-3">
                  <div className="rounded-md border p-2">
                    <span className="block font-medium">Observations</span>
                    {observations.length}
                  </div>
                  <div className="rounded-md border p-2">
                    <span className="block font-medium">State</span>
                    {liveState || activeSession?.state || '—'}
                  </div>
                  <div className="rounded-md border p-2">
                    <span className="block font-medium">Version</span>
                    {activeSession?.domainVersion || '—'}
                  </div>
                </div>

                <div className="flex gap-2">
                  <Link
                    to={`/sessions/${activeSessionId}`}
                    className={cn(
                      buttonVariants({ variant: 'outline', size: 'sm' }),
                      'no-underline',
                    )}
                  >
                    Session Detail
                    <ArrowRight className="ml-1 h-3 w-3" />
                  </Link>
                  {activeSession?.traceId && (
                    <Link
                      to={`/traces/${activeSession.traceId}`}
                      className={cn(
                        buttonVariants({ variant: 'outline', size: 'sm' }),
                        'no-underline',
                      )}
                    >
                      View Trace
                    </Link>
                  )}
                </div>
              </CardContent>
            </Card>
          )}
        </div>

        <div className="space-y-4">
          <TriggerHistory domainId={id} onSelect={handleHistorySelect} />

          {recentSessions?.items && recentSessions.items.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  <FileText className="h-4 w-4" />
                  Recent Runs
                </CardTitle>
                <CardDescription>本 domain 最近 5 次 sessions</CardDescription>
              </CardHeader>
              <CardContent className="space-y-2">
                {recentSessions.items.map((sess) => (
                  <Link
                    key={sess.id}
                    to={`/sessions/${sess.id}`}
                    className="block rounded-md border bg-card p-2 transition-colors hover:bg-accent"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-xs">{sess.id.slice(0, 14)}</span>
                      <StatusBadge status={sess.state} />
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {sess.startedAt?.toLocaleString()}
                    </p>
                  </Link>
                ))}
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
