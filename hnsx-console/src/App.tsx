import { Routes, Route, Link } from 'react-router-dom'
import DomainsPage from './pages/DomainsPage'
import DomainDetailPage from './pages/DomainDetailPage'
import SessionsPage from './pages/SessionsPage'

function App() {
  return (
    <div className="min-h-screen bg-gray-50 text-gray-900">
      <nav className="border-b bg-white px-6 py-3">
        <div className="mx-auto flex max-w-7xl items-center gap-6">
          <Link to="/" className="text-lg font-bold text-indigo-600">HnsX</Link>
          <Link to="/domains" className="text-sm hover:text-indigo-600">Domains</Link>
          <Link to="/sessions" className="text-sm hover:text-indigo-600">Sessions</Link>
        </div>
      </nav>
      <main className="mx-auto max-w-7xl p-6">
        <Routes>
          <Route path="/" element={<DomainsPage />} />
          <Route path="/domains" element={<DomainsPage />} />
          <Route path="/domains/:id" element={<DomainDetailPage />} />
          <Route path="/sessions" element={<SessionsPage />} />
        </Routes>
      </main>
    </div>
  )
}

export default App
