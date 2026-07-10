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
  useRunEval,
} from '@/hooks/useEvals'
import { Play, Eye } from 'lucide-react'

export default function EvalsPage() {
  const navigate = useNavigate()
  const [domainFilter, setDomainFilter] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [newSetId, setNewSetId] = useState('')
  const [newDomainId, setNewDomainId] = useState('')
  const [newDescription, setNewDescription] = useState('')

  const { data: domainsData, isLoading: domainsLoading } = useDomains({ limit: 200 })
  const { data: setsData, isLoading: setsLoading, error: setsError, refetch } = useEvalSets()

  const createSet = useCreateEvalSet()
  const runEvalMutation = useRunEval

  const sets = (setsData?.items ?? []).filter((s) =>
    domainFilter ? s.domain_id === domainFilter : true,
  )

  const handleCreate = async () => {
    if (!newSetId || !newDomainId) return
    await createSet.mutateAsync({
      set_id: newSetId,
      domain_id: newDomainId,
      description: newDescription || undefined,
      cases: [],
    })
    setCreateOpen(false)
    setNewSetId('')
    setNewDomainId('')
    setNewDescription('')
  }

  const handleRun = (setId: string) => async () => {
    const res = await runEvalMutation(setId).mutateAsync({})
    navigate(`/evals/${setId}/runs/${res.run_id}`)
  }

  return (
    <div className="space-y-4">
      <PageHeader title="Evals" description="Manage evaluation sets and run reports." />

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle className="text-base">Eval Sets</CardTitle>
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            New Eval Set
          </Button>
        </CardHeader>
        <CardContent>
          <div className="mb-4 max-w-md">
            <Label htmlFor="eval-domain-filter" className="text-xs">
              Filter by domain
            </Label>
            <Select value={domainFilter} onValueChange={(v) => setDomainFilter(v || '')}>
              <SelectTrigger id="eval-domain-filter">
                <SelectValue placeholder="All domains" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="">All domains</SelectItem>
                {domainsData?.items.map((d) => (
                  <SelectItem key={d.id} value={d.id}>
                    {d.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {domainsLoading || setsLoading ? (
            <Loading />
          ) : setsError ? (
            <ErrorState description={setsError.message} onRetry={refetch} />
          ) : sets.length === 0 ? (
            <Empty description="No eval sets yet. Create one with the button above." />
          ) : (
            <div className="divide-y">
              {sets.map((set) => (
                <div
                  key={set.id}
                  className="flex items-center justify-between py-3 first:pt-0 last:pb-0"
                >
                  <div className="min-w-0 space-y-1">
                    <Link
                      to={`/evals/${set.id}`}
                      className="block truncate font-medium hover:underline"
                    >
                      {set.set_id || set.id}
                    </Link>
                    <div className="flex items-center gap-3 text-xs text-muted-foreground">
                      <span>domain: {set.domain_id}</span>
                      <span>cases: {set.case_count}</span>
                      {set.description ? (
                        <span className="truncate">/ {set.description}</span>
                      ) : null}
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <Button variant="outline" size="sm" asChild>
                      <Link to={`/evals/${set.id}`}>
                        <Eye className="mr-1.5 h-4 w-4" />
                        View
                      </Link>
                    </Button>
                    <Button
                      variant="default"
                      size="sm"
                      onClick={handleRun(set.id)}
                    >
                      <Play className="mr-1.5 h-4 w-4" />
                      Run
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Create Eval Set</DialogTitle>
            <DialogDescription>
              A new eval set bound to one domain. Cases can be added once the
              shell is created.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="set-id">Set ID</Label>
              <Input
                id="set-id"
                placeholder="my-eval-set"
                value={newSetId}
                onChange={(e) => setNewSetId(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="set-domain">Domain</Label>
              <Select value={newDomainId} onValueChange={(v) => setNewDomainId(v || '')}>
                <SelectTrigger id="set-domain">
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
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleCreate}
              disabled={!newSetId || !newDomainId || createSet.isPending}
            >
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
