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