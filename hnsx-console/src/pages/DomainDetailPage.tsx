import { useEffect, useMemo } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '@/components/ui/PageHeader'
import { DomainEditor, useDomainEditor } from '@/components/domain/DomainEditor'
import { VersionsPanel } from '@/components/domain/VersionsPanel'
import {
  useDomainYaml,
  useUpdateDomainYaml,
  useValidateDomainYaml,
} from '@/hooks/useDomains'
import { ErrorState } from '@/components/ui/Error'
import { Loading } from '@/components/ui/Loading'
import { Play, ArrowLeft } from 'lucide-react'
import { load, dump } from 'js-yaml'
import { toast } from 'sonner'

interface DomainYamlPreview {
  id: string
  version: string
  description: string
  harness?: unknown
}

function bumpPatchVersion(version: string): string {
  // semver-ish: 1.2.3 -> 1.2.4; 1.2 -> 1.3; 1 -> 2
  const parts = version.split('.')
  const last = parts.length - 1
  const numeric = Number(parts[last])
  if (!Number.isNaN(numeric)) {
    parts[last] = String(numeric + 1)
    return parts.join('.')
  }
  return `${version}.1`
}

export default function DomainDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { data: yamlText, isLoading, error, refetch } = useDomainYaml(id)
  const { value, setValue } = useDomainEditor('')
  const update = useUpdateDomainYaml(id || '')
  const validate = useValidateDomainYaml(id || '')

  useEffect(() => {
    if (yamlText) {
      setValue(yamlText)
    }
  }, [yamlText, setValue])

  const parsed = useMemo<DomainYamlPreview | null>(() => {
    if (!yamlText) return null
    try {
      const obj = load(yamlText) as Record<string, unknown>
      return {
        id: String(obj.id),
        version: String(obj.version),
        description: String(obj.description || ''),
        harness: obj.harness,
      }
    } catch {
      return null
    }
  }, [yamlText])

  const handleSave = () => {
    let yamlToSave = value
    try {
      const obj = load(value) as Record<string, unknown>
      const currentVersion = parsed?.version
      const editedVersion = String(obj.version)
      if (currentVersion && editedVersion === currentVersion) {
        const nextVersion = bumpPatchVersion(currentVersion)
        obj.version = nextVersion
        yamlToSave = dump(obj, { lineWidth: -1 })
      }
    } catch (e) {
      toast.error(`Invalid YAML: ${(e as Error).message}`)
      return
    }

    update.mutate(yamlToSave, {
      onSuccess: (res) => {
        refetch()
        toast.success(`Domain saved @ ${res.version}`)
      },
    })
  }

  const handleValidate = () => {
    validate.mutate(value, {
      onSuccess: (result) => {
        if (result.valid) {
          toast.success('Domain spec is valid')
        } else {
          toast.error('Validation failed')
        }
      },
    })
  }

  const handleRun = () => {
    navigate(`/domains/${id}/run`)
  }

  if (isLoading) return <Loading />
  if (error || !parsed) {
    return <ErrorState description={error?.message || 'Domain not found'} onRetry={refetch} />
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title={parsed.id}
        description={parsed.description}
        breadcrumbs={[
          { label: 'Domains', href: '/domains' },
          { label: parsed.id },
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
          <DomainEditor
            value={value}
            onChange={setValue}
            onSave={handleSave}
            onValidate={handleValidate}
            onRun={handleRun}
            isSaving={update.isPending}
            isValidating={validate.isPending}
          />
        </TabsContent>
        <TabsContent value="info">
          <div className="rounded-md border bg-muted/30 p-4">
            <h3 className="mb-2 text-sm font-medium">Harness (raw)</h3>
            <pre className="overflow-auto text-xs">
              {JSON.stringify(parsed.harness, null, 2)}
            </pre>
          </div>
        </TabsContent>
        <TabsContent value="versions">
          <VersionsPanel
            domainId={parsed.id}
            currentVersion={parsed.version}
            onRollback={refetch}
          />
        </TabsContent>
      </Tabs>
    </div>
  )
}
