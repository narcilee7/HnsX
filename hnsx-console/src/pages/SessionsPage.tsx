interface Session {
  id: string
  domainId: string
  state: string
  startedAt: string
}

const mockSessions: Session[] = [
  { id: 'sess-001', domainId: 'customer-service', state: 'completed', startedAt: '2026-07-08T12:00:00Z' },
  { id: 'sess-002', domainId: 'code-review', state: 'failed', startedAt: '2026-07-08T12:30:00Z' },
]

export default function SessionsPage() {
  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold">Sessions</h1>
      <div className="rounded-lg border bg-white shadow-sm">
        <table className="w-full text-left text-sm">
          <thead className="bg-gray-100">
            <tr>
              <th className="px-4 py-3 font-medium">ID</th>
              <th className="px-4 py-3 font-medium">Domain</th>
              <th className="px-4 py-3 font-medium">State</th>
              <th className="px-4 py-3 font-medium">Started At</th>
            </tr>
          </thead>
          <tbody>
            {mockSessions.map((session) => (
              <tr key={session.id} className="border-t">
                <td className="px-4 py-3 font-medium">{session.id}</td>
                <td className="px-4 py-3 text-gray-600">{session.domainId}</td>
                <td className="px-4 py-3">
                  <span
                    className={`rounded-full px-2 py-1 text-xs ${
                      session.state === 'completed'
                        ? 'bg-green-100 text-green-800'
                        : 'bg-red-100 text-red-800'
                    }`}
                  >
                    {session.state}
                  </span>
                </td>
                <td className="px-4 py-3 text-gray-600">{session.startedAt}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
