import { BaseClient } from './base.js'
import type {
  Approval,
  Domain,
  DomainSummary,
  EvalCase,
  EvalRun,
  EvalSet,
  EvalSetSummary,
  ListEnvelope,
  Session,
  SessionSummary,
  Trace,
  TraceSummary,
} from './types.js'

export interface ListDomainsParams {
  limit?: number
  offset?: number
}

export class DomainRegistryClient extends BaseClient {
  list(params: ListDomainsParams = {}): Promise<ListEnvelope<DomainSummary>> {
    return this._get<ListEnvelope<DomainSummary>>(`/domains${this._queryString(params)}`)
  }

  get(id: string): Promise<Domain> {
    return this._get<Domain>(`/domains/${encodeURIComponent(id)}`)
  }

  getYaml(id: string): Promise<string> {
    return this._request<string>('GET', `/domains/${encodeURIComponent(id)}/yaml`, undefined, {
      Accept: 'application/yaml',
    })
  }

  registerYaml(yaml: string): Promise<Domain> {
    return this._request<Domain>('POST', '/domains', yaml, {
      'Content-Type': 'application/x-yaml',
    })
  }

  updateYaml(id: string, yaml: string): Promise<Domain> {
    return this._request<Domain>('PUT', `/domains/${encodeURIComponent(id)}`, yaml, {
      'Content-Type': 'application/x-yaml',
    })
  }

  remove(id: string): Promise<void> {
    return this._delete<void>(`/domains/${encodeURIComponent(id)}`)
  }

  validateYaml(id: string, yaml: string): Promise<{ valid: boolean; errors?: string[] }> {
    return this._request<{ valid: boolean; errors?: string[] }>(
      'POST',
      `/domains/${encodeURIComponent(id)}/validate`,
      yaml,
      { 'Content-Type': 'application/x-yaml' },
    )
  }
}

export interface TriggerSessionOptions {
  domainId: string
  trigger?: Record<string, unknown>
}

export interface ListSessionsParams {
  domain?: string
  state?: string
  limit?: number
  offset?: number
}

export class SessionsClient extends BaseClient {
  list(params: ListSessionsParams = {}): Promise<ListEnvelope<SessionSummary>> {
    return this._get<ListEnvelope<SessionSummary>>(`/sessions${this._queryString(params)}`)
  }

  get(id: string): Promise<Session> {
    return this._get<Session>(`/sessions/${encodeURIComponent(id)}`)
  }

  trigger(options: TriggerSessionOptions): Promise<Session> {
    return this._post<Session>('/sessions', {
      domain_id: options.domainId,
      trigger: options.trigger ?? {},
    })
  }

  cancel(id: string): Promise<Session> {
    return this._post<Session>(`/sessions/${encodeURIComponent(id)}/cancel`)
  }

  rerun(id: string): Promise<Session> {
    return this._post<Session>(`/sessions/${encodeURIComponent(id)}/rerun`)
  }
}

export interface ListTracesParams {
  domain?: string
  session?: string
  agent?: string
  from?: string
  to?: string
  limit?: number
  offset?: number
}

export class TracesClient extends BaseClient {
  list(params: ListTracesParams = {}): Promise<ListEnvelope<TraceSummary>> {
    return this._get<ListEnvelope<TraceSummary>>(`/traces${this._queryString(params)}`)
  }

  get(traceId: string): Promise<Trace> {
    return this._get<Trace>(`/traces/${encodeURIComponent(traceId)}`)
  }
}

export interface ListApprovalsParams {
  domain?: string
  session?: string
  status?: string
}

export class ApprovalsClient extends BaseClient {
  list(params: ListApprovalsParams = {}): Promise<ListEnvelope<Approval>> {
    return this._get<ListEnvelope<Approval>>(`/approvals${this._queryString(params)}`)
  }

  get(id: string): Promise<Approval> {
    return this._get<Approval>(`/approvals/${encodeURIComponent(id)}`)
  }

  approve(id: string): Promise<Approval> {
    return this._post<Approval>(`/approvals/${encodeURIComponent(id)}/approve`)
  }

  reject(id: string): Promise<Approval> {
    return this._post<Approval>(`/approvals/${encodeURIComponent(id)}/reject`)
  }
}

export interface CreateEvalSetOptions {
  setId: string
  domainId: string
  description?: string
  cases: EvalCase[]
}

export class EvalsClient extends BaseClient {
  listSets(): Promise<ListEnvelope<EvalSetSummary>> {
    return this._get<ListEnvelope<EvalSetSummary>>('/evals')
  }

  getSet(id: string): Promise<EvalSet> {
    return this._get<EvalSet>(`/evals/${encodeURIComponent(id)}`)
  }

  createSet(options: CreateEvalSetOptions): Promise<EvalSet> {
    return this._post<EvalSet>('/evals', {
      set_id: options.setId,
      domain_id: options.domainId,
      description: options.description,
      cases: options.cases,
    })
  }

  updateSet(
    id: string,
    options: { description?: string; cases: EvalCase[] },
  ): Promise<EvalSet> {
    return this._put<EvalSet>(`/evals/${encodeURIComponent(id)}`, {
      description: options.description,
      cases: options.cases,
    })
  }

  removeSet(id: string): Promise<void> {
    return this._delete<void>(`/evals/${encodeURIComponent(id)}`)
  }

  runSet(id: string): Promise<{ run_id: string; state: string }> {
    return this._post<{ run_id: string; state: string }>(`/evals/${encodeURIComponent(id)}/run`)
  }

  listRuns(setId: string): Promise<ListEnvelope<EvalRun>> {
    return this._get<ListEnvelope<EvalRun>>(`/evals/${encodeURIComponent(setId)}/runs`)
  }

  getRun(setId: string, runId: string): Promise<EvalRun> {
    return this._get<EvalRun>(
      `/evals/${encodeURIComponent(setId)}/runs/${encodeURIComponent(runId)}`,
    )
  }
}
