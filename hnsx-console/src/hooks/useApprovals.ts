import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listApprovals,
  approveApproval,
  rejectApproval,
} from '@/api/approvals'
import type { ApprovalListParams } from '@/api/approvals'

const approvalKeys = {
  all: ['approvals'] as const,
  lists: () => [...approvalKeys.all, 'list'] as const,
  list: (params: ApprovalListParams) => [...approvalKeys.lists(), params] as const,
}

export function useApprovals(params: ApprovalListParams = {}) {
  return useQuery({
    queryKey: approvalKeys.list(params),
    queryFn: () => listApprovals(params),
    refetchInterval: 5000,
  })
}

interface ResolveInput {
  id: string
  decision: 'approve' | 'reject'
  reviewed_by?: string
  comment?: string
}

export function useResolveApproval() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, decision, reviewed_by, comment }: ResolveInput) => {
      const body = { reviewed_by, comment }
      if (decision === 'approve') {
        return approveApproval(id, body)
      }
      return rejectApproval(id, body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: approvalKeys.lists() })
    },
  })
}
