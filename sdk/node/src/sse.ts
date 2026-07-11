import { APIError } from './errors.js'
import type { SSEEvent } from './types.js'

export interface StreamSessionEventsOptions {
  baseURL: string
  sessionId: string
  headers?: Record<string, string>
  signal?: AbortSignal
  onEvent?: (event: SSEEvent) => void
  onError?: (error: Error) => void
}

/**
 * streamSessionEvents consumes the SSE stream for a session. It yields each
 * event as it arrives and closes the reader when the signal is aborted or the
 * stream ends.
 */
export async function* streamSessionEvents(
  options: StreamSessionEventsOptions,
): AsyncGenerator<SSEEvent, void, unknown> {
  const url = `${options.baseURL}/api/v1/sessions/${encodeURIComponent(options.sessionId)}/events`
  const response = await fetch(url, {
    headers: {
      Accept: 'text/event-stream',
      ...options.headers,
    },
    signal: options.signal,
  })
  if (!response.ok) {
    throw await parseSSEError(response)
  }
  if (!response.body) {
    return
  }

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let current: Partial<SSEEvent> = {}

  try {
    while (true) {
      if (options.signal?.aborted) {
        return
      }
      const { done, value } = await reader.read()
      if (done) {
        return
      }
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() ?? ''
      for (const line of lines) {
        if (line === '') {
          if (current.name) {
            const ev = { name: current.name, payload: current.payload || '' }
            options.onEvent?.(ev)
            yield ev
          }
          current = {}
          continue
        }
        if (line.startsWith('event: ')) {
          current.name = line.slice(7).trim()
        } else if (line.startsWith('data: ')) {
          current.payload = (current.payload || '') + line.slice(6)
        }
      }
    }
  } finally {
    reader.releaseLock()
  }
}

async function parseSSEError(response: Response): Promise<APIError> {
  let body: { code?: string; message?: string } = {}
  try {
    body = await response.json()
  } catch {
    // ignore
  }
  return new APIError(
    body.code || `HTTP_${response.status}`,
    body.message || response.statusText || 'SSE request failed',
    response.status,
  )
}
