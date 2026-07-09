import { useState } from 'react'
import Editor from '@monaco-editor/react'
import { Button } from '@/components/ui/button'
import { Save, Check, Play } from 'lucide-react'

interface DomainEditorProps {
  value: string
  onChange: (value: string) => void
  onSave?: () => void
  onValidate?: () => void
  onRun?: () => void
}

export function DomainEditor({ value, onChange, onSave, onValidate, onRun }: DomainEditorProps) {
  return (
    <div className="space-y-4">
      <div className="rounded-md border overflow-hidden">
        <Editor
          height="60vh"
          defaultLanguage="yaml"
          value={value}
          onChange={(v) => onChange(v || '')}
          theme="vs-light"
          options={{ minimap: { enabled: false }, scrollBeyondLastLine: false }}
        />
      </div>
      <div className="flex gap-2">
        <Button onClick={onSave}>
          <Save className="mr-2 h-4 w-4" /> Save
        </Button>
        <Button variant="outline" onClick={onValidate}>
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
