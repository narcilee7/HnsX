import { get } from './client'

export interface TemplateVariable {
  name: string
  default?: string
}

export interface TemplateRequirements {
  providers?: string[]
  min_workers?: number
  sandbox_runtimes?: string[]
}

export interface Template {
  id: string
  name: string
  description: string
  tags: string[]
  source: string
  variables: TemplateVariable[]
  requirements: TemplateRequirements
}

export interface ListTemplatesResponse {
  items: Template[]
  total: number
}

export function listTemplates(tag?: string): Promise<ListTemplatesResponse> {
  const search = new URLSearchParams()
  if (tag) search.set('tag', tag)
  const qs = search.toString()
  return get<ListTemplatesResponse>(`/templates${qs ? `?${qs}` : ''}`)
}
