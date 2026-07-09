// ---------- Tokens ----------
import './tokens/morandi.css'

// ---------- Primitives ----------
export { MetricCard, type MetricCardProps } from './primitives/MetricCard'
export { Sparkline, type SparklineProps } from './primitives/Sparkline'
export { TrendIndicator, type TrendIndicatorProps, type TrendDirection } from './primitives/TrendIndicator'

// ---------- Charts ----------
export { ChartFrame, type ChartFrameProps } from './charts/ChartFrame'
export { ChartSkeleton, ChartEmpty, type ChartSkeletonProps, type ChartEmptyProps } from './charts/ChartStates'
export { TokenUsageChart, type TokenUsageChartProps, type TokenUsagePoint } from './charts/TokenUsageChart'
export { LatencyHistogram, type LatencyHistogramProps, type LatencyBucket } from './charts/LatencyHistogram'
export { StatusDonut, type StatusDonutProps, type StatusDonutSlice } from './charts/StatusDonut'
export { ActivityHeatmap, type ActivityHeatmapProps, type ActivityHeatmapCell } from './charts/ActivityHeatmap'
export { AgentFlowDiagram, type AgentFlowDiagramProps, type AgentHandoff } from './charts/AgentFlowDiagram'

// ---------- Integrations ----------
export {
  GrafanaEmbed,
  resolveGrafanaBaseUrl,
  type GrafanaEmbedProps,
  type GrafanaTimeRange,
} from './integrations/GrafanaEmbed'
export {
  TraceMiniBar,
  type TraceMiniBarProps,
  type TraceMiniBarSpan,
} from './integrations/TraceMiniBar'
export { SpanList, type SpanListProps, type SpanListItem } from './integrations/SpanList'

// ---------- Lib ----------
export {
  cn,
  formatNumber,
  formatCompact,
  formatPercent,
  formatDurationMs,
  formatCostUsd,
  formatTokens,
} from './lib/utils'
export {
  CHART_SLOTS,
  STATUS_TOKENS,
  type ChartSlot,
  type StatusToken,
  chartVar,
  statusVar,
  slotFor,
} from './lib/palette'

// ---------- Playground ----------
export { default as ObservabilityPlayground } from './playground'