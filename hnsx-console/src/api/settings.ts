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

export interface Runtime {
  runtime_id: string
  version?: string
  region?: string
  /** 'active' | 'draining' | 'offline' */
  status?: string
  last_heartbeat_at?: string
  /** 可选元数据 */
  capacity?: number
  active_sessions?: number
}

export function listRuntimes(): Promise<{ items: Runtime[]; total: number }> {
  return get<{ items: Runtime[]; total: number }>('/runtimes')
}