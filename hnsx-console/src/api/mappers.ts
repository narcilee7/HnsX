import { format, fromUnixTime } from 'date-fns'
import type { JsonValue } from '@bufbuild/protobuf'
import {
  toJson,
  type DomainSpec,
  type SessionStatus,
  type TraceRecord,
  type EvalSet,
  type EvalCase,
  type EvalRunResult,
  type AuditRecord,
  type Observation,
  AgentSchema,
  PromptSchema,
  SkillSchema,
  ToolSchema,
  MCPConfigSchema,
  ObservationSchema,
  EvalCaseResultSchema,
  SandboxSchema,
  PolicySchema,
  MemorySchema,
  SessionSchema,
} from '@hnsx/sdk-node'

export interface DomainSummary {
  id: string
  version: string
  description: string
  status: string
  createdAt: Date | null
  updatedAt: Date | null
}

export interface SessionViewModel {
  id: string
  domainId: string
  domainVersion: string
  state: string
  result?: string
  traceId?: string
  startedAt: Date | null
  completedAt: Date | null
}

export interface TraceViewModel {
  traceId: string
  sessionId: string
  domainId: string
  domainVersion: string
  status: string
  startedAt: Date | null
  durationMs?: number
  agentRefs: string[]
  observations: JsonValue[]
}

export interface ObservationViewModel {
  observationId: string
  traceId: string
  sessionId: string
  domainId: string
  domainVersion: string
  stepId?: string
  agentId?: string
  parentId?: string
  kind: string
  role?: string
  payload: JsonValue
  metadata: JsonValue
  createdAt: Date | null
}

export interface EvalSetViewModel {
  id: string
  domainId: string
  description: string
  cases: EvalCaseViewModel[]
}

export interface EvalCaseViewModel {
  id: string
  name: string
  input: string
  expect: string
}

export interface EvalRunViewModel {
  id: string
  domainId: string
  setId: string
  state: string
  score: number
  total: number
  passed: number
  totalCostUsd: number
  durationMs: number
  baselineRunId?: string
  cases: JsonValue[]
}

export interface AuditLogViewModel {
  id: string
  timestamp: Date | null
  sessionId?: string
  domainId?: string
  action: string
  actor: string
  resource?: string
  decision?: string
  reason?: string
}

export function toDate(value: string | number | bigint | undefined | null): Date | null {
  if (value === undefined || value === null || value === '') return null
  if (typeof value === 'bigint') {
    return fromUnixTime(Number(value) / 1000)
  }
  if (typeof value === 'number') {
    return fromUnixTime(value / 1000)
  }
  return new Date(value)
}

export function formatDate(date: Date | null): string {
  if (!date) return '-'
  return format(date, 'yyyy-MM-dd HH:mm:ss')
}

export function parseJsonField(value: string | undefined | null): JsonValue {
  if (!value) return {}
  try {
    return JSON.parse(value) as JsonValue
  } catch {
    return value
  }
}

export function mapDomainSummary(spec: DomainSpec): DomainSummary {
  return {
    id: spec.id,
    version: spec.version,
    description: spec.description,
    status: 'active',
    createdAt: null,
    updatedAt: null,
  }
}

export function mapDomainSummaryFromJson(json: {
  id: string
  version: string
  description: string
  status?: string
  created_at?: string
  updated_at?: string
}): DomainSummary {
  return {
    id: json.id,
    version: json.version,
    description: json.description,
    status: json.status || 'active',
    createdAt: toDate(json.created_at),
    updatedAt: toDate(json.updated_at),
  }
}

export function mapSessionStatus(status: SessionStatus): SessionViewModel {
  return {
    id: status.sessionId,
    domainId: status.domainId,
    domainVersion: status.domainVersion,
    state: status.state,
    result: status.result,
    traceId: status.traceId,
    startedAt: toDate(status.startedAtMs),
    completedAt: toDate(status.completedAtMs),
  }
}

export function mapSessionStatusFromJson(json: {
  id: string
  domain_id: string
  domain_version?: string
  state: string
  result?: unknown
  trace_id?: string
  started_at?: string
  completed_at?: string
}): SessionViewModel {
  return {
    id: json.id,
    domainId: json.domain_id,
    domainVersion: json.domain_version || '',
    state: json.state,
    result: typeof json.result === 'string' ? json.result : JSON.stringify(json.result),
    traceId: json.trace_id,
    startedAt: toDate(json.started_at),
    completedAt: toDate(json.completed_at),
  }
}

export function mapObservation(obs: Observation): ObservationViewModel {
  return {
    observationId: obs.observationId,
    traceId: obs.traceId,
    sessionId: obs.sessionId,
    domainId: obs.domainId,
    domainVersion: obs.domainVersion,
    stepId: obs.stepId || undefined,
    agentId: obs.agentId || undefined,
    parentId: obs.parentId || undefined,
    kind: obs.kind,
    role: obs.role || undefined,
    payload: parseJsonField(obs.payload),
    metadata: parseJsonField(obs.metadata),
    createdAt: toDate(obs.createdAtMs),
  }
}

