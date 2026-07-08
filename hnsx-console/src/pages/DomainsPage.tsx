import { useState } from 'react'
import { Link } from 'react-router-dom'

interface Domain {
  id: string
  version: string
  description: string
  mode: string
}

const mockDomains: Domain[] = [
  {
    id: 'customer-service',
    version: '0.1.0',
    description: 'Routes customer questions to the right specialist agent.',
    mode: 'workflow',
  },
  {
    id: 'code-review',
    version: '0.1.0',
    description: 'Automated code review harness.',
    mode: 'single-task',
  },
]

export default function DomainsPage() {
  const [domains] = useState<Domain[]>(mockDomains)

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Domains</h1>
        <button className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700">
          Register Domain
        </button>
      </div>
      <div className="rounded-lg border bg-white shadow-sm">
        <table className="w-full text-left text-sm">
          <thead className="bg-gray-100">
            <tr>
              <th className="px-4 py-3 font-medium">ID</th>
              <th className="px-4 py-3 font-medium">Version</th>
              <th className="px-4 py-3 font-medium">Mode</th>
              <th className="px-4 py-3 font-medium">Description</th>
              <th className="px-4 py-3 font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {domains.map((domain) => (
              <tr key={domain.id} className="border-t">
                <td className="px-4 py-3 font-medium">{domain.id}</td>
                <td className="px-4 py-3 text-gray-600">{domain.version}</td>
                <td className="px-4 py-3">
                  <span className="rounded-full bg-gray-100 px-2 py-1 text-xs">{domain.mode}</span>
                </td>
                <td className="px-4 py-3 text-gray-600">{domain.description}</td>
                <td className="px-4 py-3">
                  <Link
                    to={`/domains/${domain.id}`}
                    className="text-indigo-600 hover:underline"
                  >
                    View
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
