import { useEffect, useState } from 'react'
import { getDeployments, startDeployment, pauseDeployment, rollbackDeployment } from '../api/client'
import type { Deployment } from '../types/api'
import CopyableId from '../components/CopyableId'

const statusStyle: Record<string, string> = {
  PENDING: 'bg-gray-100 text-gray-600',
  ROLLING_OUT: 'bg-indigo-50 text-indigo-700',
  PAUSED: 'bg-amber-50 text-amber-700',
  COMPLETE: 'bg-green-50 text-green-700',
  ROLLED_BACK: 'bg-red-50 text-red-700',
  FAILED: 'bg-red-50 text-red-700',
}

export default function Deployments() {
  const [deps, setDeps] = useState<Deployment[]>([])
  const reload = () => getDeployments().then(d => setDeps(d || [])).catch(() => {})
  useEffect(() => { reload() }, [])

  return (
    <div>
      <h2 className="text-2xl font-semibold text-gray-900 mb-6">Deployments</h2>
      <div className="border border-gray-200 rounded-lg shadow-sm overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">ID</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Strategy</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Status</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Stage</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider"></th>
            </tr>
          </thead>
          <tbody>
            {deps.length === 0 && <tr><td colSpan={5} className="px-4 py-8 text-center text-sm text-gray-400">No deployments</td></tr>}
            {deps.map(d => (
              <tr key={d.id} className="border-b border-gray-100 hover:bg-gray-50">
                <td className="px-4 py-3"><CopyableId id={d.id} /></td>
                <td className="px-4 py-3 text-sm text-gray-900">{d.strategy}</td>
                <td className="px-4 py-3">
                  <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium ${statusStyle[d.status] || 'bg-gray-100 text-gray-600'}`}>{d.status}</span>
                </td>
                <td className="px-4 py-3 text-sm text-gray-500">{d.current_stage}</td>
                <td className="px-4 py-3">
                  <div className="flex gap-3">
                    {d.status === 'PENDING' && <button onClick={() => startDeployment(d.id).then(reload)} className="text-indigo-600 hover:text-indigo-800 text-sm">Start</button>}
                    {d.status === 'ROLLING_OUT' && <button onClick={() => pauseDeployment(d.id).then(reload)} className="text-amber-600 hover:text-amber-800 text-sm">Pause</button>}
                    {(d.status === 'ROLLING_OUT' || d.status === 'PAUSED') && <button onClick={() => rollbackDeployment(d.id).then(reload)} className="text-red-600 hover:text-red-800 text-sm">Rollback</button>}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
