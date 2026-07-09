import { useMemo, useState } from 'react'
import type { Harness } from '@hnsx/sdk-node'
import { cn } from '@/lib/utils'

interface HarnessGraphProps {
  harness?: Harness
  className?: string
}

interface NodeLayout {
  id: string
  kind: 'agent' | 'tool' | 'skill' | 'mcp' | 'prompt'
  x: number
  y: number
  width: number
  height: number
  /** referenced names (for tooltip / detail panel) */
  meta?: string
}

const LANE_GAP = 100 // vertical space between lanes
const NODE_WIDTH = 132
const NODE_HEIGHT = 44
const LANE_PADDING_X = 24
const LANE_PADDING_Y = 16
const EDGE_COLOR = 'var(--chart-grid)'

/**
 * Harness 五要素拓扑图：手画 SVG，无外部 graph 库依赖。
 * 设计原则：
 *   - 每个 entity type 一条 lane（横向 row），从上到下：Agent → Tool → Skill → MCP
 *   - 节点：圆角矩形，色按 chart 槽位
 *   - 边：agent.toolRefs / skillRefs / mcpRefs 拉 bezier 曲线到底层节点
 *   - 点击节点：右侧/下方 detail 面板显示完整定义
 *   - 横向 scroll：宽于容器时溢出滚动
 */
export function HarnessGraph({ harness, className }: HarnessGraphProps) {
  const [selected, setSelected] = useState<string | null>(null)

  const { nodes, edges, width, height } = useMemo(() => {
    if (!harness) return { nodes: [] as NodeLayout[], edges: [] as Edge[], width: 0, height: 0 }

    const usedTools = new Set<string>()
    const usedSkills = new Set<string>()
    const usedMcps = new Set<string>()
    for (const agent of harness.agents) {
      for (const t of agent.toolRefs) usedTools.add(t)
      for (const s of agent.skillRefs) usedSkills.add(s)
    }

    // Determine which tools/skills/mcps are actually referenced (skip orphans)
    const toolsToShow = harness.tools.filter((t) => usedTools.has(t.id))
    const skillsToShow = harness.skills.filter((s) => usedSkills.has(s.id))
    const mcpsToShow = harness.mcps.filter((m) => usedMcps.has(m.id))

    const lanes: { kind: NodeLayout['kind']; ids: string[]; meta?: (id: string) => string | undefined }[] = [
      { kind: 'agent' as const, ids: harness.agents.map((a) => a.id), meta: (id: string) => harness.agents.find((a) => a.id === id)?.description },
      { kind: 'tool' as const, ids: toolsToShow.map((t) => t.id), meta: (id: string) => harness.tools.find((t) => t.id === id)?.description },
      { kind: 'skill' as const, ids: skillsToShow.map((s) => s.id), meta: (id: string) => harness.skills.find((s) => s.id === id)?.description },
      { kind: 'mcp' as const, ids: mcpsToShow.map((m) => m.id) },
    ].filter((lane) => lane.ids.length > 0)

    const widestLane = Math.max(0, ...lanes.map((l) => l.ids.length))
    const innerWidth = Math.max(widestLane * (NODE_WIDTH + 16) - 16, 0)
    const totalWidth = innerWidth + LANE_PADDING_X * 2

    const positions = new Map<string, NodeLayout>()
    let y = LANE_PADDING_Y
    for (const lane of lanes) {
      const startX = LANE_PADDING_X + (innerWidth - (lane.ids.length * (NODE_WIDTH + 16) - 16)) / 2
      lane.ids.forEach((id, i) => {
        positions.set(id, {
          id,
          kind: lane.kind,
          x: startX + i * (NODE_WIDTH + 16),
          y,
          width: NODE_WIDTH,
          height: NODE_HEIGHT,
          meta: lane.meta?.(id),
        })
      })
      y += LANE_GAP
    }

    const edges: Edge[] = []
    for (const agent of harness.agents) {
      const a = positions.get(agent.id)
      if (!a) continue
      for (const t of agent.toolRefs) {
        const target = positions.get(t)
        if (target) edges.push({ from: a, to: target, kind: 'tool' })
      }
      for (const s of agent.skillRefs) {
        const target = positions.get(s)
        if (target) edges.push({ from: a, to: target, kind: 'skill' })
      }
    }

    const totalHeight = y - LANE_GAP + LANE_PADDING_Y
    return { nodes: Array.from(positions.values()), edges, width: totalWidth, height: totalHeight }
  }, [harness])

  if (!harness) {
    return <p className="text-sm text-muted-foreground">No harness configuration available.</p>
  }
  if (nodes.length === 0) {
    return <p className="text-sm text-muted-foreground">Harness is empty — no agents, tools, or skills configured.</p>
  }

  return (
    <div className={cn('grid gap-4 lg:grid-cols-[1fr_280px]', className)}>
      <div className="overflow-x-auto rounded-lg border bg-card p-4">
        <svg
          width={width}
          height={height}
          viewBox={`0 0 ${width} ${height}`}
          style={{ minWidth: width }}
          role="img"
          aria-label="Harness topology graph"
        >
          <defs>
            <marker
              id="arrowhead"
              viewBox="0 0 10 10"
              refX="9"
              refY="5"
              markerWidth="5"
              markerHeight="5"
              orient="auto-start-reverse"
            >
              <path d="M0,0 L10,5 L0,10 z" fill={EDGE_COLOR} />
            </marker>
          </defs>
          {/* edges first (below nodes) */}
          {edges.map((e, i) => {
            const x1 = e.from.x + e.from.width / 2
            const y1 = e.from.y + e.from.height
            const x2 = e.to.x + e.to.width / 2
            const y2 = e.to.y
            const midY = (y1 + y2) / 2
            const path = `M ${x1} ${y1} C ${x1} ${midY}, ${x2} ${midY}, ${x2} ${y2}`
            return (
              <path
                key={`edge-${i}`}
                d={path}
                fill="none"
                stroke={EDGE_COLOR}
                strokeWidth={1}
                strokeDasharray={e.kind === 'skill' ? '3 3' : undefined}
                markerEnd="url(#arrowhead)"
                opacity={selected && selected !== e.from.id && selected !== e.to.id ? 0.2 : 0.7}
              />
            )
          })}

          {/* nodes */}
          {nodes.map((n) => {
            const isSelected = selected === n.id
            const tone = kindToVar(n.kind)
            return (
              <g
                key={n.id}
                transform={`translate(${n.x}, ${n.y})`}
                onClick={() => setSelected(n.id === selected ? null : n.id)}
                style={{ cursor: 'pointer' }}
              >
                <rect
                  width={n.width}
                  height={n.height}
                  rx={6}
                  fill="var(--card)"
                  stroke={tone}
                  strokeWidth={isSelected ? 2.5 : 1.25}
                  style={{ transition: 'stroke-width 120ms' }}
                />
                <rect
                  x={0}
                  y={0}
                  width={4}
                  height={n.height}
                  rx={2}
                  fill={tone}
                />
                <text
                  x={12}
                  y={18}
                  fontSize={10}
                  fill="var(--chart-text-muted)"
                  fontFamily="ui-monospace, monospace"
                  style={{ textTransform: 'uppercase' }}
                >
                  {n.kind}
                </text>
                <text
                  x={12}
                  y={34}
                  fontSize={12}
                  fill="var(--chart-text-primary)"
                  fontWeight={500}
                  style={{ pointerEvents: 'none' }}
                >
                  {truncate(n.id, 16)}
                </text>
              </g>
            )
          })}
        </svg>
      </div>

      {/* detail panel */}
      <div className="rounded-lg border bg-card p-4">
        <DetailPanel harness={harness} selectedId={selected} />
      </div>
    </div>
  )
}

