import { PageHeader } from '@/components/ui/PageHeader'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

export default function ObservabilityPage() {
  return (
    <div className="space-y-4">
      <PageHeader title="Observability" description="Metrics and dashboards via Grafana." />
      <Card className="overflow-hidden">
        <CardHeader>
          <CardTitle>Grafana Dashboard</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <div className="flex h-[600px] items-center justify-center bg-muted">
            <p className="text-sm text-muted-foreground">Grafana iframe placeholder.</p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
