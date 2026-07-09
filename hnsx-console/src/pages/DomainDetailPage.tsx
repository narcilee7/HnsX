import { useParams, Link } from 'react-router-dom'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/ui/PageHeader'
import { DomainEditor, useDomainEditor } from '@/components/domain/DomainEditor'
import { HarnessVisualizer } from '@/components/domain/HarnessVisualizer'
import { VersionsPanel } from '@/components/domain/VersionsPanel'
import { useDomain } from '@/hooks/useDomains'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { Play, ArrowLeft } from 'lucide-react'

export default function DomainDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: domain, isLoading, error, refetch } = useDomain(id)
  const { value, setValue } = useDomainEditor('')

  if (isLoading) return <Loading />
  if (error || !domain) {
    return <ErrorState description={error?.message || 'Domain not found'} onRetry={refetch} />
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
          to="/domains"
          className={cn(buttonVariants({ variant: 'outline' }), 'no-underline')}
        >
          <ArrowLeft className="mr-2 h-4 w-4" /> Back
        </Link>
        <Link
          to={`/domains/${id}/run`}
          className={cn(buttonVariants({ variant: 'default' }), 'no-underline')}
        >
          <Play className="mr-2 h-4 w-4" /> Run
        </Link>
      </PageHeader>

      <Tabs defaultValue="editor" className="w-full">
        <TabsList>
          <TabsTrigger value="editor">Editor</TabsTrigger>
          <TabsTrigger value="info">Info</TabsTrigger>
          <TabsTrigger value="versions">Versions</TabsTrigger>
        </TabsList>
        <TabsContent value="editor" className="space-y-4">
          <DomainEditor value={value} onChange={setValue} />
        </TabsContent>
        <TabsContent value="info">
          <HarnessVisualizer harness={domain.harness} />
        </TabsContent>
        <TabsContent value="versions">
          <VersionsPanel
            domainId={domain.id}
            currentVersion={domain.version}
            onRollback={refetch}
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}
