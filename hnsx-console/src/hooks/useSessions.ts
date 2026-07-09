import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { listSessions, getSession, createSession, getSessionTrace } from '@/api/sessions'
import { mapObservationFromJson, type ObservationViewModel } from '@/api/mappers'

const sessionKeys = {
  all: ['sessions'] as const,
  lists: () => [...sessionKeys.all, 'list'] as const,
  list: (params: { domain?: string; state?: string; limit?: number; offset?: number }) =>
    [...sessionKeys.lists(), params] as const,
  details: () => [...sessionKeys.all, 'detail'] as const,
  detail: (id: string) => [...sessionKeys.details(), id] as const,
}

export function useSessions(params: { domain?: string; state?: string; limit?: number; offset?: number } = {}) {
  return useQuery({
    queryKey: sessionKeys.list(params),
    queryFn: () => listSessions(params),
    refetchInterval: 5000,
  })
}

export function useSession(id: string | undefined) {
  return useQuery({
    queryKey: sessionKeys.detail(id || ''),
    queryFn: () => getSession(id!),
    enabled: !!id,
    refetchInterval: 5000,
  })
}

export function useCreateSession() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: { domain_id: string; domain_version?: string; trigger?: Record<string, unknown> }) =>
      createSession(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.lists() })
    },
  })
}

export interface SessionEventState {
  observations: ObservationViewModel[]
  state: string | null
  connected: boolean
  error: Error | null
}

export function useSessionEvents(id: string | undefined) {
  const [events, setEvents] = useState<SessionEventState>({
    observations: [],
    state: null,
    connected: false,
    error: null,
  })
  const reconnectAttemptRef = useRef(0)
  const abortRef = useRef<(() => void) | null>(null)

  useEffect(() => {
    if (!id) return

    let cancelled = false
    reconnectAttemptRef.current = 0

    async function loadHistory() {
      try {
        const trace = await getSessionTrace(id!)
        if (cancelled) return
        const obsList = Array.isArray((trace as { observations?: unknown[] })?.observations)
          ? ((trace as { observations: unknown[] }).observations as Record<string, unknown>[])
          : []
        setEvents((prev) => ({
          ...prev,
          observations: obsList.map((o) => mapObservationFromJson(o)),
        }))
      } catch {
        // ignore history errors; SSE will still try to connect
      }
    }

    function connect() {
      if (cancelled) return
      const source = new EventSource(`/api/v1/sessions/${id}/events`)

      function cleanup() {
        source.close()
      }
      abortRef.current = cleanup

      source.addEventListener('open', () => {
        if (cancelled) return
        reconnectAttemptRef.current = 0
        setEvents((prev) => ({ ...prev, connected: true, error: null }))
      })

      source.addEventListener('observation', (event) => {
        try {
          const data = JSON.parse(event.data) as Record<string, unknown>
          const mapped = mapObservationFromJson(data)
          setEvents((prev) => ({
            ...prev,
            observations: [...prev.observations, mapped],
          }))
        } catch {
          // ignore malformed events
        }
      })

      source.addEventListener('state', (event) => {
        try {
          const data = JSON.parse(event.data) as { state?: string }
          setEvents((prev) => ({ ...prev, state: data.state || null }))
        } catch {
          // ignore malformed events
        }
      })

      source.addEventListener('error', (event) => {
        if (cancelled) return
        const message = (event as ErrorEvent).message || 'SSE connection error'
        setEvents((prev) => ({ ...prev, connected: false, error: new Error(message) }))
        source.close()
        scheduleReconnect()
      })

      source.addEventListener('done', () => {
        if (cancelled) return
        setEvents((prev) => ({ ...prev, connected: false }))
        source.close()
      })
    }

    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    function scheduleReconnect() {
      if (cancelled) return
      if (reconnectAttemptRef.current >= 5) return
      reconnectAttemptRef.current += 1
      const delay = Math.min(1000 * 2 ** reconnectAttemptRef.current, 30000)
      reconnectTimer = setTimeout(() => {
        if (!cancelled) connect()
      }, delay)
    }

    setEvents({ observations: [], state: null, connected: false, error: null })
    loadHistory()
    connect()

    return () => {
      cancelled = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (abortRef.current) abortRef.current()
    }
  }, [id])

  return events
}
