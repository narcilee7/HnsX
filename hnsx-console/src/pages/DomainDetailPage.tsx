import { useParams } from 'react-router-dom'
import Editor from '@monaco-editor/react'

const defaultYaml = `id: customer-service
version: 0.1.0
description: |
  Routes customer questions to the right specialist agent.
harness:
  agents:
    - id: triage
      model:
        provider: anthropic
        model: claude-haiku-4-5
      adapter:
        kind: noop
  session:
    mode: workflow
`

export default function DomainDetailPage() {
  const { id } = useParams<{ id: string }>()

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-bold">Domain: {id}</h1>
      <div className="rounded-lg border bg-white p-4 shadow-sm">
        <Editor
          height="60vh"
          defaultLanguage="yaml"
          defaultValue={defaultYaml}
          theme="vs-light"
          options={{ minimap: { enabled: false } }}
        />
      </div>
      <div className="flex gap-3">
        <button className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700">
          Save
        </button>
        <button className="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium hover:bg-gray-50">
          Validate
        </button>
        <button className="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium hover:bg-gray-50">
          Run Session
        </button>
      </div>
    </div>
  )
}
