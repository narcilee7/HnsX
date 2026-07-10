import { get, post } from './client'

/**
 * Approval as returned by the server's /api/v1/approvals endpoint.
 * Matches the server `internal/approval/model.Approval` JSON shape.
 */
export interface Approval {
  id: string
  session_id: string
  domain_id: string
  action: string
  resource: string
  risk_level: 'low' | 'medium' | 'high' | 'critical'
  context: Record<string, unknown>
  status: 'pending' | 'approved' | 'rejected' | 'expired'
  requested_by: string
  reviewed_by?: string
  comment?: string
  created_at: string
  updated_at: string
  resolved_at?: string
}

export interface ApprovalListParams {
  status?: string
  domain?: string
  session?: string
  limit?: number
  offset?: number
}

export function listApprovals(
  params: ApprovalListParams = {},
): Promise<{ items: Approval[]; total: number }> {
  const search = new URLSearchParams()
  if (params.status) search.set('status', params.status)
  if (params.domain) search.set('domain', params.domain)
  if (params.session) search.set('session', params.session)
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.offset !== undefined) search.set('offset', String(params.offset))
  return get<{ items: Approval[]; total: number }>(`/approvals?${search.toString()}`)
}

export function getApproval(id: string): Promise<Approval> {
  return get<Approval>(`/approvals/${id}`)
}

interface DecisionBody {
  reviewed_by?: string
  comment?: string
}

export function approveApproval(id: string, body: DecisionBody = {}): Promise<Approval> {
  return post<Approval>(`/approvals/${id}/approve`, body)
}

export function rejectApproval(id: string, body: DecisionBody = {}): Promise<Approval> {
  return post<Approval>(`/approvals/${id}/reject`, body)
}
