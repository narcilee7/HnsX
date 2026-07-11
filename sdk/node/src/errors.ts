/**
 * APIError is the canonical error envelope returned by the HnsX server.
 */
export class APIError extends Error {
  constructor(
    readonly code: string,
    message: string,
    readonly status: number,
    readonly details?: Record<string, unknown>,
  ) {
    super(message)
    this.name = 'APIError'
  }

  toJSON() {
    return {
      code: this.code,
      message: this.message,
      status: this.status,
      details: this.details,
    }
  }
}

export async function parseAPIError(response: Response): Promise<APIError> {
  let body: { code?: string; message?: string; details?: Record<string, unknown> } = {}
  try {
    body = await response.json()
  } catch {
    // ignore
  }
  return new APIError(
    body.code || `HTTP_${response.status}`,
    body.message || response.statusText || 'Request failed',
    response.status,
    body.details,
  )
}
