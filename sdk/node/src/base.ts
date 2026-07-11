import { APIError, parseAPIError } from './errors.js'

export interface RequestOptions {
  baseURL: string
  headers?: Record<string, string>
}

export class BaseClient {
  constructor(readonly opts: RequestOptions) {}

  protected async _request<T>(
    method: string,
    path: string,
    body?: unknown,
    extraHeaders?: Record<string, string>,
  ): Promise<T> {
    const url = `${this.opts.baseURL}/api/v1${path}`
    const headers: Record<string, string> = {
      Accept: 'application/json',
      ...this.opts.headers,
      ...extraHeaders,
    }
    const init: RequestInit = { method, headers }
    if (body !== undefined) {
      const isYaml = headers['Content-Type']?.includes('yaml')
      if (!headers['Content-Type']) {
        headers['Content-Type'] = 'application/json'
      }
      init.body = isYaml ? String(body) : JSON.stringify(body)
    }

    const response = await fetch(url, init)
    if (!response.ok) {
      throw await parseAPIError(response)
    }
    if (response.status === 204) {
      return undefined as T
    }
    return (await response.json()) as T
  }

  protected _get<T>(path: string): Promise<T> {
    return this._request<T>('GET', path)
  }

  protected _post<T>(path: string, body?: unknown): Promise<T> {
    return this._request<T>('POST', path, body)
  }

  protected _put<T>(path: string, body?: unknown): Promise<T> {
    return this._request<T>('PUT', path, body)
  }

  protected _delete<T>(path: string): Promise<T> {
    return this._request<T>('DELETE', path)
  }

  protected _queryString(params: object): string {
    const qs = new URLSearchParams()
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== '') {
        qs.set(key, String(value))
      }
    }
    const s = qs.toString()
    return s ? `?${s}` : ''
  }
}

export { APIError, parseAPIError }
