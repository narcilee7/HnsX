import { useQuery } from '@tanstack/react-query'
import { getMetrics, type MetricSummary } from '@/api/metrics'

const metricsKeys = {
  all: ['metrics'] as const,
  summary: (params: { domain?: string; from?: string; to?: string }) =>
    [...metricsKeys.all, 'summary', params] as const,
}

export function useMetrics(params: { domain?: string; from?: string; to?: string } = {}) {
  return useQuery<MetricSummary>({
    queryKey: metricsKeys.summary(params),
    queryFn: () => getMetrics(params),
    refetchInterval: 5000,
  })
}
