import { ObservabilityPlayground } from '@hnsx/observability'

/**
 * 内部验收页 — 把 observability 包的所有组件渲在一屏，
 * 方便肉眼检查配色 / 间距 / typography 是否一致。
 * 不走 AppShell —— playground 自己就是 full-bleed 的。
 */
export default function PlaygroundPage() {
  return (
    <div className="-m-6 min-h-[calc(100vh-3.5rem)] bg-background">
      <ObservabilityPlayground />
    </div>
  )
}