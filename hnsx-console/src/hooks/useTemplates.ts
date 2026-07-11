import { useQuery } from '@tanstack/react-query'
import { listTemplates } from '@/api/templates'

const templateKeys = {
  all: ['templates'] as const,
  lists: () => [...templateKeys.all, 'list'] as const,
  list: (tag?: string) => [...templateKeys.lists(), tag ?? 'all'] as const,
}

export function useTemplates(tag?: string) {
  return useQuery({
    queryKey: templateKeys.list(tag),
    queryFn: () => listTemplates(tag),
  })
}
