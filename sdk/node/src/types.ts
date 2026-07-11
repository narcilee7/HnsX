/**
 * Wire types for the HnsX REST API. These mirror the JSON envelopes produced
 * by hnsx-server/pkg/api and are intentionally decoupled from the proto
 * generated types so the REST client can be used without pulling Connect.
 */

export interface DomainSummary {
  id: string
  version: string
  description?: string
  status: string
  created_at?: string
  updated_at?: string
}

export interface Domain {
  id: string
  version: string
  description?: string
  harness?: Record<string, unknown>
  status?: string
  created_at?: string
  updated_at?: string
}

export interface SessionSummary {
  id: string
  domain_id: string
  domain_version?: string
  orchestration?: string
  state: string
  started_at?: string
  completed_at?: string
}

export interface Session {
  id: string
  domain_id: string
  domain_version?: string
  orchestration?: string
  state: string
  trigger?: Record<string, unknown>
  started_at?: string
  completed_at?: string
  result?: Record<string, unknown>
}

export interface TraceSummary {
  trace_id: string
  session_id: string
  domain_id: string
  domain_version?: string
  status: string
  started_at?: string
  completed_at?: string
  duration_ms?: number
  observation_count?: number
  total_cost_usd?: number
  prompt_tokens?: number
  completion_tokens?: number
  agent_invocations?: number
  tool_invocations?: number
}

export interface Trace {
  trace_id: string
  session_id: string
  domain_id: string
  domain_version?: string
  status: string
  started_at?: string
  completed_at?: string
  duration_ms?: number
  observation_count?: number
  total_cost_usd?: number
  prompt_tokens?: number
  completion_tokens?: number
  agent_invocations?: number
  tool_invocations?: number
  observations?: Observation[]
}

export interface Observation {
  kind: string
  trace_id?: string
  session_id?: string
  domain_id?: string
  domain_version?: string
  step_id?: string
  agent_id?: string
  payload?: Record<string, unknown>
  metadata?: Record<string, unknown>
  cost_usd?: number
  prompt_tokens?: number
  completion_tokens?: number
  latency_ms?: number
  timestamp?: string
}

export interface Approval {
  id: string
  session_id?: string
  domain_id?: string
  action?: string
  resource?: string
  risk_level?: string
  context?: Record<string, unknown>
  status: string
  requested_by?: string
  reviewed_by?: string
  comment?: string
  created_at?: string
  updated_at?: string
  resolved_at?: string
}

export interface EvalSetSummary {
  id: string
  set_id: string
  domain_id: string
  description?: string
  case_count?: number
  created_at?: string
}

export interface EvalSet extends EvalSetSummary {
  cases: EvalCase[]
  updated_at?: string
}

export interface EvalCase {
  id: string
  name?: string
  input: Record<string, unknown>
  expect?: Record<string, unknown>
  scorer?: EvalScorer
}

export interface EvalScorer {
  type: string
  config?: Record<string, unknown>
}

export interface EvalRun {
  id: string
  eval_set_id: string
  domain_id: string
  domain_version?: string
  orchestration?: string
  state: string
  score?: number
  total_cases: number
  passed_cases?: number
  total_cost_usd?: number
  duration_ms?: number
  cases?: EvalCaseResult[]
  created_at?: string
  completed_at?: string
}

export interface EvalCaseResult {
  case_id: string
  session_id?: string
  score?: number
  passed?: boolean
  actual?: Record<string, unknown>
  details?: Record<string, unknown>
  duration_ms?: number
  cost_usd?: number
}

export interface ListEnvelope<T> {
  items: T[]
  total: number
  limit?: number
  offset?: number
}

export interface SSEEvent {
  name: string
  payload: string
}
