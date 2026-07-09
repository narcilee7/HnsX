import { get } from './client'
import { mapAuditRecord } from './mappers'
import type { AuditLogViewModel } from './mappers'
import type { JsonValue } from '@bufbuild/protobuf'
import { fromJson, AuditRecordSchema } from '@hnsx/sdk-node'

export interface AuditListParams {
  action?: string
  decision?: string
  actor?: string
  resource?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export function listAuditLogs(params: AuditListParams = {}): Promise<{
  items: AuditLogViewModel[]
  total: number
}> {
  const search = new URLSearchParams()
  if (params.action) search.set('action', params.action)
  if (params.decision) search.set('decision', params.decision)
  if (params.actor) search.set('actor', params.actor)
  if (params.resource) search.set('resource', params.resource)
  if (params.from) search.set('from', params.from)
  if (params.to) search.set('to', params.to)
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.offset !== undefined) search.set('offset', String(params.offset))
  return get<unknown>(`/audit?${search.toString()}`).then((res) => {
    const data = res as { items: unknown[]; total: number }
    return {
      items: data.items.map((item) => mapAuditRecord(fromJson(AuditRecordSchema, item as JsonValue))),
      total: data.total,
    }
  })
}
