import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listDomains, getDomain, createDomain, updateDomain, validateDomain } from '@/api/domains'
import type { DomainSpec } from '@hnsx/sdk-node'

const domainKeys = {
  all: ['domains'] as const,
  lists: () => [...domainKeys.all, 'list'] as const,
  list: (params: { limit?: number; offset?: number }) => [...domainKeys.lists(), params] as const,
  details: () => [...domainKeys.all, 'detail'] as const,
  detail: (id: string) => [...domainKeys.details(), id] as const,
}

export function useDomains(params: { limit?: number; offset?: number } = {}) {
  return useQuery({
    queryKey: domainKeys.list(params),
    queryFn: () => listDomains(params),
  })
}

export function useDomain(id: string | undefined) {
  return useQuery({
    queryKey: domainKeys.detail(id || ''),
    queryFn: () => getDomain(id!),
    enabled: !!id,
  })
}

export function useCreateDomain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (spec: DomainSpec) => createDomain(spec),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: domainKeys.lists() })
    },
  })
}

export function useUpdateDomain(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (spec: DomainSpec) => updateDomain(id, spec),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: domainKeys.detail(id) })
      queryClient.invalidateQueries({ queryKey: domainKeys.lists() })
    },
  })
}

export function useValidateDomain(id: string) {
  return useMutation({
    mutationFn: (harness: unknown) => validateDomain(id, harness),
  })
}
