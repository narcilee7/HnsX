import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
import { useDomains } from '@/hooks/useDomains'
import {
  useEvalSets,
  useCreateEvalSet,
  useDeleteEvalSet,
  useRunEval,
} from '@/hooks/useEvals'
import { Play, Trash2, Plus, Eye } from 'lucide-react'

export default function EvalsPage() {
  const navigate = useNavigate()
  const [domainId, setDomainId] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [newSetId, setNewSetId] = useState('')
  const [newDescription, setNewDescription] = useState('')

  const { data: domainsData, isLoading: domainsLoading } = useDomains({ limit: 200 })
  const {
    data: sets,
    isLoading: setsLoading,
    error: setsError,
    refetch,
  } = useEvalSets(domainId || undefined)

  const createSet = useCreateEvalSet(domainId || undefined)
  const deleteSet = useDeleteEvalSet(domainId || undefined)
  const runEval = useRunEval(domainId || undefined, undefined)

  const handleCreate = async () => {
    await createSet.mutateAsync({
      id: newSetId,
      description: newDescription,
      cases: [],
    })
    setCreateOpen(false)
    setNewSetId('')
    setNewDescription('')
  }

  const handleRun = async (setId: string) => {
    const res = await runEval.mutateAsync({})
    navigate(`/domains/${domainId}/evals/${setId}/runs/${res.eval_run_id}`)
  }

  if (domainsLoading) return <Loading />

  return (
    <div className="space-y-4">
      <PageHeader title="Evals" description="Manage evaluation sets and run reports.">
        <Button onClick={() => setCreateOpen(true)} disabled={!domainId}>
          <Plus className="mr-1.5 h-4 w-4" />
          New Eval Set
        </Button>
        <Dialog open={createOpen} onOpenChange={setCreateOpen}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>Create Eval Set</DialogTitle>
              <DialogDescription>Create a new eval set in {domainId}.</DialogDescription>
            </DialogHeader>
            <div className="space-y-4 py-2">
              <div className="space-y-1.5">
                <Label htmlFor="set-id">ID</Label>
                <Input
                  id="set-id"
                  placeholder="my-eval-set"
                  value={newSetId}
                  onChange={(e) => setNewSetId(e.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="set-description">Description</Label>
                <Input
                  id="set-description"
                  placeholder="What this eval set measures"
                  value={newDescription}
                  onChange={(e) => setNewDescription(e.target.value)}
                />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
              <Button onClick={handleCreate} disabled={!newSetId || createSet.isPending}>
                Create
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </PageHeader>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-base">Domain</CardTitle>
        </CardHeader>
        <CardContent>
          <Select value={domainId} onValueChange={(v) => setDomainId(v || '')}>
            <SelectTrigger className="w-full md:w-80">
              <SelectValue placeholder="Select a domain" />
            </SelectTrigger>
            <SelectContent>
              {domainsData?.items.map((d) => (
                <SelectItem key={d.id} value={d.id}>
                  {d.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CardContent>
      </Card>

      {domainId ? (
        setsError ? (
          <ErrorState description={setsError.message} onRetry={refetch} />
        ) : (
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Eval Sets</CardTitle>
            </CardHeader>
            <CardContent>
              {setsLoading ? (
                <div className="space-y-2">
                  {Array.from({ length: 4 }).map((_, i) => (
                    <div key={i} className="h-12 animate-pulse rounded bg-muted" />
                  ))}
                </div>
              ) : !sets || sets.length === 0 ? (
                <Empty description="No eval sets found for this domain." />
              ) : (
                <div className="divide-y">
                  {sets.map((set) => (
                    <div
                      key={set.id}
                      className="flex items-center justify-between py-4 first:pt-0 last:pb-0"
                    >
                      <div className="min-w-0 space-y-1">
                        <Link
                          to={`/domains/${domainId}/evals/${set.id}`}
                          className="block truncate font-medium hover:underline"
                        >
                          {set.id}
                        </Link>
                        {set.description && (
                          <p className="truncate text-sm text-muted-foreground">
                            {set.description}
                          </p>
                        )}
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          asChild
                        >
                          <Link to={`/domains/${domainId}/evals/${set.id}`}>
                            <Eye className="mr-1.5 h-4 w-4" />
                            View
                          </Link>
                        </Button>
                        <Button
                          variant="default"
                          size="sm"
                          onClick={() => handleRun(set.id)}
                          disabled={runEval.isPending}
                        >
                          <Play className="mr-1.5 h-4 w-4" />
                          Run
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={() => {
                            if (confirm(`Delete eval set "${set.id}"?`)) {
                              deleteSet.mutate(set.id)
                            }
                          }}
                          disabled={deleteSet.isPending}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        )
      ) : null}
    </div>
  )
}
