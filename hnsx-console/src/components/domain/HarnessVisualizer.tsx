import { useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toJson, SandboxSchema, PolicySchema, MemorySchema, SessionSchema } from '@hnsx/sdk-node'
import type { Harness } from '@hnsx/sdk-node'
import { HarnessGraph } from './HarnessGraph'
import { Network, ListTree } from 'lucide-react'

interface HarnessVisualizerProps {
  harness?: Harness
}

export function HarnessVisualizer({ harness }: HarnessVisualizerProps) {
  const [view, setView] = useState<'graph' | 'list'>('graph')

  if (!harness) {
    return <p className="text-muted-foreground">No harness configuration available.</p>
  }

  const sections = [
    { title: 'Agents', count: harness.agents.length, items: harness.agents.map((a) => a.id) },
    { title: 'Prompts', count: harness.prompts.length, items: harness.prompts.map((p) => p.id) },
    { title: 'Skills', count: harness.skills.length, items: harness.skills.map((s) => s.id) },
    { title: 'Tools', count: harness.tools.length, items: harness.tools.map((t) => t.id) },
    { title: 'MCPs', count: harness.mcps.length, items: harness.mcps.map((m) => m.id) },
  ]

  return (
    <div className="space-y-4">
      <Tabs value={view} onValueChange={(v) => setView(v as 'graph' | 'list')}>
        <TabsList>
          <TabsTrigger value="graph">
            <Network className="mr-1.5 h-4 w-4" />
            Graph
          </TabsTrigger>
          <TabsTrigger value="list">
            <ListTree className="mr-1.5 h-4 w-4" />
            List
          </TabsTrigger>
        </TabsList>

        <TabsContent value="graph" className="mt-4">
          <HarnessGraph harness={harness} />
        </TabsContent>

        <TabsContent value="list" className="mt-4">
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
            {sections.map((section) => (
              <Card key={section.title}>
                <CardHeader>
                  <CardTitle>{section.title}</CardTitle>
                  <CardDescription>{section.count} configured</CardDescription>
                </CardHeader>
                <CardContent>
                  {section.items.length === 0 ? (
                    <p className="text-sm text-muted-foreground">None</p>
                  ) : (
                    <ul className="space-y-1 text-sm">
                      {section.items.map((item) => (
                        <li key={item} className="rounded-md bg-muted px-2 py-1">{item}</li>
                      ))}
                    </ul>
                  )}
                </CardContent>
              </Card>
            ))}
            <Card>
              <CardHeader>
                <CardTitle>Sandbox</CardTitle>
              </CardHeader>
              <CardContent>
                <pre className="text-sm">{JSON.stringify(harness.sandbox ? toJson(SandboxSchema, harness.sandbox) : null, null, 2)}</pre>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Policy</CardTitle>
              </CardHeader>
              <CardContent>
                <pre className="text-sm">{JSON.stringify(harness.policy ? toJson(PolicySchema, harness.policy) : null, null, 2)}</pre>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Memory</CardTitle>
              </CardHeader>
              <CardContent>
                <pre className="text-sm">{JSON.stringify(harness.memory ? toJson(MemorySchema, harness.memory) : null, null, 2)}</pre>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Session</CardTitle>
              </CardHeader>
              <CardContent>
                <pre className="text-sm">{JSON.stringify(harness.session ? toJson(SessionSchema, harness.session) : null, null, 2)}</pre>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}