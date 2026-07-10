import { get } from './client'
import { mapTraceRecord, mapTraceSummaryFromJson } from './mappers'
import type { TraceSummaryViewModel, TraceViewModel } from './mappers'
import type { JsonValue } from '@bufbuild/protobuf'
import { fromJson, TraceRecordSchema } from '@hnsx/sdk-node'

export interface TraceListParams {
  domain?: string
  session?: string
  agent?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export function listTraces(params: TraceListParams = {}): Promise<{
  items: TraceSummaryViewModel[]
  total: number
}> {
  const search = new URLSearchParams()
  if (params.domain) search.set('domain', params.domain)
  if (params.session) search.set('session', params.session)
  if (params.agent) search.set('agent', params.agent)
  if (params.from) search.set('from', params.from)
  if (params.to) search.set('to', params.to)
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.offset !== undefined) search.set('offset', String(params.offset))
  return get<unknown>(`/traces?${search.toString()}`).then((res) => {
    const data = res as { items: unknown[]; total: number }
    return {
      items: data.items.map((item) => mapTraceSummaryFromJson(item as Record<string, unknown>)),
      total: data.total,
    }
  })
}

export function getTrace(traceId: string): Promise<TraceViewModel> {
  return get<unknown>(`/traces/${traceId}`).then((res) =>
    mapTraceRecord(fromJson(TraceRecordSchema, res as JsonValue)),
  )
}
