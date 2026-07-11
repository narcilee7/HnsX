/**
 * Type shims for third-party libs that ship without .d.ts files.
 * react-sparklines / react-calendar-heatmap haven't published TS types.
 *
 * 镜像一份在 hnsx-console 侧：因为 tsc 用 host 的 tsconfig 编译观测包源码，
 * 不会自动加载 observability 包内的 .d.ts。把这份 shim 放进 host 的 src/types/。
 *
 * 观测包内的同名 shim（observability/src/types/shims.d.ts）仍然保留，
 * 用于将来独立发版时其他宿主引用。
 */
declare module 'react-sparklines' {
  import type { CSSProperties, ReactElement } from 'react'

  export interface SparklinesProps {
    data: number[]
    height?: number
    width?: number | string
    margin?: number | [number, number, number, number]
    style?: CSSProperties
    children?: React.ReactNode
  }
  export const Sparklines: (props: SparklinesProps) => ReactElement

  export interface SparklinesLineProps {
    style?: CSSProperties
    color?: string
  }
  export const SparklinesLine: (props: SparklinesLineProps) => ReactElement

  export interface SparklinesCurveProps {
    style?: CSSProperties
    color?: string
  }
  export const SparklinesCurve: (props: SparklinesCurveProps) => ReactElement

  export interface SparklinesSpotsProps {
    size?: number
    style?: CSSProperties
  }
  export const SparklinesSpots: (props: SparklinesSpotsProps) => ReactElement

  export interface SparklinesReferenceLineProps {
    type?: 'mean' | 'avg' | 'min' | 'max' | 'median'
    style?: CSSProperties
  }
  export const SparklinesReferenceLine: (props: SparklinesReferenceLineProps) => ReactElement
}

declare module 'react-calendar-heatmap' {
  import type { ComponentType, ReactElement } from 'react'

  export interface CalendarHeatmapValue {
    date: string
    count?: number
    [key: string]: unknown
  }

  export interface CalendarHeatmapRectProps {
    [key: string]: unknown
  }

  export interface CalendarHeatmapProps {
    startDate?: Date | string | number
    endDate?: Date | string | number
    values: CalendarHeatmapValue[]
    classForValue?: (value: CalendarHeatmapValue | undefined) => string
    gutterSize?: number
    rectRender?: (
      props: CalendarHeatmapRectProps,
      value: CalendarHeatmapValue | undefined,
    ) => ReactElement
    transformDayElement?: (
      rect: ReactElement,
      value: CalendarHeatmapValue | undefined,
      index: number,
    ) => ReactElement
    showMonthLabels?: boolean
    showWeekdayLabels?: boolean
    horizontal?: boolean
    rx?: number
    ry?: number
  }

  const CalendarHeatmap: ComponentType<CalendarHeatmapProps>
  export default CalendarHeatmap
}