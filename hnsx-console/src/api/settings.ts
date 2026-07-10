import { get, post, put, del } from './client'

// ---------- Secrets ----------

/**
 * Secret metadata returned by /api/v1/secrets. The plaintext value is
 * never carried on the wire — operators verify a typed value against
 * the last-4 fingerprint instead. The CLI / SDK calls a separate
 * resolve path that the audit log can attribute.
 */
export interface Secret {
  name: string
  description?: string
  kind?: string
  fingerprint: string
  created_at?: string
  updated_at?: string
}

export function listSecrets(): Promise<{ items: Secret[]; total: number }> {
  return get<{ items: Secret[]; total: number }>('/secrets')
}

export function createSecret(body: {
  name: string
  value: string
  description?: string
  kind?: string
}): Promise<void> {
  return post('/secrets', body)
}

export function updateSecret(name: string, body: { value: string; description?: string; kind?: string }): Promise<void> {
  return put(`/secrets/${name}`, body)
}

export function deleteSecret(name: string): Promise<void> {
  return del(`/secrets/${name}`)
}

// ---------- Policies ----------

export interface PolicyBudget {
  max_cost_usd?: number
  max_turns?: number
  max_tokens?: number
}

export interface PolicyPermissions {
  allow_file_write?: boolean
  allow_file_delete?: boolean
  allow_network?: boolean
  allow_shell?: boolean
}

export interface PolicyGuardrail {
  id?: string
  type?: string
  on?: string
  action?: string
  schema?: string
  message?: string
  config?: unknown
}

/**
 * Named policy returned by /api/v1/policies. A policy is a NAMED bundle
 * (Budget + Permissions + Guardrails) that can be bound to a domain
 * via POST /api/v1/domains/:id/policies.
 */
export interface Policy {
  id: string
  name?: string
  description?: string
  bound_domain?: string
  budget?: PolicyBudget
  permissions?: PolicyPermissions
  guardrails?: PolicyGuardrail[]
  created_at?: string
  updated_at?: string
}

export interface PolicyWriteBody {
  id?: string
  name?: string
  description?: string
  budget?: PolicyBudget
  permissions?: PolicyPermissions
  guardrails?: PolicyGuardrail[]
}

export function listPolicies(): Promise<{ items: Policy[]; total: number }> {
  return get<{ items: Policy[]; total: number }>('/policies')
}

export function createPolicy(body: PolicyWriteBody): Promise<Policy> {
  return post<Policy>('/policies', body)
}

export function updatePolicy(id: string, body: PolicyWriteBody): Promise<Policy> {
  return put<Policy>(`/policies/${id}`, body)
}

export function deletePolicy(id: string): Promise<void> {
  return del(`/policies/${id}`)
}

export interface BindPolicyBody {
  policy_id: string
}

export function bindPolicy(domainID: string, body: BindPolicyBody): Promise<void> {
  return post(`/domains/${domainID}/policies`, body)
}

// ---------- Runtimes ----------

/**
 * Runtime worker snapshot. Server reads live worker.Registry.List() — the
 * worker only appears here once it has called Register() and at least one
 * Heartbeat() has landed.
 */
export interface Runtime {
  runtime_id: string
  version?: string
  region?: string
  hostname?: string
  pid?: string
  /** 'healthy' | 'degraded' | 'offline' — derived from heart-beat freshness */
  status?: string
  last_heartbeat_at?: string
  age_seconds?: number
  healthy?: boolean
  /** advertised by the worker on Register via ResourceCapacity */
  capacity?: number
  active_sessions?: number
  capabilities?: string[]
  models?: string[]
  sandbox_runtimes?: string[]
  labels?: Record<string, string>
}

export function listRuntimes(): Promise<{ items: Runtime[]; total: number }> {
  return get<{ items: Runtime[]; total: number }>('/runtimes')
}