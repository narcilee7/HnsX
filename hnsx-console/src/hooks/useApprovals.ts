import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { listApprovals, approveApproval, rejectApproval } from '@/api/approvals'
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

export function useResolveApproval() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, decision, comment }: { id: string; decision: 'approve' | 'reject'; comment?: string }) => {
      if (decision === 'approve') {
        return approveApproval(id, comment)
      }
      return rejectApproval(id, comment)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: approvalKeys.lists() })
    },
  })
}
