import { Routes, Route } from 'react-router-dom'
import { AppShell } from '@/components/layout'
import { ToastProvider } from '@/components/ui/ToastProvider'
import DashboardPage from '@/pages/DashboardPage'
import DomainsPage from '@/pages/DomainsPage'
import DomainDetailPage from '@/pages/DomainDetailPage'
import DomainRunPage from '@/pages/DomainRunPage'
import SessionsPage from '@/pages/SessionsPage'
import SessionDetailPage from '@/pages/SessionDetailPage'
import TracesPage from '@/pages/TracesPage'
import TraceDetailPage from '@/pages/TraceDetailPage'
import EvalsPage from '@/pages/EvalsPage'
import EvalSetPage from '@/pages/EvalSetPage'
import EvalRunPage from '@/pages/EvalRunPage'
import ObservabilityPage from '@/pages/ObservabilityPage'
import AuditPage from '@/pages/AuditPage'
import ApprovalsPage from '@/pages/ApprovalsPage'
import SettingsPage from '@/pages/SettingsPage'

function App() {
  return (
    <>
      <ToastProvider />
      <AppShell>
        <Routes>
          <Route path="/" element={<DashboardPage />} />
          <Route path="/domains" element={<DomainsPage />} />
          <Route path="/domains/:id" element={<DomainDetailPage />} />
          <Route path="/domains/:id/run" element={<DomainRunPage />} />
          <Route path="/sessions" element={<SessionsPage />} />
          <Route path="/sessions/:id" element={<SessionDetailPage />} />
          <Route path="/traces" element={<TracesPage />} />
          <Route path="/traces/:id" element={<TraceDetailPage />} />
          <Route path="/evals" element={<EvalsPage />} />
          <Route path="/domains/:domainId/evals/:setId" element={<EvalSetPage />} />
          <Route path="/domains/:domainId/evals/:setId/runs/:runId" element={<EvalRunPage />} />
          <Route path="/observability" element={<ObservabilityPage />} />
          <Route path="/audit" element={<AuditPage />} />
          <Route path="/approvals" element={<ApprovalsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </AppShell>
    </>
  )
}

export default App
