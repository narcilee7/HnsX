import { get, post, put, del } from './client'
import { mapDomainSummaryFromJson } from './mappers'
import type { DomainSummary } from './mappers'
import type { JsonValue } from '@bufbuild/protobuf'
import {
  fromJson,
  toJson,
  type DomainSpec,
  DomainSpecSchema,
} from '@hnsx/sdk-node'

export interface ListParams {
  limit?: number
  offset?: number
}

export function listDomains(params: ListParams = {}): Promise<{ items: DomainSummary[]; total: number }> {
  const search = new URLSearchParams()
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.offset !== undefined) search.set('offset', String(params.offset))
  return get<unknown>(`/domains?${search.toString()}`).then((res) => {
    const data = res as { items: unknown[]; total: number }
    return {
      items: data.items.map((item) =>
        mapDomainSummaryFromJson(item as Parameters<typeof mapDomainSummaryFromJson>[0]),
      ),
      total: data.total,
    }
  })
}

export function getDomain(id: string): Promise<DomainSpec> {
  return get<unknown>(`/domains/${id}`).then((res) => fromJson(DomainSpecSchema, res as JsonValue))
}

export function createDomain(spec: DomainSpec): Promise<{ id: string }> {
  return post('/domains', toJson(DomainSpecSchema, spec))
}

export function updateDomain(id: string, spec: DomainSpec): Promise<void> {
  return put(`/domains/${id}`, toJson(DomainSpecSchema, spec))
}

export function deleteDomain(id: string): Promise<void> {
  return del(`/domains/${id}`)
}

export function validateDomain(id: string, harness: unknown): Promise<{ valid: boolean; errors?: string[] }> {
  return post(`/domains/${id}/validate`, { harness })
}

export function listDomainVersions(id: string): Promise<{ version: string; created_at: string }[]> {
  return get(`/domains/${id}/versions`)
}

export function getDomainVersion(id: string, version: string): Promise<DomainSpec> {
  return get<unknown>(`/domains/${id}/versions/${version}`).then((res) =>
    fromJson(DomainSpecSchema, res as JsonValue),
  )
}

