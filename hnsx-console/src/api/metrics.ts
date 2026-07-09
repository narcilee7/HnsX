import { get } from './client'

export interface MetricSummary {
  domain_id?: string
  total_sessions?: number
  completed_sessions?: number
  failed_sessions?: number
  total_cost_usd?: number
  avg_duration_ms?: number
  agent_invocations?: number
  tool_invocations?: number
}

export interface AgentMetric {
  agent_id: string
  invocations?: number
  avg_cost_usd?: number
  avg_duration_ms?: number
}

export function getMetrics(params: {
  domain?: string
  from?: string
  to?: string
} = {}): Promise<MetricSummary> {
  const search = new URLSearchParams()
  if (params.domain) search.set('domain', params.domain)
  if (params.from) search.set('from', params.from)
  if (params.to) search.set('to', params.to)
  return get(`/metrics?${search.toString()}`)
}

export function getAgentMetrics(params: { domain?: string } = {}): Promise<{ items: AgentMetric[] }> {
  const search = new URLSearchParams()
  if (params.domain) search.set('domain', params.domain)
  return get(`/metrics/agents?${search.toString()}`)
}
