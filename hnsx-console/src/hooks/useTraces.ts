import { useQuery } from '@tanstack/react-query'
import { listTraces, getTrace } from '@/api/traces'
import type { TraceListParams } from '@/api/traces'

const traceKeys = {
  all: ['traces'] as const,
  lists: () => [...traceKeys.all, 'list'] as const,
  list: (params: TraceListParams) => [...traceKeys.lists(), params] as const,
  details: () => [...traceKeys.all, 'detail'] as const,
  detail: (id: string) => [...traceKeys.details(), id] as const,
}

export function useTraces(params: TraceListParams = {}) {
  return useQuery({
    queryKey: traceKeys.list(params),
    queryFn: () => listTraces(params),
  })
}

export function useTrace(id: string | undefined) {
  return useQuery({
    queryKey: traceKeys.detail(id || ''),
    queryFn: () => getTrace(id!),
    enabled: !!id,
  })
}
