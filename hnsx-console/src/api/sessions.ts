import { get, post } from './client'
import { mapSessionStatusFromJson } from './mappers'
import type { SessionViewModel } from './mappers'

export interface SessionListParams {
  domain?: string
  state?: string
  limit?: number
  offset?: number
}

export function listSessions(params: SessionListParams = {}): Promise<{
  items: SessionViewModel[]
  total: number
}> {
  const search = new URLSearchParams()
  if (params.domain) search.set('domain', params.domain)
  if (params.state) search.set('state', params.state)
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.offset !== undefined) search.set('offset', String(params.offset))
  return get<unknown>(`/sessions?${search.toString()}`).then((res) => {
    const data = res as { items: unknown[]; total: number }
    return {
      items: data.items.map((item) =>
        mapSessionStatusFromJson(item as Parameters<typeof mapSessionStatusFromJson>[0]),
      ),
      total: data.total,
    }
  })
}

export function getSession(id: string): Promise<SessionViewModel> {
  return get<unknown>(`/sessions/${id}`).then((res) => mapSessionStatusFromJson(res as Parameters<typeof mapSessionStatusFromJson>[0]))
}

export function createSession(body: {
  domain_id: string
  domain_version?: string
  trigger?: Record<string, unknown>
}): Promise<{ id: string; state: string }> {
  return post('/sessions', body)
}

export function rerunSession(id: string): Promise<{ id: string; state: string }> {
  return post(`/sessions/${id}/rerun`)
}

export function cancelSession(id: string): Promise<void> {
  return post(`/sessions/${id}/cancel`)
}

export function getSessionTrace(id: string): Promise<unknown> {
  return get(`/sessions/${id}/trace`)
}
