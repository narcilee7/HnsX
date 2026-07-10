import { get, post, put, del } from './client'
import { mapEvalSet, mapEvalRunResult } from './mappers'
import type { EvalRunViewModel, EvalSetViewModel } from './mappers'
import type { JsonValue } from '@bufbuild/protobuf'
import {
  fromJson,
  type EvalCase,
  type EvalRunResult,
  type EvalSet,
  EvalCaseSchema,
  EvalRunResultSchema,
  EvalSetSchema,
} from '@hnsx/sdk-node'

export interface ListParams {
  limit?: number
  offset?: number
}

export interface EvalSetListItem {
  id: string
  set_id: string
  domain_id: string
  description?: string
  case_count: number
  created_at?: string
}

/**
 * Server returns a flat items[] list (no domain nesting). Console pages
 * filter by domain client-side once the list lands — keeps round-trips
 * down and matches the server's flat /api/v1/evals shape.
 */
export function listEvalSets(params: ListParams = {}): Promise<{
  items: EvalSetListItem[]
  total: number
}> {
  const search = new URLSearchParams()
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.offset !== undefined) search.set('offset', String(params.offset))
  return get<unknown>(`/evals?${search.toString()}`).then((res) => {
    const data = res as { items: EvalSetListItem[]; total: number }
    return data
  })
}

export function getEvalSet(setId: string): Promise<EvalSetViewModel> {
  return get<unknown>(`/evals/${setId}`).then((res) =>
    mapEvalSet(fromJson(EvalSetSchema, res as JsonValue), ''),
  )
}

export function updateEvalSet(
  setId: string,
  body: {
    description?: string
    cases: EvalCase[]
  },
): Promise<{ id: string; set_id: string }> {
  return put(`/evals/${setId}`, body)
}

export function deleteEvalSet(setId: string): Promise<void> {
  return del(`/evals/${setId}`)
}

export function createEvalSet(body: {
  set_id: string
  domain_id: string
  description?: string
  cases: EvalCase[]
}): Promise<{ id: string; set_id: string }> {
  return post('/evals', body)
}

export function runEval(
  setId: string,
  body: { orchestration?: string; baseline_run_id?: string } = {},
): Promise<{ run_id: string; state: string }> {
  return post(`/evals/${setId}/run`, body)
}

export interface EvalRunListItem {
  id: string
  eval_set_id: string
  domain_id: string
  domain_version: string
  orchestration: string
  state: string
  score: number
  total_cases: number
  passed_cases: number
  total_cost_usd: number
  duration_ms: number
  created_at?: string
  completed_at?: string
}

export function listEvalRuns(
  setId: string,
  params: ListParams = {},
): Promise<{
  items: EvalRunListItem[]
  total: number
}> {
  const search = new URLSearchParams()
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.offset !== undefined) search.set('offset', String(params.offset))
  return get<unknown>(`/evals/${setId}/runs?${search.toString()}`).then((res) => {
    const data = res as { items: EvalRunListItem[]; total: number }
    return data
  })
}

export function getEvalRun(setId: string, runId: string): Promise<EvalRunViewModel> {
  return get<unknown>(`/evals/${setId}/runs/${runId}`).then((res) =>
    mapEvalRunResult(fromJson(EvalRunResultSchema, res as JsonValue)),
  )
}

// Re-exports for typed callers that already had them through domains.ts
export type { EvalCase, EvalSet, EvalRunResult }
export { EvalCaseSchema, EvalSetSchema, EvalRunResultSchema }
