import { useQuery } from '@tanstack/react-query'
import { listAuditLogs } from '@/api/audit'
import type { AuditListParams } from '@/api/audit'

const auditKeys = {
  all: ['audit'] as const,
  lists: () => [...auditKeys.all, 'list'] as const,
  list: (params: AuditListParams) => [...auditKeys.lists(), params] as const,
}

export function useAuditLogs(params: AuditListParams = {}) {
  return useQuery({
    queryKey: auditKeys.list(params),
    queryFn: () => listAuditLogs(params),
  })
}
