import { Suspense } from 'react'
import { Routes, Route } from 'react-router-dom'
import { AppShell } from '@/components/layout'
import { ErrorBoundary } from '@/components/layout/ErrorBoundary'
import { Loading } from '@/components/ui/Loading'
import { ToastProvider } from '@/components/ui/ToastProvider'
import { routes } from '@/routes'

function App() {
  return (
    <>
      <ToastProvider />
      <AppShell>
        <ErrorBoundary>
          <Suspense fallback={<Loading />}>
            <Routes>
              {routes.map((route) => (
                <Route key={route.path} path={route.path} element={<route.component />} />
              ))}
            </Routes>
          </Suspense>
        </ErrorBoundary>
      </AppShell>
    </>
  )
}

export default App
