import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listSecrets,
  createSecret,
  updateSecret,
  deleteSecret,
  listPolicies,
  listRuntimes,
} from '@/api/settings'
import type { Secret } from '@/api/settings'

const keys = {
  secrets: ['settings', 'secrets'] as const,
  policies: ['settings', 'policies'] as const,
  runtimes: ['settings', 'runtimes'] as const,
}

export function useSecrets() {
  return useQuery({ queryKey: keys.secrets, queryFn: listSecrets })
}

export function useCreateSecret() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: Parameters<typeof createSecret>[0]) => createSecret(body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.secrets }),
  })
}

export function useUpdateSecret() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Parameters<typeof updateSecret>[1] }) =>
      updateSecret(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.secrets }),
  })
}

export function useDeleteSecret() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteSecret(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.secrets }),
  })
}

export function usePolicies() {
  return useQuery({ queryKey: keys.policies, queryFn: listPolicies })
}

export function useRuntimes() {
  return useQuery({
    queryKey: keys.runtimes,
    queryFn: listRuntimes,
    refetchInterval: 10_000, // 10s — heartbeats
  })
}

export type { Secret }