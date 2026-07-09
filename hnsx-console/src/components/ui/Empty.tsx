import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

interface EmptyProps {
  title?: string
  description?: string
}

export function Empty({ title = 'No data', description = 'There is nothing to show here yet.' }: EmptyProps) {
  return (
    <Card className="flex flex-col items-center justify-center py-12">
      <CardHeader className="text-center">
        <CardTitle>{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent />
    </Card>
  )
}