interface Edge {
  from: NodeLayout
  to: NodeLayout
  kind: 'tool' | 'skill'
}

function DetailPanel({ harness, selectedId }: { harness: Harness; selectedId: string | null }) {
  if (!selectedId) {
    return (
      <div className="flex h-full min-h-[120px] items-center justify-center text-center text-xs text-muted-foreground">
        点击节点查看定义
      </div>
    )
  }
  const agent = harness.agents.find((a) => a.id === selectedId)
  if (agent) {
    return (
      <div className="space-y-2">
        <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--chart-1)]">agent</p>
        <p className="font-mono text-sm font-medium">{agent.id}</p>
        {agent.description && <p className="text-xs text-muted-foreground">{agent.description}</p>}
        <div className="space-y-1 pt-1 text-xs">
          {agent.model && <Field label="model" value={`${agent.model.provider}/${agent.model.model}`} />}
          {agent.adapter && <Field label="adapter" value={`${agent.adapter.kind}${agent.adapter.timeoutSeconds ? ` · ${agent.adapter.timeoutSeconds}s` : ''}`} />}
          {agent.prompt && <Field label="prompt" value={agent.prompt.id} />}
          {agent.toolRefs.length > 0 && <Field label="tools" value={agent.toolRefs.join(', ')} />}
          {agent.skillRefs.length > 0 && <Field label="skills" value={agent.skillRefs.join(', ')} />}
        </div>
      </div>
    )
  }
  const tool = harness.tools.find((t) => t.id === selectedId)
  if (tool) {
    return (
      <div className="space-y-2">
        <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--chart-2)]">tool</p>
        <p className="font-mono text-sm font-medium">{tool.id}</p>
        {tool.description && <p className="text-xs text-muted-foreground">{tool.description}</p>}
        {tool.type && <Field label="type" value={tool.type} />}
        {tool.config && <Field label="config" value={tool.config} />}
      </div>
    )
  }
  const skill = harness.skills.find((s) => s.id === selectedId)
  if (skill) {
    return (
      <div className="space-y-2">
        <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--chart-3)]">skill</p>
        <p className="font-mono text-sm font-medium">{skill.id}</p>
        {skill.description && <p className="text-xs text-muted-foreground">{skill.description}</p>}
      </div>
    )
  }
  const mcp = harness.mcps.find((m) => m.id === selectedId)
  if (mcp) {
    return (
      <div className="space-y-2">
        <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--chart-4)]">mcp</p>
        <p className="font-mono text-sm font-medium">{mcp.id}</p>
        {mcp.command && <Field label="command" value={mcp.command} />}
        {mcp.args && mcp.args.length > 0 && <Field label="args" value={mcp.args.join(' ')} />}
      </div>
    )
  }
  return null
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-2">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-mono text-right text-[10px]">{value}</span>
    </div>
  )
}

function kindToVar(kind: NodeLayout['kind']): string {
  switch (kind) {
    case 'agent':
      return 'var(--chart-1)'
    case 'tool':
      return 'var(--chart-2)'
    case 'skill':
      return 'var(--chart-3)'
    case 'mcp':
      return 'var(--chart-4)'
    case 'prompt':
      return 'var(--chart-5)'
  }
}

function truncate(s: string, n: number): string {
  return s.length > n ? `${s.slice(0, n - 1)}…` : s
}