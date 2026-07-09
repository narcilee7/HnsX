import { useMemo, useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { cn } from '../lib/utils'
import { formatDurationMs } from '../lib/utils'

export interface SpanListItem {
  /** Span 唯一 ID */
  id: string
  /** 名称 — 如 'tool_call:web_search'、'agent:planner.run' */
  name: string
  /** 类型 — 用于颜色和过滤 */
  kind?: string
  /** agent id / span 来源 */
  agentId?: string
  /** role（user / assistant / tool / system） */
  role?: string
  /** 开始 / 结束时间（毫秒） */
  startMs: number
  endMs: number
  /** 嵌套用 — 父 span id */
  parentId?: string
  /** token 用量 */
  tokens?: { input?: number; output?: number; total?: number }
  /** 详细 payload — 展开时显示 */
  payload?: Record<string, unknown>
  /** metadata — 展开时显示 */
  metadata?: Record<string, unknown>
}

export interface SpanListProps {
  spans: SpanListItem[]
  /** trace 起始时间（毫秒） — 不传则用最小 startMs */
  originMs?: number
  /** 整个 trace 的总时长（毫秒） — 不传则自动算 */
  totalMs?: number
  /** 高度 — 默认 480 */
  height?: number | string
  className?: string
  /** 渲染头部 — 通常放 trace 总览统计 */
  header?: React.ReactNode
  /** 嵌套层级 — 默认 true 显示父/子层级 */
  nested?: boolean
  /** 默认展开 — 默认 false */
  defaultExpanded?: boolean
}

const kindVariant: Record<string, string> = {
  text: 'chart-1',
  message: 'chart-1',
  user: 'chart-3',
  assistant: 'chart-1',
  tool_call: 'chart-2',
  tool_result: 'chart-2',
  state: 'chart-5',
  cost: 'chart-4',
  error: 'danger',
  thinking: 'chart-5',
}

/**
 * Span 列表视图。设计原则：
 *   - 每条 span 显示：name | agent | kind | role | duration bar | tokens
 *   - 默认按 startMs 升序
 *   - 点击展开 payload + metadata
 *   - 嵌套层级用缩进 + 左侧色条体现父子关系
 *   - 时间条按 (startMs - origin) / total 比例绘制
 */
export function SpanList({
  spans,
  originMs,
  totalMs,
  height = 480,
  className,
  header,
  nested = true,
  defaultExpanded = false,
}: SpanListProps) {
  const { ordered, origin, total, root } = useMemo(() => {
    if (spans.length === 0) {
      return { ordered: [] as SpanListItem[], origin: 0, total: 1, root: [] as SpanListItem[] }
    }
    const o = originMs ?? Math.min(...spans.map((s) => s.startMs))
    const t = totalMs ?? Math.max(...spans.map((s) => s.endMs)) - o
    const sorted = [...spans].sort((a, b) => a.startMs - b.startMs)
    const rootItems = sorted.filter((s) => !s.parentId)
    return { ordered: sorted, origin: o, total: Math.max(t, 1), root: rootItems }
  }, [spans, originMs, totalMs])

  if (spans.length === 0) {
    return (
      <div
        className={cn(
          'flex flex-col items-center justify-center gap-1 rounded-lg border border-[var(--chart-grid)] bg-[var(--card)] p-6 text-center',
          className,
        )}
        style={{ minHeight: height }}
      >
        <p className="text-sm text-[var(--chart-text-secondary)]">暂无 Span</p>
        <p className="text-xs text-[var(--chart-text-muted)]">Trace 执行后会按时间顺序列出所有 Observation。</p>
      </div>
    )
  }

  return (
    <div
      className={cn('flex flex-col rounded-lg border border-[var(--chart-grid)] bg-[var(--card)]', className)}
      style={{ height }}
    >
      {header}
      <div className="flex-1 overflow-auto">
        {nested
          ? root.map((span) => (
              <SpanRow
                key={span.id}
                span={span}
                all={ordered}
                depth={0}
                origin={origin}
                total={total}
                defaultExpanded={defaultExpanded}
              />
            ))
          : ordered.map((span) => (
              <SpanRow
                key={span.id}
                span={span}
                all={ordered}
                depth={0}
                origin={origin}
                total={total}
                defaultExpanded={defaultExpanded}
                skipNesting
              />
            ))}
      </div>
    </div>
  )
}

interface SpanRowProps {
  span: SpanListItem
  all: SpanListItem[]
  depth: number
  origin: number
  total: number
  defaultExpanded: boolean
  skipNesting?: boolean
}

function SpanRow({ span, all, depth, origin, total, defaultExpanded, skipNesting }: SpanRowProps) {
  const [expanded, setExpanded] = useState(defaultExpanded)
  const variant = kindVariant[span.kind ?? ''] ?? 'chart-5'
  const duration = span.endMs - span.startMs
  const hasChildren = !skipNesting && all.some((s) => s.parentId === span.id)
  const tokens = span.tokens
  const totalTokens = tokens?.total ?? (tokens?.input ?? 0) + (tokens?.output ?? 0)

  const left = ((span.startMs - origin) / total) * 100
  const widthPct = Math.max(((span.endMs - span.startMs) / total) * 100, 0.5)

  const children = skipNesting ? [] : all.filter((s) => s.parentId === span.id)

  return (
    <>
      <div
        className={cn(
          'group border-b border-[var(--chart-grid)] transition-colors hover:bg-[var(--chart-grid)]/40',
        )}
        style={{ paddingLeft: 8 + depth * 16 }}
      >
        <div className="flex items-center gap-2 px-2 py-2">
          {/* expand toggle */}
          <button
            type="button"
            onClick={() => setExpanded((v) => !v)}
            className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-[var(--chart-text-muted)] hover:bg-[var(--chart-grid)]"
            aria-label={expanded ? 'Collapse' : 'Expand'}
          >
            {hasChildren || (span.payload && Object.keys(span.payload).length > 0) ? (
              expanded ? (
                <ChevronDown className="h-3.5 w-3.5" />
              ) : (
                <ChevronRight className="h-3.5 w-3.5" />
              )
            ) : null}
          </button>

          {/* name + meta */}
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <span
              className="h-2 w-2 shrink-0 rounded-sm"
              style={{ background: `var(--${variant})` }}
              aria-hidden
            />
            <span className="truncate text-sm font-medium text-[var(--chart-text-primary)]">{span.name}</span>
            {span.agentId && (
              <span className="shrink-0 rounded bg-[var(--chart-grid)]/60 px-1.5 py-0.5 text-[10px] text-[var(--chart-text-secondary)]">
                {span.agentId}
              </span>
            )}
            {span.role && (
              <span className="shrink-0 rounded border border-[var(--chart-grid)] px-1.5 py-0.5 text-[10px] text-[var(--chart-text-muted)]">
                {span.role}
              </span>
            )}
          </div>

          {/* duration */}
          <div className="flex shrink-0 items-center gap-3">
            {totalTokens > 0 && (
              <span className="text-xs tabular-nums text-[var(--chart-text-muted)]">
                {formatTokensCompact(totalTokens)} tok
              </span>
            )}
            <span className="w-20 text-right text-xs tabular-nums text-[var(--chart-text-secondary)]">
              {formatDurationMs(duration)}
            </span>
          </div>
        </div>

        {/* 时间条 — 横跨整行 */}
        <div className="relative mx-2 mb-2 h-1.5 rounded bg-[var(--chart-grid)]/40">
          <div
            className="absolute top-0 h-full rounded"
            style={{
              left: `${left}%`,
              width: `${widthPct}%`,
              background: `var(--${variant})`,
              opacity: 0.7,
            }}
          />
        </div>

        {/* expanded payload */}
        {expanded && (span.payload || span.metadata) && (
          <div className="grid gap-2 px-2 pb-3 md:grid-cols-2">
            {span.payload && Object.keys(span.payload).length > 0 && (
              <div className="space-y-1">
                <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--chart-text-muted)]">
                  Payload
                </p>
                <pre className="max-h-40 overflow-auto rounded bg-[var(--chart-grid)]/40 p-2 text-[11px] text-[var(--chart-text-secondary)]">
                  {JSON.stringify(span.payload, null, 2)}
                </pre>
              </div>
            )}
            {span.metadata && Object.keys(span.metadata).length > 0 && (
              <div className="space-y-1">
                <p className="text-[10px] font-medium uppercase tracking-wider text-[var(--chart-text-muted)]">
                  Metadata
                </p>
                <pre className="max-h-40 overflow-auto rounded bg-[var(--chart-grid)]/40 p-2 text-[11px] text-[var(--chart-text-secondary)]">
                  {JSON.stringify(span.metadata, null, 2)}
                </pre>
              </div>
            )}
          </div>
        )}
      </div>

      {/* children */}
      {expanded &&
        children.map((child) => (
          <SpanRow
            key={child.id}
            span={child}
            all={all}
            depth={depth + 1}
            origin={origin}
            total={total}
            defaultExpanded={defaultExpanded}
          />
        ))}
    </>
  )
}

function formatTokensCompact(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${(n / 1000).toFixed(n < 10000 ? 1 : 0)}K`
  return `${(n / 1_000_000).toFixed(1)}M`
}