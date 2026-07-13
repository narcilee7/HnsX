import { Suspense, lazy, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Save, Check, Play } from 'lucide-react'

// Monaco Editor 体积大，仅在进入编辑页时按需加载
const Editor = lazy(() => import('@monaco-editor/react'))

interface DomainEditorProps {
  value: string
  onChange: (value: string) => void
  onSave?: () => void
  onValidate?: () => void
  onRun?: () => void
  isSaving?: boolean
  isValidating?: boolean
}

function EditorSkeleton() {
  return (
    <div className="rounded-md border">
      <Skeleton className="h-[60vh] w-full rounded-none" />
    </div>
  )
}

export function DomainEditor({ value, onChange, onSave, onValidate, onRun, isSaving, isValidating }: DomainEditorProps) {
  return (
    <div className="space-y-4">
      <div className="rounded-md border overflow-hidden">
        <Suspense fallback={<EditorSkeleton />}>
          <Editor
            height="60vh"
            defaultLanguage="yaml"
            value={value}
            onChange={(v) => onChange(v || '')}
            theme="vs-light"
            options={{ minimap: { enabled: false }, scrollBeyondLastLine: false }}
          />
        </Suspense>
      </div>
      <div className="flex gap-2">
        <Button onClick={onSave} disabled={isSaving}>
          <Save className="mr-2 h-4 w-4" /> Save
        </Button>
        <Button variant="outline" onClick={onValidate} disabled={isValidating}>
          <Check className="mr-2 h-4 w-4" /> Validate
        </Button>
        <Button variant="outline" onClick={onRun}>
          <Play className="mr-2 h-4 w-4" /> Run Session
        </Button>
      </div>
    </div>
  )
}

export function useDomainEditor(initial: string) {
  const [value, setValue] = useState(initial)
  return { value, setValue }
}
