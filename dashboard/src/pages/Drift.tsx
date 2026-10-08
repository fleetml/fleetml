import { useState } from 'react'
import { getDriftStatus } from '../api/client'
import { UnauthorizedError } from '../api/auth'
import type { DriftResult } from '../types/api'

const sevStyle: Record<string, string> = {
  NONE: 'text-green-600',
  WARNING: 'text-amber-600',
  CRITICAL: 'text-red-600',
}

export default function Drift() {
  const [versionId, setVersionId] = useState('')
  const [results, setResults] = useState<DriftResult[]>([])
  const [error, setError] = useState('')

  const load = () => {
    if (!versionId) return
    setError('')
    getDriftStatus(versionId).then(r => setResults(r || [])).catch(e => { if (!(e instanceof UnauthorizedError)) setError(e.message) })
  }

  return (
    <div>
      <h2 className="text-2xl font-semibold text-gray-900 mb-6">Drift Detection</h2>
      <div className="flex gap-2 mb-6">
        <input
          value={versionId}
          onChange={e => setVersionId(e.target.value)}
          placeholder="Model Version ID"
          className="border border-gray-300 rounded-md px-3 py-2 text-sm flex-1 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
        />
        <button onClick={load} className="bg-indigo-600 text-white px-4 py-2 rounded-md text-sm font-medium hover:bg-indigo-700">Check</button>
      </div>
      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      {results.length > 0 ? (
        <div className="border border-gray-200 rounded-lg shadow-sm overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="bg-gray-50 border-b border-gray-200">
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Metric</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">PSI</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Severity</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Baseline N</th>
                <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Current N</th>
              </tr>
            </thead>
            <tbody>
              {results.map((r, i) => (
                <tr key={i} className="border-b border-gray-100 hover:bg-gray-50">
                  <td className="px-4 py-3 text-sm text-gray-900">{r.metric}</td>
                  <td className="px-4 py-3 text-sm font-mono text-gray-900">{r.psi_value?.toFixed(4)}</td>
                  <td className={`px-4 py-3 text-sm font-medium ${sevStyle[r.severity] || 'text-gray-500'}`}>{r.severity}</td>
                  <td className="px-4 py-3 text-sm text-gray-500">{r.baseline_sample?.toLocaleString()}</td>
                  <td className="px-4 py-3 text-sm text-gray-500">{r.current_sample?.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : !error && <p className="text-sm text-gray-400">Enter a model version ID to check drift.</p>}
    </div>
  )
}
