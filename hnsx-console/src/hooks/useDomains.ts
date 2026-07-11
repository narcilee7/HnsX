import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listDomains,
  getDomain,
  getDomainYaml,
  createDomain,
  createDomainYaml,
  updateDomain,
  updateDomainYaml,
  deleteDomain,
  validateDomain,
  validateDomainYaml,
  listDomainVersions,
  getDomainVersion,
} from '@/api/domains'
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

export function useDomainVersions(id: string | undefined) {
  return useQuery({
    queryKey: [...domainKeys.detail(id || ''), 'versions'] as const,
    queryFn: () => listDomainVersions(id!),
    enabled: !!id,
  })
}

export function useDomainVersion(id: string | undefined, version: string | undefined) {
  return useQuery({
    queryKey: [...domainKeys.detail(id || ''), 'version', version || ''] as const,
    queryFn: () => getDomainVersion(id!, version!),
    enabled: !!id && !!version,
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

export function useCreateDomainYaml() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (yaml: string) => createDomainYaml(yaml),
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

export function useUpdateDomainYaml(id: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (yaml: string) => updateDomainYaml(id, yaml),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: domainKeys.detail(id) })
      queryClient.invalidateQueries({ queryKey: domainKeys.lists() })
    },
  })
}

export function useDeleteDomain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteDomain(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: domainKeys.lists() })
    },
  })
}

export function useValidateDomain(id: string) {
  return useMutation({
    mutationFn: (spec: DomainSpec) => validateDomain(id, spec),
  })
}

export function useDomainYaml(id: string | undefined) {
  return useQuery({
    queryKey: [...domainKeys.detail(id || ''), 'yaml'] as const,
    queryFn: () => getDomainYaml(id!),
    enabled: !!id,
  })
}

export function useValidateDomainYaml(id: string) {
  return useMutation({
    mutationFn: (yaml: string) => validateDomainYaml(id, yaml),
  })
}
