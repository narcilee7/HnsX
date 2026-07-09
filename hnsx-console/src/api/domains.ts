import { get, post, put, del } from './client'
import { mapDomainSummaryFromJson } from './mappers'
import type { DomainSummary } from './mappers'
import type { JsonValue } from '@bufbuild/protobuf'
import {
  fromJson,
  toJson,
  type DomainSpec,
  type EvalSet,
  type EvalRunResult,
  DomainSpecSchema,
  EvalSetSchema,
  EvalRunResultSchema,
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

export function listEvalSets(domainId: string): Promise<EvalSet[]> {
  return get<unknown>(`/domains/${domainId}/evals`).then((res) => {
    const data = res as { items: unknown[] }
    return data.items.map((item) => fromJson(EvalSetSchema, item as JsonValue))
  })
}

export function getEvalSet(domainId: string, setId: string): Promise<EvalSet> {
  return get<unknown>(`/domains/${domainId}/evals/${setId}`).then((res) => fromJson(EvalSetSchema, res as JsonValue))
}

export function createEvalSet(domainId: string, set: EvalSet): Promise<void> {
  return post(`/domains/${domainId}/evals`, toJson(EvalSetSchema, set))
}

export function updateEvalSet(domainId: string, setId: string, set: EvalSet): Promise<void> {
  return put(`/domains/${domainId}/evals/${setId}`, toJson(EvalSetSchema, set))
}

export function deleteEvalSet(domainId: string, setId: string): Promise<void> {
  return del(`/domains/${domainId}/evals/${setId}`)
}

export function runEval(
  domainId: string,
  setId: string,
  body: { orchestration?: string; baseline_run_id?: string } = {},
): Promise<{ eval_run_id: string; state: string }> {
  return post(`/domains/${domainId}/evals/${setId}/run`, body)
}

export function listEvalRuns(
  domainId: string,
  setId: string,
  params: ListParams = {},
): Promise<{ items: EvalRunResult[]; total: number }> {
  const search = new URLSearchParams()
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.offset !== undefined) search.set('offset', String(params.offset))
  return get<unknown>(`/domains/${domainId}/evals/${setId}/runs?${search.toString()}`).then((res) => {
    const data = res as { items: unknown[]; total: number }
    return {
      items: data.items.map((item) => fromJson(EvalRunResultSchema, item as JsonValue)),
      total: data.total,
    }
  })
}

export function getEvalRun(domainId: string, setId: string, runId: string): Promise<EvalRunResult> {
  return get<unknown>(`/domains/${domainId}/evals/${setId}/runs/${runId}`).then((res) =>
    fromJson(EvalRunResultSchema, res as JsonValue),
  )
}
