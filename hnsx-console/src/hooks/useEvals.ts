import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { create, type EvalSet, EvalSetSchema, EvalCaseSchema } from '@hnsx/sdk-node'
import {
  listEvalSets,
  getEvalSet,
  createEvalSet,
  updateEvalSet,
  deleteEvalSet,
  runEval,
  listEvalRuns,
  getEvalRun,
} from '@/api/domains'
import { mapEvalSet, mapEvalRunResult } from '@/api/mappers'
import type { EvalSetViewModel, EvalRunViewModel } from '@/api/mappers'

const evalKeys = {
  all: ['evals'] as const,
  lists: (domainId: string) => [...evalKeys.all, 'list', domainId] as const,
  details: (domainId: string) => [...evalKeys.all, 'detail', domainId] as const,
  detail: (domainId: string, setId: string) =>
    [...evalKeys.details(domainId), setId] as const,
  runs: (domainId: string, setId: string) =>
    [...evalKeys.detail(domainId, setId), 'runs'] as const,
  run: (domainId: string, setId: string, runId: string) =>
    [...evalKeys.runs(domainId, setId), runId] as const,
}

export function useEvalSets(domainId: string | undefined) {
  return useQuery<EvalSetViewModel[]>({
    queryKey: evalKeys.lists(domainId || ''),
    queryFn: async () => {
      const sets = await listEvalSets(domainId!)
      return sets.map((s) => mapEvalSet(s, domainId!))
    },
    enabled: !!domainId,
  })
}

export function useEvalSet(domainId: string | undefined, setId: string | undefined) {
  return useQuery<EvalSetViewModel>({
    queryKey: evalKeys.detail(domainId || '', setId || ''),
    queryFn: async () => {
      const set = await getEvalSet(domainId!, setId!)
      return mapEvalSet(set, domainId!)
    },
    enabled: !!domainId && !!setId,
  })
}

export interface EvalSetInput {
  id: string
  description: string
  cases: { id: string; name: string; input: string; expect: string }[]
}

function toEvalSet(input: EvalSetInput): EvalSet {
  return create(EvalSetSchema, {
    id: input.id,
    description: input.description,
    cases: input.cases.map((c) =>
      create(EvalCaseSchema, {
        id: c.id,
        name: c.name,
        input: c.input,
        expect: c.expect,
      }),
    ),
  })
}

export function useCreateEvalSet(domainId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: EvalSetInput) => createEvalSet(domainId!, toEvalSet(input)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: evalKeys.lists(domainId || '') })
    },
  })
}

export function useUpdateEvalSet(domainId: string | undefined, setId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: EvalSetInput) => updateEvalSet(domainId!, setId!, toEvalSet(input)),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: evalKeys.detail(domainId || '', setId || '') })
      queryClient.invalidateQueries({ queryKey: evalKeys.lists(domainId || '') })
    },
  })
}

export function useDeleteEvalSet(domainId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (setId: string) => deleteEvalSet(domainId!, setId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: evalKeys.lists(domainId || '') })
    },
  })
}

export function useRunEval(domainId: string | undefined, setId: string | undefined) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: { orchestration?: string; baselineRunId?: string } = {}) =>
      runEval(domainId!, setId!, {
        orchestration: body.orchestration,
        baseline_run_id: body.baselineRunId,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: evalKeys.runs(domainId || '', setId || '') })
    },
  })
}

export function useEvalRuns(
  domainId: string | undefined,
  setId: string | undefined,
  params: { limit?: number; offset?: number } = {},
) {
  return useQuery<{ items: EvalRunViewModel[]; total: number }>({
    queryKey: [...evalKeys.runs(domainId || '', setId || ''), params],
    queryFn: async () => {
      const res = await listEvalRuns(domainId!, setId!, params)
      return {
        items: res.items.map((r) => mapEvalRunResult(r)),
        total: res.total,
      }
    },
    enabled: !!domainId && !!setId,
  })
}

export function useEvalRun(
  domainId: string | undefined,
  setId: string | undefined,
  runId: string | undefined,
) {
  return useQuery<EvalRunViewModel>({
    queryKey: evalKeys.run(domainId || '', setId || '', runId || ''),
    queryFn: async () => {
      const res = await getEvalRun(domainId!, setId!, runId!)
      return mapEvalRunResult(res)
    },
    enabled: !!domainId && !!setId && !!runId,
  })
}
