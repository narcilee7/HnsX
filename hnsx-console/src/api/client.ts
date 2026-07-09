import { toast } from 'sonner'

export const API_BASE = '/api/v1'

export class ApiError extends Error {
  code: string
  status: number
  details?: Record<string, unknown>

  constructor(code: string, message: string, status: number, details?: Record<string, unknown>) {
    super(message)
    this.code = code
    this.status = status
    this.details = details
    this.name = 'ApiError'
  }
}

async function parseError(response: Response): Promise<ApiError> {
  let body: { code?: string; message?: string; details?: Record<string, unknown> } = {}
  try {
    body = await response.json()
  } catch {
    // ignore
  }
  return new ApiError(
    body.code || `HTTP_${response.status}`,
    body.message || response.statusText || 'Request failed',
    response.status,
    body.details,
  )
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const url = `${API_BASE}${path}`
  const init: RequestInit = {
    method,
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/json',
    },
  }
  if (body !== undefined) {
    init.body = JSON.stringify(body)
  }

  const response = await fetch(url, init)
  if (!response.ok) {
    const error = await parseError(response)
    toast.error(error.message)
    throw error
  }
  if (response.status === 204) {
    return undefined as T
  }
  return response.json() as Promise<T>
}

export function get<T>(path: string) {
  return request<T>('GET', path)
}

export function post<T>(path: string, body?: unknown) {
  return request<T>('POST', path, body)
}

export function put<T>(path: string, body?: unknown) {
  return request<T>('PUT', path, body)
}

export function del<T>(path: string) {
  return request<T>('DELETE', path)
}
