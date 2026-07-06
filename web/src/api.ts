const API_PREFIX = '/api/v1';

export interface Domain {
  id: string;
  version: string;
  yaml_body: string;
}

export interface InstanceInfo {
  instance_id: string;
  domain_id: string;
  tags: string[];
  region: string;
  capabilities: string[];
}

export interface TraceRecord {
  session_id: string;
  domain_id: string;
  step_id: string;
  agent_id: string;
  started_at_ms: number;
  duration_ms: number;
  input: string;
  output: string;
}

export interface InvocationMetrics {
  domain_id: string;
  invocation_count: number;
  total_cost_usd: number;
  total_prompt_tokens: number;
  total_completion_tokens: number;
  avg_latency_ms: number;
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`${API_PREFIX}${path}`);
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${await res.text()}`);
  }
  return res.json();
}

export const api = {
  listDomains: () => get<Domain[]>('/domains'),
  listInstances: (domainId: string) => get<InstanceInfo[]>(`/instances/${encodeURIComponent(domainId)}`),
  listTraces: (domainId: string) => get<TraceRecord[]>(`/traces/${encodeURIComponent(domainId)}`),
  getMetrics: (domainId: string) => get<InvocationMetrics>(`/metrics/${encodeURIComponent(domainId)}`),
  getPrometheus: () => fetch('/metrics').then((r) => r.text()),
};
