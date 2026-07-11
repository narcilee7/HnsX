import { useMemo, useState } from 'react'
import { LayoutTemplate, Terminal } from 'lucide-react'
import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Empty } from '@/components/ui/Empty'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { useTemplates } from '@/hooks/useTemplates'
import type { Template } from '@/api/templates'

function TemplateCard({ template }: { template: Template }) {
  const variables = template.variables ?? []
  const requirements = template.requirements ?? {}

  return (
    <Card hoverable className="h-full">
      <CardHeader>
        <div className="flex items-start justify-between gap-2">
          <CardTitle className="flex items-center gap-2">
            <LayoutTemplate className="h-4 w-4 text-primary" />
            {template.name}
          </CardTitle>
        </div>
        <CardDescription className="line-clamp-3">{template.description}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap gap-1">
          {template.tags.map((tag) => (
            <Badge key={tag} variant="secondary">
              {tag}
            </Badge>
          ))}
        </div>
        {variables.length > 0 && (
          <div className="text-xs text-muted-foreground">
            Variables: {variables.map((v) => v.name).join(', ')}
          </div>
        )}
        {(requirements.providers?.length ?? 0) > 0 && (
          <div className="text-xs text-muted-foreground">
            Providers: {requirements.providers?.join(', ')}
          </div>
        )}
      </CardContent>
      <CardFooter className="flex flex-col items-start gap-2 border-t bg-muted/50">
        <div className="flex w-full items-center justify-between gap-2">
          <code className="rounded bg-muted px-2 py-1 text-xs">hnsx try {template.id}</code>
          <Button variant="outline" size="sm" className="gap-1" asChild>
            <a href={`/api/v1/domains/${template.id}/yaml`} target="_blank" rel="noreferrer">
              <Terminal className="h-3.5 w-3.5" />
              View YAML
            </a>
          </Button>
        </div>
      </CardFooter>
    </Card>
  )
}

export default function GalleryPage() {
  const { data, isLoading, error, refetch } = useTemplates()
  const [filter, setFilter] = useState('')

  const filtered = useMemo(() => {
    const term = filter.trim().toLowerCase()
    if (!term) return data?.items ?? []
    return (data?.items ?? []).filter((t) =>
      t.name.toLowerCase().includes(term) ||
      t.description.toLowerCase().includes(term) ||
      t.tags.some((tag) => tag.toLowerCase().includes(term)),
    )
  }, [data, filter])

  const allTags = useMemo(() => {
    const set = new Set<string>()
    data?.items.forEach((t) => t.tags.forEach((tag) => set.add(tag)))
    return Array.from(set).sort()
  }, [data])

  if (error) {
    return <ErrorState description={error.message} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title="Template Gallery"
        description="Discover starter domains and copy the CLI command to run them."
      >
        <Input
          placeholder="Filter templates..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="w-64"
        />
      </PageHeader>

      {allTags.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {allTags.map((tag) => (
            <Badge
              key={tag}
              variant={filter.toLowerCase() === tag.toLowerCase() ? 'default' : 'outline'}
              className="cursor-pointer"
              onClick={() => setFilter(filter.toLowerCase() === tag.toLowerCase() ? '' : tag)}
            >
              {tag}
            </Badge>
          ))}
        </div>
      )}

      {isLoading ? (
        <Loading />
      ) : filtered.length === 0 ? (
        <Empty
          title="No templates found"
          description={filter ? 'Try a different filter.' : 'The template index is empty.'}
        />
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filtered.map((template) => (
            <TemplateCard key={template.id} template={template} />
          ))}
        </div>
      )}
    </div>
  )
}