export function mapObservationFromJson(json: Record<string, unknown>): ObservationViewModel {
  return {
    observationId: String(json.observation_id ?? json.observationId ?? ''),
    traceId: String(json.trace_id ?? json.traceId ?? ''),
    sessionId: String(json.session_id ?? json.sessionId ?? ''),
    domainId: String(json.domain_id ?? json.domainId ?? ''),
    domainVersion: String(json.domain_version ?? json.domainVersion ?? ''),
    stepId: json.step_id ? String(json.step_id) : json.stepId ? String(json.stepId) : undefined,
    agentId: json.agent_id ? String(json.agent_id) : json.agentId ? String(json.agentId) : undefined,
    parentId: json.parent_id ? String(json.parent_id) : json.parentId ? String(json.parentId) : undefined,
    kind: String(json.kind ?? ''),
    role: json.role ? String(json.role) : undefined,
    payload: typeof json.payload === 'string' ? parseJsonField(json.payload) : (json.payload as JsonValue) ?? {},
    metadata: typeof json.metadata === 'string' ? parseJsonField(json.metadata) : (json.metadata as JsonValue) ?? {},
    createdAt: toDate((json.created_at_ms ?? json.createdAtMs) as string | number | bigint | undefined),
  }
}

export function extractAgentRefs(observations: ObservationViewModel[] | JsonValue[]): string[] {
  const refs = new Set<string>()
  for (const obs of observations) {
    if (Array.isArray(obs) || typeof obs !== 'object' || obs === null) continue
    const record = obs as Record<string, unknown>
    const agentId = record.agent_id ?? record.agentId
    if (agentId) refs.add(String(agentId))
  }
  return Array.from(refs)
}

export function mapTraceRecord(record: TraceRecord): TraceViewModel {
  const mappedObservations = record.observations.map((o) => mapObservation(o))
  const firstObs = mappedObservations[0]
  const lastObs = mappedObservations[mappedObservations.length - 1]
  const startedAt = firstObs?.createdAt
  const completedAt = lastObs?.createdAt
  const durationMs = startedAt && completedAt ? completedAt.getTime() - startedAt.getTime() : undefined

  return {
    traceId: record.traceId,
    sessionId: record.sessionId,
    domainId: record.domainId,
    domainVersion: record.domainVersion,
    status: 'completed',
    startedAt,
    durationMs,
    agentRefs: extractAgentRefs(mappedObservations),
    observations: record.observations.map((o) => toJson(ObservationSchema, o)),
  }
}

export function mapEvalCase(evalCase: EvalCase): EvalCaseViewModel {
  return {
    id: evalCase.id,
    name: evalCase.name,
    input: evalCase.input,
    expect: evalCase.expect,
  }
}

export function mapEvalSet(evalSet: EvalSet, domainId: string): EvalSetViewModel {
  return {
    id: evalSet.id,
    domainId,
    description: evalSet.description,
    cases: evalSet.cases.map((c) => mapEvalCase(c)),
  }
}

export function mapEvalRunResult(result: EvalRunResult): EvalRunViewModel {
  return {
    id: result.evalRunId,
    domainId: result.domainId,
    setId: result.setId,
    state: result.state,
    score: result.score,
    total: result.total,
    passed: result.passed,
    totalCostUsd: result.totalCostUsd,
    durationMs: Number(result.durationMs),
    baselineRunId: result.baselineRunId,
    cases: result.cases.map((c) => toJson(EvalCaseResultSchema, c)),
  }
}

export function mapAuditRecord(record: AuditRecord): AuditLogViewModel {
  return {
    id: record.recordId,
    timestamp: toDate(record.timestampMs),
    sessionId: record.sessionId,
    domainId: record.domainId,
    action: record.action,
    actor: record.actor,
    resource: record.resource,
    decision: record.decision,
    reason: record.reason,
  }
}

export function harnessToJson(harness: DomainSpec['harness']): JsonValue {
  if (!harness) return {}
  return {
    agents: harness.agents.map((a) => toJson(AgentSchema, a)),
    prompts: harness.prompts.map((p) => toJson(PromptSchema, p)),
    skills: harness.skills.map((s) => toJson(SkillSchema, s)),
    tools: harness.tools.map((t) => toJson(ToolSchema, t)),
    mcps: harness.mcps.map((m) => toJson(MCPConfigSchema, m)),
    sandbox: harness.sandbox ? toJson(SandboxSchema, harness.sandbox) : null,
    policy: harness.policy ? toJson(PolicySchema, harness.policy) : null,
    memory: harness.memory ? toJson(MemorySchema, harness.memory) : null,
    session: harness.session ? toJson(SessionSchema, harness.session) : null,
  }
}
