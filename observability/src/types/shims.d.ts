/**
 * Type shims for third-party libs that ship without .d.ts files.
 * react-sparklines and react-calendar-heatmap haven't published TS types.
 * 把这些放一起便于后续切换到官方 @types 或 fork 时一次性替换。
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
    /** 起始日期 — Date | string | number */
    startDate?: Date | string | number
    /** 结束日期 — Date | string | number */
    endDate?: Date | string | number
    /** 数据点数组 */
    values: CalendarHeatmapValue[]
    /** 命名空间 class */
    classForValue?: (value: CalendarHeatmapValue | undefined) => string
    /** gutter 像素 */
    gutterSize?: number
    /** 自定义 day rect 渲染 */
    rectRender?: (
      props: CalendarHeatmapRectProps,
      value: CalendarHeatmapValue | undefined,
    ) => ReactElement
    /** 自定义 day element */
    transformDayElement?: (
      rect: ReactElement,
      value: CalendarHeatmapValue | undefined,
      index: number,
    ) => ReactElement
    /** 月标签开关 */
    showMonthLabels?: boolean
    /** 周几标签开关 */
    showWeekdayLabels?: boolean
    /** 月份分隔 */
    horizontal?: boolean
    /** 直角矩形 vs 圆角 */
    rx?: number
    ry?: number
  }

  const CalendarHeatmap: ComponentType<CalendarHeatmapProps>
  export default CalendarHeatmap
}