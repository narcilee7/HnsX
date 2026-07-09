import { useState } from 'react'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Loading } from '@/components/ui/Loading'
import { ErrorState } from '@/components/ui/Error'
import { Empty } from '@/components/ui/Empty'
import { useDomainVersions, useDomainVersion, useUpdateDomain } from '@/hooks/useDomains'
import { toJson, DomainSpecSchema } from '@hnsx/sdk-node'
import { formatDate } from '@/api/mappers'
import { History, GitCompare, RotateCcw } from 'lucide-react'
import { cn } from '@/lib/utils'

interface VersionsPanelProps {
  domainId: string
  currentVersion: string
  onRollback?: () => void
}

export function VersionsPanel({ domainId, currentVersion, onRollback }: VersionsPanelProps) {
  const { data: versions, isLoading, error, refetch } = useDomainVersions(domainId)
  const [fromVer, setFromVer] = useState<string | null>(null)
  const [toVer, setToVer] = useState<string | null>(null)

  // pick sensible defaults
  if (versions && versions.length > 0 && !fromVer && !toVer) {
    setFromVer(versions[versions.length - 1]?.version ?? null)
    setToVer(currentVersion)
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <History className="h-4 w-4" />
            Version History
          </CardTitle>
          <CardDescription>
            当前 <span className="font-mono">{currentVersion}</span>，共 {versions?.length ?? 0} 个版本
          </CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Loading />
          ) : error ? (
            <ErrorState description={error.message} onRetry={refetch} />
          ) : !versions || versions.length === 0 ? (
            <Empty description="还没有历史版本。" />
          ) : (
            <div className="overflow-hidden rounded-lg border">
              <table className="w-full text-sm">
                <thead className="bg-muted/50 text-xs text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2 text-left font-medium">Version</th>
                    <th className="px-3 py-2 text-left font-medium">Created At</th>
                    <th className="px-3 py-2 text-right font-medium">Action</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {[...versions].reverse().map((v) => {
                    const isCurrent = v.version === currentVersion
                    return (
                      <tr key={v.version} className="hover:bg-muted/30">
                        <td className="px-3 py-2">
                          <div className="flex items-center gap-2">
                            <span className="font-mono text-xs">{v.version}</span>
                            {isCurrent && <Badge variant="default">current</Badge>}
                          </div>
                        </td>
                        <td className="px-3 py-2 text-xs text-muted-foreground">
                          {formatDate(v.created_at ? new Date(v.created_at) : null)}
                        </td>
                        <td className="px-3 py-2 text-right">
                          <div className="inline-flex gap-1">
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => {
                                setFromVer(v.version)
                                setToVer(currentVersion)
                              }}
                              disabled={isCurrent}
                            >
                              <GitCompare className="mr-1 h-3 w-3" />
                              Diff
                            </Button>
                            <RollbackButton
                              domainId={domainId}
                              version={v.version}
                              isCurrent={isCurrent}
                              onSuccess={onRollback}
                            />
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      {fromVer && toVer && fromVer !== toVer && (
        <DiffView domainId={domainId} fromVer={fromVer} toVer={toVer} onClear={() => { setFromVer(null); setToVer(null) }} />
      )}
    </div>
  )
}

function RollbackButton({
  domainId,
  version,
  isCurrent,
  onSuccess,
}: {
  domainId: string
  version: string
  isCurrent: boolean
  onSuccess?: () => void
}) {
  const { data: targetSpec } = useDomainVersion(isCurrent ? undefined : domainId, isCurrent ? undefined : version)
  const update = useUpdateDomain(domainId)

  const handleRollback = () => {
    if (!targetSpec) return
    if (!confirm(`Rollback to ${version}? This will create a new version equal to ${version}.`)) return
    update.mutate(
      { ...targetSpec, version: `${version}-rollback` },
      {
        onSuccess: () => onSuccess?.(),
      },
    )
  }

  return (
    <Button
      size="sm"
      variant="ghost"
      onClick={handleRollback}
      disabled={isCurrent || update.isPending || (!targetSpec && !isCurrent)}
      title={isCurrent ? 'Already current' : `Rollback to ${version}`}
    >
      <RotateCcw className="mr-1 h-3 w-3" />
      Rollback
    </Button>
  )
}

function DiffView({
  domainId,
  fromVer,
  toVer,
  onClear,
}: {
  domainId: string
  fromVer: string
  toVer: string
  onClear: () => void
}) {
  const { data: fromSpec, isLoading: fromLoading } = useDomainVersion(domainId, fromVer)
  const { data: toSpec, isLoading: toLoading } = useDomainVersion(domainId, toVer)

  if (fromLoading || toLoading) return <Loading />
  if (!fromSpec || !toSpec) return null

  const fromText = JSON.stringify(toJson(DomainSpecSchema, fromSpec), null, 2)
  const toText = JSON.stringify(toJson(DomainSpecSchema, toSpec), null, 2)
  const diff = computeLineDiff(fromText, toText)
  const added = diff.filter((d) => d.type === 'add').length
  const removed = diff.filter((d) => d.type === 'remove').length

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <div>
          <CardTitle className="flex items-center gap-2 text-base">
            <GitCompare className="h-4 w-4" />
            Diff: <span className="font-mono">{fromVer}</span> → <span className="font-mono">{toVer}</span>
          </CardTitle>
          <CardDescription className="mt-1">
            <span className="text-[var(--success-text)]">+ {added}</span>
            <span className="mx-2 text-muted-foreground">·</span>
            <span className="text-[var(--danger-text)]">− {removed}</span>
          </CardDescription>
        </div>
        <Button size="sm" variant="ghost" onClick={onClear}>
          Close
        </Button>
      </CardHeader>
      <CardContent>
        <pre className="max-h-[480px] overflow-auto rounded-md border bg-muted/30 p-3 text-[11px] leading-relaxed">
          {diff.map((d, i) => (
            <DiffLine key={i} line={d} />
          ))}
        </pre>
      </CardContent>
    </Card>
  )
}

type DiffLine = { type: 'eq' | 'add' | 'remove'; text: string }

function DiffLine({ line }: { line: DiffLine }) {
  const className = cn(
    'block whitespace-pre-wrap px-1',
    line.type === 'add' && 'bg-[var(--success-soft)] text-[var(--success-text)]',
    line.type === 'remove' && 'bg-[var(--danger-soft)] text-[var(--danger-text)]',
  )
  const prefix = line.type === 'add' ? '+ ' : line.type === 'remove' ? '− ' : '  '
  return (
    <span className={className}>
      <span className="select-none opacity-60">{prefix}</span>
      {line.text}
    </span>
  )
}

/**
 * 简化版 line diff：LCS 找最长公共子序列，标 equal / add / remove。
 * 不用引入 diff 库，自研 50 行足够。
 */
function computeLineDiff(a: string, b: string): DiffLine[] {
  const aLines = a.split('\n')
  const bLines = b.split('\n')
  const m = aLines.length
  const n = bLines.length

  // LCS dp table
  const dp: number[][] = Array.from({ length: m + 1 }, () => Array(n + 1).fill(0))
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      if (aLines[i - 1] === bLines[j - 1]) {
        dp[i]![j] = (dp[i - 1]![j - 1] ?? 0) + 1
      } else {
        dp[i]![j] = Math.max(dp[i - 1]![j] ?? 0, dp[i]![j - 1] ?? 0)
      }
    }
  }

  // backtrack
  const result: DiffLine[] = []
  let i = m
  let j = n
  while (i > 0 && j > 0) {
    if (aLines[i - 1] === bLines[j - 1]) {
      result.push({ type: 'eq', text: aLines[i - 1]! })
      i--
      j--
    } else if ((dp[i - 1]![j] ?? 0) >= (dp[i]![j - 1] ?? 0)) {
      result.push({ type: 'remove', text: aLines[i - 1]! })
      i--
    } else {
      result.push({ type: 'add', text: bLines[j - 1]! })
      j--
    }
  }
  while (i > 0) {
    result.push({ type: 'remove', text: aLines[i - 1]! })
    i--
  }
  while (j > 0) {
    result.push({ type: 'add', text: bLines[j - 1]! })
    j--
  }
  return result.reverse()
}