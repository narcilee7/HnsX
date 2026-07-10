import { useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { Empty } from '@/components/ui/Empty'
import {
  useEvalSet,
  useEvalRuns,
  useRunEval,
  useUpdateEvalSet,
  useDeleteEvalSet,
} from '@/hooks/useEvals'
import { Play, ArrowLeft, Pencil, Trash2 } from 'lucide-react'
import { formatDate } from '@/api/mappers'

export default function EvalSetPage() {
  const { setId } = useParams<{ setId: string }>()
  const navigate = useNavigate()
  const {
    data: evalSet,
    isLoading: setLoading,
    error: setError,
    refetch: refetchSet,
  } = useEvalSet(setId)
  const { data: runsData, isLoading: runsLoading } = useEvalRuns(setId)
  const runEval = useRunEval(setId || '')
  const updateEvalSet = useUpdateEvalSet(setId || '')
  const deleteEvalSet = useDeleteEvalSet()

  const [editOpen, setEditOpen] = useState(false)
  const [editDescription, setEditDescription] = useState('')

  const handleRun = async () => {
    const res = await runEval.mutateAsync({})
    navigate(`/evals/${setId}/runs/${res.run_id}`)
  }

  const openEdit = () => {
    setEditDescription(evalSet?.description || '')
    setEditOpen(true)
  }

  const handleUpdate = async () => {
    if (!evalSet) return
    await updateEvalSet.mutateAsync({
      description: editDescription,
      cases: evalSet.cases.map((c) => ({
        id: c.id,
        name: c.name,
        input: c.input,
        expect: c.expect,
      })),
    })
    setEditOpen(false)
  }

  const handleDelete = async () => {
    if (!setId) return
    if (!window.confirm(`Delete eval set "${setId}"? This cannot be undone.`)) return
    await deleteEvalSet.mutateAsync(setId)
  }

  if (setLoading) return <Loading />
  if (setError || !evalSet) {
    return (
      <ErrorState
        description={setError?.message || 'Eval set not found'}
        onRetry={refetchSet}
      />
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={`Eval Set: ${evalSet.id}`}
        description={evalSet.description || 'Cases defined by this set.'}
        breadcrumbs={[
          { label: 'Evals', href: '/evals' },
          { label: evalSet.id },
        ]}
      >
        <div className="flex items-center gap-2">
          <Button variant="default" size="sm" onClick={handleRun} disabled={runEval.isPending}>
            <Play className="mr-1.5 h-4 w-4" />
            Run Eval
          </Button>
          <Button variant="outline" size="sm" onClick={openEdit}>
            <Pencil className="mr-1.5 h-4 w-4" />
            Edit
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleDelete}
            disabled={deleteEvalSet.isPending}
            className="text-destructive hover:bg-destructive/10"
          >
            <Trash2 className="mr-1.5 h-4 w-4" />
            Delete
          </Button>
          <Button variant="outline" size="sm" asChild>
            <Link to="/evals" className="no-underline">
              <ArrowLeft className="mr-1.5 h-4 w-4" />
              Back
            </Link>
          </Button>
        </div>
      </PageHeader>

      <div className="grid gap-4 lg:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Domain
            </CardTitle>
          </CardHeader>
          <CardContent>
            {evalSet.domainId ? (
              <Link to={`/domains/${evalSet.domainId}`} className="font-medium hover:underline">
                {evalSet.domainId}
              </Link>
            ) : (
              <span className="text-muted-foreground">—</span>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Cases
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-lg font-semibold">{evalSet.cases.length}</div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Cases</CardTitle>
        </CardHeader>
        <CardContent>
          {evalSet.cases.length === 0 ? (
            <Empty description="No cases in this eval set." />
          ) : (
            <div className="divide-y">
              {evalSet.cases.map((c) => (
                <div key={c.id} className="space-y-2 py-4 first:pt-0 last:pb-0">
                  <div className="flex items-center justify-between">
                    <span className="font-medium">{c.name || c.id}</span>
                    <Badge variant="outline">{c.id}</Badge>
                  </div>
                  <div className="grid gap-4 md:grid-cols-2">
                    <div className="rounded-md bg-muted p-3">
                      <div className="mb-1 text-xs font-medium text-muted-foreground">Input</div>
                      <pre className="whitespace-pre-wrap text-xs">{c.input}</pre>
                    </div>
                    <div className="rounded-md bg-muted p-3">
                      <div className="mb-1 text-xs font-medium text-muted-foreground">Expected</div>
                      <pre className="whitespace-pre-wrap text-xs">{c.expect}</pre>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Runs</CardTitle>
        </CardHeader>
        <CardContent>
          {runsLoading ? (
            <Loading />
          ) : !runsData || runsData.items.length === 0 ? (
            <Empty description="No runs yet. Click Run Eval to start one." />
          ) : (
            <div className="divide-y">
              {runsData.items.map((run) => (
                <div
                  key={run.id}
                  className="flex items-center justify-between py-3 first:pt-0 last:pb-0"
                >
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <Link
                        to={`/evals/${setId}/runs/${run.id}`}
                        className="font-medium hover:underline"
                      >
                        {run.id}
                      </Link>
                      <Badge variant={run.state === 'completed' ? 'default' : 'secondary'}>
                        {run.state}
                      </Badge>
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {run.total_cases} cases · score {run.score.toFixed(2)} ·{' '}
                      {formatDate(run.created_at ? new Date(run.created_at) : null)}
                    </div>
                  </div>
                  <Button variant="outline" size="sm" asChild>
                    <Link to={`/evals/${setId}/runs/${run.id}`}>View</Link>
                  </Button>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Eval Set</DialogTitle>
            <DialogDescription>Update the eval set description.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2">
              <label className="text-sm font-medium">Description</label>
              <textarea
                className="flex min-h-[80px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                value={editDescription}
                onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => setEditDescription(e.target.value)}
                rows={3}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleUpdate} disabled={updateEvalSet.isPending}>
              Save
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
