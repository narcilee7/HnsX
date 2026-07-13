import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ApiError, request, get, post, put, del, getText } from './client'

const mockToken = 'test-token'

vi.mock('@/stores/authStore', () => ({
  useAuthStore: {
    getState: vi.fn(() => ({ token: mockToken })),
  },
}))

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
  },
}))

describe('ApiError', () => {
  it('preserves code, message, status and details', () => {
    const error = new ApiError('DOMAIN_NOT_FOUND', 'Domain not found', 404, { domainId: 'x' })
    expect(error.code).toBe('DOMAIN_NOT_FOUND')
    expect(error.message).toBe('Domain not found')
    expect(error.status).toBe(404)
    expect(error.details).toEqual({ domainId: 'x' })
    expect(error.name).toBe('ApiError')
  })
})

describe('request', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns parsed JSON on success', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({ items: [] }),
    }))

    const result = await request<{ items: unknown[] }>('GET', '/domains')
    expect(result).toEqual({ items: [] })
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/domains',
      expect.objectContaining({
        method: 'GET',
        headers: expect.objectContaining({
          Authorization: 'Bearer test-token',
          'Content-Type': 'application/json',
        }),
      }),
    )
  })

  it('returns undefined on 204 No Content', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
    }))

    const result = await request<undefined>('DELETE', '/domains/x')
    expect(result).toBeUndefined()
  })

  it('throws ApiError and shows toast on HTTP error', async () => {
    const { toast } = await import('sonner')
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 422,
      statusText: 'Unprocessable Entity',
      json: vi.fn().mockResolvedValue({ code: 'VALIDATION_FAILED', message: 'Invalid YAML', details: { line: 3 } }),
    }))

    await expect(request('POST', '/domains', {})).rejects.toThrow(ApiError)
    await expect(request('POST', '/domains', {})).rejects.toThrow('Invalid YAML')
    expect(toast.error).toHaveBeenCalledWith('Invalid YAML')
  })

  it('falls back to statusText when body lacks message', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      json: vi.fn().mockRejectedValue(new Error('parse failed')),
    }))

    await expect(request('GET', '/metrics')).rejects.toThrow('Internal Server Error')
  })
})

describe('HTTP helpers', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: vi.fn().mockResolvedValue({}),
    }))
  })

  it('get uses GET method', async () => {
    await get('/domains')
    expect(fetch).toHaveBeenCalledWith('/api/v1/domains', expect.objectContaining({ method: 'GET' }))
  })

  it('post uses POST method', async () => {
    await post('/domains', { id: 'x' })
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/domains',
      expect.objectContaining({ method: 'POST', body: JSON.stringify({ id: 'x' }) }),
    )
  })

  it('put uses PUT method', async () => {
    await put('/domains/x', { id: 'x' })
    expect(fetch).toHaveBeenCalledWith(
      '/api/v1/domains/x',
      expect.objectContaining({ method: 'PUT', body: JSON.stringify({ id: 'x' }) }),
    )
  })

  it('del uses DELETE method', async () => {
    await del('/domains/x')
    expect(fetch).toHaveBeenCalledWith('/api/v1/domains/x', expect.objectContaining({ method: 'DELETE' }))
  })

  it('getText returns text response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      text: vi.fn().mockResolvedValue('yaml: content'),
    }))

    const text = await getText('/domains/x/yaml')
    expect(text).toBe('yaml: content')
  })
})
