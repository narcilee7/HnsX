import { lazy, type ComponentType } from 'react'

interface RouteConfig {
  path: string
  component: ComponentType
}

// 所有页面路由统一配置，支持 React.lazy 按需加载
// 新增页面时只需在此数组追加一行，无需修改 App.tsx
export const routes: RouteConfig[] = [
  { path: '/login', component: lazy(() => import('@/pages/LoginPage')) },
  { path: '/', component: lazy(() => import('@/pages/DashboardPage')) },
  { path: '/domains', component: lazy(() => import('@/pages/DomainsPage')) },
  { path: '/domains/:id', component: lazy(() => import('@/pages/DomainDetailPage')) },
  { path: '/domains/:id/workspace', component: lazy(() => import('@/pages/DomainWorkspacePage')) },
  { path: '/domains/:id/run', component: lazy(() => import('@/pages/DomainRunPage')) },
  { path: '/sessions', component: lazy(() => import('@/pages/SessionsPage')) },
  { path: '/sessions/:id', component: lazy(() => import('@/pages/SessionDetailPage')) },
  { path: '/traces', component: lazy(() => import('@/pages/TracesPage')) },
  { path: '/traces/:id', component: lazy(() => import('@/pages/TraceDetailPage')) },
  { path: '/evals', component: lazy(() => import('@/pages/EvalsPage')) },
  { path: '/evals/:setId', component: lazy(() => import('@/pages/EvalSetPage')) },
  { path: '/evals/:setId/runs/:runId', component: lazy(() => import('@/pages/EvalRunPage')) },
  { path: '/observability', component: lazy(() => import('@/pages/ObservabilityPage')) },
  { path: '/audit', component: lazy(() => import('@/pages/AuditPage')) },
  { path: '/approvals', component: lazy(() => import('@/pages/ApprovalsPage')) },
  { path: '/settings', component: lazy(() => import('@/pages/SettingsPage')) },
  { path: '/playground', component: lazy(() => import('@/pages/PlaygroundPage')) },
  { path: '/gallery', component: lazy(() => import('@/pages/GalleryPage')) },
]
