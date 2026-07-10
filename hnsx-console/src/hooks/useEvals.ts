import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { create, EvalCaseSchema } from '@hnsx/sdk-node'
import {
  listEvalSets,
  getEvalSet,
  createEvalSet,
  runEval,
  getEvalRun,
  type EvalSetListItem,
} from '@/api/evals'
import type { EvalSetViewModel, EvalRunViewModel } from '@/api/mappers'

const evalKeys = {
  all: ['evals'] as const,
  lists: () => [...evalKeys.all, 'list'] as const,
  detail: (setId: string) => [...evalKeys.all, 'detail', setId] as const,
  run: (setId: string, runId: string) =>
    [...evalKeys.detail(setId), 'run', runId] as const,
}

/**
 * EvalSets are server-global, not per-domain (the route is /api/v1/evals,
 * not /api/v1/domains/:id/evals). The page filters by domain client-side
 * from the dataset's domain_id field.
 */
export function useEvalSets() {
  return useQuery<{ items: EvalSetListItem[]; total: number }>({
    queryKey: evalKeys.lists(),
    queryFn: () => listEvalSets({ limit: 100 }),
  })
}

export function useEvalSet(setId: string | undefined) {
  return useQuery<EvalSetViewModel>({
    queryKey: evalKeys.detail(setId || ''),
    queryFn: () => getEvalSet(setId!),
    enabled: !!setId,
  })
}

export interface CreateEvalSetInput {
  set_id: string
  domain_id: string
  description?: string
  cases: Array<{
    id: string
    name?: string
    input: unknown
    expect: unknown
    scorer?: unknown
  }>
}

export function useCreateEvalSet() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateEvalSetInput) =>
      createEvalSet({
        set_id: input.set_id,
        domain_id: input.domain_id,
        description: input.description,
        cases: input.cases.map((c) =>
          create(EvalCaseSchema, {
            id: c.id,
            name: c.name ?? c.id,
            input: JSON.stringify(c.input ?? {}),
            expect: JSON.stringify(c.expect ?? {}),
            scorer: c.scorer ? { scorers: [] } : undefined,
          }),
        ),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: evalKeys.lists() }),
  })
}

export function useRunEval(setId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { orchestration?: string; baseline_run_id?: string } = {}) =>
      runEval(setId, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: evalKeys.detail(setId) }),
  })
}

export function useEvalRun(setId: string | undefined, runId: string | undefined) {
  return useQuery<EvalRunViewModel>({
    queryKey: evalKeys.run(setId || '', runId || ''),
    queryFn: () => getEvalRun(setId!, runId!),
    enabled: !!setId && !!runId,
  })
}
