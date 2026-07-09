import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { PageHeader } from '@/components/ui/PageHeader'
import { useCreateSession } from '@/hooks/useSessions'

export default function DomainRunPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [trigger, setTrigger] = useState('{}')
  const { mutate: createSession, isPending } = useCreateSession()

  const handleRun = () => {
    let parsed: Record<string, unknown> = {}
    try {
      parsed = JSON.parse(trigger)
    } catch {
      // ignore
    }
    createSession(
      { domain_id: id || '', trigger: parsed },
      {
        onSuccess: (res) => {
          navigate(`/sessions/${res.id}`)
        },
      },
    )
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <PageHeader title="Run Session" description={`Trigger a new session for domain ${id}.`} />
      <Card>
        <CardHeader>
          <CardTitle>Session Trigger</CardTitle>
          <CardDescription>Provide a JSON payload to start the session.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="domain">Domain</Label>
            <Input id="domain" value={id} disabled />
          </div>
          <div className="space-y-2">
            <Label htmlFor="trigger">Trigger (JSON)</Label>
            <textarea
              id="trigger"
              value={trigger}
              onChange={(e) => setTrigger(e.target.value)}
              className="min-h-[120px] w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm"
            />
          </div>
          <Button onClick={handleRun} disabled={isPending}>
            {isPending ? 'Starting...' : 'Run Session'}
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
