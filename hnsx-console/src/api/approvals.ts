import { get, post } from './client'

export interface Approval {
  id: string
  session_id: string
  step_id?: string
  requested_action: string
  risk_description?: string
  status: 'pending' | 'approved' | 'rejected'
  created_at: string
  resolved_at?: string
  resolver?: string
}

export interface ApprovalListParams {
  status?: string
  limit?: number
  offset?: number
}

export function listApprovals(params: ApprovalListParams = {}): Promise<{
  items: Approval[]
  total: number
}> {
  const search = new URLSearchParams()
  if (params.status) search.set('status', params.status)
  if (params.limit !== undefined) search.set('limit', String(params.limit))
  if (params.offset !== undefined) search.set('offset', String(params.offset))
  return get(`/approvals?${search.toString()}`)
}

export function approveApproval(id: string, comment?: string): Promise<void> {
  return post(`/approvals/${id}/approve`, { comment })
}

export function rejectApproval(id: string, comment?: string): Promise<void> {
  return post(`/approvals/${id}/reject`, { comment })
}
