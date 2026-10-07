import { useEffect, useState } from 'react'
import { getExperiments, getExperimentResults } from '../api/client'
import type { Experiment, ExperimentResult } from '../types/api'

const statusStyle: Record<string, string> = {
  DRAFT: 'bg-gray-100 text-gray-600',
  RUNNING: 'bg-indigo-50 text-indigo-700',
  PAUSED: 'bg-amber-50 text-amber-700',
  CONCLUDED: 'bg-green-50 text-green-700',
  CANCELLED: 'bg-gray-100 text-gray-500',
}

export default function Experiments() {
  const [exps, setExps] = useState<Experiment[]>([])
  const [results, setResults] = useState<ExperimentResult | null>(null)
  useEffect(() => { getExperiments().then(e => setExps(e || [])).catch(() => {}) }, [])

  return (
    <div>
      <h2 className="text-2xl font-semibold text-gray-900 mb-6">Experiments</h2>
      <div className="border border-gray-200 rounded-lg shadow-sm overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Name</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Metric</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider"></th>
            </tr>
          </thead>
          <tbody>
            {exps.length === 0 && <tr><td colSpan={4} className="px-4 py-8 text-center text-sm text-gray-400">No experiments</td></tr>}
            {exps.map(e => (
              <tr key={e.id} className="border-b border-gray-100 hover:bg-gray-50">
                <td className="px-4 py-3 text-sm font-medium text-gray-900">{e.name}</td>
                <td className="px-4 py-3">
                  <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium ${statusStyle[e.status] || 'bg-gray-100 text-gray-600'}`}>{e.status}</span>
                </td>
                <td className="px-4 py-3 text-sm text-gray-500">{e.primary_metric}</td>
                <td className="px-4 py-3">
                  <button onClick={() => getExperimentResults(e.id).then(setResults).catch(() => {})} className="text-indigo-600 hover:text-indigo-800 text-sm">Results</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {results && (
        <div className="mt-4 border border-gray-200 rounded-lg shadow-sm p-5">
          <h3 className="text-sm font-semibold text-gray-900 mb-3">Results</h3>
          <div className="grid grid-cols-2 gap-x-8 gap-y-2 text-sm">
            <div className="flex justify-between"><span className="text-gray-500">Recommendation</span><span className="font-medium text-gray-900">{results.recommendation}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">P-value</span><span className="text-gray-900">{results.p_value?.toFixed(4)}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">Significant</span><span className={results.is_significant ? 'text-green-600' : 'text-gray-500'}>{results.is_significant ? 'Yes' : 'No'}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">Effect size</span><span className="text-gray-900">{results.effect_size?.toFixed(3)}</span></div>
          </div>
          <button onClick={() => setResults(null)} className="mt-3 text-sm text-gray-400 hover:text-gray-600">Dismiss</button>
        </div>
      )}
    </div>
  )
}
