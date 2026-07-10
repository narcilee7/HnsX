import { get, post, put, del } from './client'

// ---------- Secrets ----------

export interface Secret {
  id: string
  name?: string
  /** Provider — e.g. 'env' / 'vault' / 'aws-sm' — never the value itself */
  provider?: string
  scope?: string
  created_at?: string
  updated_at?: string
  /** last 4 chars for verification only, value never returned */
  fingerprint?: string
}

export function listSecrets(): Promise<{ items: Secret[]; total: number }> {
  return get<{ items: Secret[]; total: number }>('/secrets')
}

export function createSecret(body: { id: string; value: string; provider?: string; scope?: string }): Promise<void> {
  return post('/secrets', body)
}

export function updateSecret(id: string, body: { value: string; provider?: string }): Promise<void> {
  return put(`/secrets/${id}`, body)
}

export function deleteSecret(id: string): Promise<void> {
  return del(`/secrets/${id}`)
}

// ---------- Policies ----------

export interface Policy {
  id: string
  name?: string
  /** rule body — 通常是 YAML/JSON 字符串 */
  rule?: string
  /** scope — 例如 'global' / 'domain:customer-service' */
  scope?: string
  /** effect — 'allow' / 'deny' / 'require_approval' */
  effect?: string
  description?: string
  created_at?: string
  updated_at?: string
}

export function listPolicies(): Promise<{ items: Policy[]; total: number }> {
  return get<{ items: Policy[]; total: number }>('/policies')
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