import {
  ApprovalsClient,
  DomainRegistryClient,
  EvalsClient,
  SessionsClient,
  TracesClient,
} from './resources.js'
import { streamSessionEvents } from './sse.js'
import type { StreamSessionEventsOptions } from './sse.js'

export interface HnsXClientOptions {
  baseURL?: string
  headers?: Record<string, string>
}

export const DEFAULT_BASE_URL = 'http://127.0.0.1:50052'

/**
 * HnsXClient is the entry point for the Node/TypeScript SDK. It exposes
 * resource-oriented sub-clients that talk to the HnsX REST API.
 */
export class HnsXClient {
  readonly domains: DomainRegistryClient
  readonly sessions: SessionsClient
  readonly traces: TracesClient
  readonly approvals: ApprovalsClient
  readonly evals: EvalsClient

  constructor(options: HnsXClientOptions = {}) {
    const baseURL = (options.baseURL || DEFAULT_BASE_URL).replace(/\/$/, '')
    const opts = { baseURL, headers: options.headers }
    this.domains = new DomainRegistryClient(opts)
    this.sessions = new SessionsClient(opts)
    this.traces = new TracesClient(opts)
    this.approvals = new ApprovalsClient(opts)
    this.evals = new EvalsClient(opts)
  }

  /**
   * streamSessionEvents consumes the live SSE event stream for a session.
   */
  streamSessionEvents(
    sessionId: string,
    options?: Omit<StreamSessionEventsOptions, 'baseURL' | 'sessionId' | 'headers'>,
  ): AsyncGenerator<import('./types.js').SSEEvent, void, unknown> {
    return streamSessionEvents({
      baseURL: this.sessions.opts.baseURL,
      sessionId,
      headers: this.sessions.opts.headers,
      ...options,
    })
  }
}

export { streamSessionEvents }
export * from './errors.js'
export * from './types.js'
export * from './resources.js'
