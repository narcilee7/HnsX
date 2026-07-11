import { useMemo } from 'react'
import { useFormContext, Controller } from 'react-hook-form'
import { z } from 'zod'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export interface JsonSchemaFormProps {
  schema: unknown
  namePrefix?: string
}

export function buildZodSchema(jsonSchema: unknown): z.ZodTypeAny {
  if (!jsonSchema || typeof jsonSchema !== 'object') {
    return z.any()
  }
  const s = jsonSchema as Record<string, unknown>
  const type = String(s.type || 'object')

  switch (type) {
    case 'string': {
      const values = Array.isArray(s.enum) ? s.enum.map(String) : []
      if (values.length > 0) {
        return z.enum(values as [string, ...string[]])
      }
      return z.string()
    }
    case 'number':
      return z.coerce.number()
    case 'integer':
      return z.coerce.number().int()
    case 'boolean':
      return z.boolean()
    case 'array':
      return z.array(buildZodSchema(s.items))
    case 'object': {
      const props = (s.properties || {}) as Record<string, unknown>
      const required = new Set<string>(
        Array.isArray(s.required) ? (s.required as string[]) : [],
      )
      const shape: Record<string, z.ZodTypeAny> = {}
      for (const [key, val] of Object.entries(props)) {
        let field = buildZodSchema(val)
        if (!required.has(key)) {
          field = field.optional() as z.ZodTypeAny
        }
        shape[key] = field
      }
      return z.object(shape)
    }
    default:
      return z.any()
  }
}

export function buildDefaultValues(jsonSchema: unknown): Record<string, unknown> {
  if (!jsonSchema || typeof jsonSchema !== 'object') return {}
  const s = jsonSchema as Record<string, unknown>
  if (s.type !== 'object') return {}
  const props = (s.properties || {}) as Record<string, unknown>
  const defaults: Record<string, unknown> = {}
  for (const [key, val] of Object.entries(props)) {
    const field = val as Record<string, unknown>
    const type = String(field.type || 'string')
    if (Array.isArray(field.enum) && field.enum.length > 0) {
      defaults[key] = field.enum[0]
    } else if (type === 'boolean') {
      defaults[key] = false
    } else if (type === 'number' || type === 'integer') {
      defaults[key] = ''
    } else {
      defaults[key] = ''
    }
  }
  return defaults
}

function fieldPath(prefix: string | undefined, name: string): string {
  return prefix ? `${prefix}.${name}` : name
}

export function JsonSchemaForm({ schema, namePrefix }: JsonSchemaFormProps) {
  const { control, formState } = useFormContext()
  const fields = useMemo(() => {
    if (!schema || typeof schema !== 'object') return []
    const s = schema as Record<string, unknown>
    if (s.type !== 'object') return []
    const props = (s.properties || {}) as Record<string, unknown>
    return Object.entries(props).map(([name, prop]) => ({
      name,
      schema: prop as Record<string, unknown>,
    }))
  }, [schema])

  return (
    <div className="space-y-4">
      {fields.map(({ name, schema: fieldSchema }) => {
        const path = fieldPath(namePrefix, name)
        const title =
          typeof fieldSchema.title === 'string'
            ? fieldSchema.title
            : name
        const description =
          typeof fieldSchema.description === 'string'
            ? fieldSchema.description
            : undefined
        const type = String(fieldSchema.type || 'string')
        const values = Array.isArray(fieldSchema.enum)
          ? fieldSchema.enum.map(String)
          : []
        const error = formState.errors[name]

        return (
          <div key={path} className="space-y-1.5">
            <Label htmlFor={path}>
              {title}
              {values.length > 0 ? (
                <Controller
                  name={path}
                  control={control}
                  render={({ field }) => (
                    <Select
                      value={String(field.value ?? '')}
                      onValueChange={field.onChange}
                    >
                      <SelectTrigger id={path} className="mt-1.5 w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {values.map((v) => (
                          <SelectItem key={v} value={v}>
                            {v}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
              ) : type === 'boolean' ? (
                <Controller
                  name={path}
                  control={control}
                  render={({ field }) => (
                    <div className="mt-1.5 flex items-center gap-2">
                      <input
                        id={path}
                        type="checkbox"
                        className="h-4 w-4 rounded border-input"
                        checked={!!field.value}
                        onChange={(e) => field.onChange(e.target.checked)}
                      />
                      <span className="text-xs text-muted-foreground">
                        {description || 'Enable'}
                      </span>
                    </div>
                  )}
                />
              ) : type === 'number' || type === 'integer' ? (
                <Controller
                  name={path}
                  control={control}
                  render={({ field }) => (
                    <Input
                      id={path}
                      type="number"
                      className="mt-1.5"
                      value={String(field.value ?? '')}
                      onChange={(e) =>
                        field.onChange(
                          e.target.value === '' ? '' : Number(e.target.value),
                        )
                      }
                    />
                  )}
                />
              ) : name === 'question' || fieldSchema.format === 'textarea' ? (
                <Controller
                  name={path}
                  control={control}
                  render={({ field }) => (
                    <Textarea
                      id={path}
                      className="mt-1.5 min-h-[100px]"
                      placeholder={description}
                      {...field}
                      value={String(field.value ?? '')}
                    />
                  )}
                />
              ) : (
                <Controller
                  name={path}
                  control={control}
                  render={({ field }) => (
                    <Input
                      id={path}
                      className="mt-1.5"
                      placeholder={description}
                      {...field}
                      value={String(field.value ?? '')}
                    />
                  )}
                />
              )}
            </Label>
            {error && (
              <p className="text-xs text-[var(--danger-text)]">
                {String(error.message || 'Invalid')}
              </p>
            )}
          </div>
        )
      })}
    </div>
  )
}

export function getFieldNames(jsonSchema: unknown): string[] {
  if (!jsonSchema || typeof jsonSchema !== 'object') return []
  const s = jsonSchema as Record<string, unknown>
  if (s.type !== 'object') return []
  return Object.keys((s.properties || {}) as Record<string, unknown>)
}
