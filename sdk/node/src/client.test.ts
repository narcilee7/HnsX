import { describe, it, expect, beforeAll, afterAll, afterEach } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'
import { HnsXClient, APIError, streamSessionEvents } from './client.js'

const server = setupServer()

beforeAll(() => server.listen({ onUnhandledRequest: 'warn' }))
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

function createSSEResponse(chunks: string[]) {
  const encoder = new TextEncoder()
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk))
      }
      controller.close()
    },
  })
}

describe('HnsXClient', () => {
  const client = new HnsXClient({ baseURL: 'http://localhost:50052' })

  it('lists domains', async () => {
    server.use(
      http.get('*/api/v1/domains', () => {
        return HttpResponse.json({
          items: [{ id: 'customer-service', version: '1.0.0', status: 'active' }],
          total: 1,
        })
      }),
    )
    const res = await client.domains.list()
    expect(res.total).toBe(1)
    expect(res.items[0].id).toBe('customer-service')
  })

  it('gets a domain', async () => {
    server.use(
      http.get('*/api/v1/domains/customer-service', () => {
        return HttpResponse.json({ id: 'customer-service', version: '1.0.0' })
      }),
    )
    const domain = await client.domains.get('customer-service')
    expect(domain.id).toBe('customer-service')
  })

  it('registers a domain from yaml', async () => {
    server.use(
      http.post('*/api/v1/domains', async ({ request }) => {
        const body = await request.text()
        expect(body).toContain('id: customer-service')
        expect(request.headers.get('content-type')).toContain('yaml')
        return HttpResponse.json({ id: 'customer-service', version: '1.0.0' })
      }),
    )
    const domain = await client.domains.registerYaml('id: customer-service\nversion: "1.0.0"')
    expect(domain.id).toBe('customer-service')
  })

  it('triggers a session', async () => {
    server.use(
      http.post('*/api/v1/sessions', async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>
        expect(body.domain_id).toBe('customer-service')
        expect((body.trigger as Record<string, unknown>).question).toBe('hi')
        return HttpResponse.json({ id: 'sess-123', state: 'running' })
      }),
    )
    const session = await client.sessions.trigger({
      domainId: 'customer-service',
      trigger: { question: 'hi' },
    })
    expect(session.id).toBe('sess-123')
    expect(session.state).toBe('running')
  })

  it('lists sessions with query params', async () => {
    server.use(
      http.get('*/api/v1/sessions', ({ request }) => {
        const url = new URL(request.url)
        expect(url.searchParams.get('domain')).toBe('customer-service')
        expect(url.searchParams.get('limit')).toBe('10')
        return HttpResponse.json({ items: [{ id: 's1', state: 'completed' }], total: 1 })
      }),
    )
    const res = await client.sessions.list({ domain: 'customer-service', limit: 10 })
    expect(res.items[0].id).toBe('s1')
  })

  it('approves an approval', async () => {
    server.use(
      http.post('*/api/v1/approvals/approve-1/approve', () => {
        return HttpResponse.json({ id: 'approve-1', status: 'approved' })
      }),
    )
    const approval = await client.approvals.approve('approve-1')
    expect(approval.status).toBe('approved')
  })

  it('throws APIError on failure', async () => {
    server.use(
      http.get('*/api/v1/domains/missing', () => {
        return HttpResponse.json({ code: 'DOMAIN_NOT_FOUND', message: 'not found' }, { status: 404 })
      }),
    )
    await expect(client.domains.get('missing')).rejects.toBeInstanceOf(APIError)
    try {
      await client.domains.get('missing')
    } catch (err) {
      expect((err as APIError).code).toBe('DOMAIN_NOT_FOUND')
      expect((err as APIError).status).toBe(404)
    }
  })

  it('streams session events', async () => {
    server.use(
      http.get('*/api/v1/sessions/sess-123/events', () => {
        return new HttpResponse(
          createSSEResponse([
            'event: observation.text\n',
            'data: {"message":"hello"}\n',
            '\n',
            'event: session.completed\n',
            'data: {}\n',
            '\n',
          ]),
          { headers: { 'content-type': 'text/event-stream' } },
        )
      }),
    )

    const events: { name: string; payload: string }[] = []
    for await (const ev of client.streamSessionEvents('sess-123')) {
      events.push(ev)
    }
    expect(events).toHaveLength(2)
    expect(events[0].name).toBe('observation.text')
    expect(events[1].name).toBe('session.completed')
  })
})

describe('streamSessionEvents', () => {
  it('parses standalone helper', async () => {
    server.use(
      http.get('*/api/v1/sessions/standalone/events', () => {
        return new HttpResponse(
          createSSEResponse(['event: ping\n', 'data: {}\n', '\n']),
          { headers: { 'content-type': 'text/event-stream' } },
        )
      }),
    )
    const events: { name: string; payload: string }[] = []
    for await (const ev of streamSessionEvents({
      baseURL: 'http://localhost:50052',
      sessionId: 'standalone',
    })) {
      events.push(ev)
    }
    expect(events).toHaveLength(1)
    expect(events[0].name).toBe('ping')
  })
})
