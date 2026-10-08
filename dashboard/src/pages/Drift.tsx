import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { checkDrift, getDriftHistory, getDriftStatus } from '../api/client'
import { UnauthorizedError } from '../api/auth'
import type { DriftResult } from '../types/api'
import { useCatalog } from '../hooks/useCatalog'
import VersionPicker from '../components/VersionPicker'
import { Button, PageHeader, StatusBadge, Table, errorMessage, shortDate } from '../components/ui'

const fmtWindow = (from: string, to: string) =>
  `${new Date(from).toLocaleDateString()} – ${new Date(to).toLocaleDateString()}`

export default function Drift() {
  const catalog = useCatalog()
  const [params, setParams] = useSearchParams()
  const versionId = params.get('version') || ''
  const [latest, setLatest] = useState<DriftResult[]>([])
  const [history, setHistory] = useState<DriftResult[]>([])
  const [error, setError] = useState('')
  const [checking, setChecking] = useState(false)

  const load = (id: string) => {
    setError('')
    Promise.all([getDriftStatus(id), getDriftHistory(id)])
      .then(([l, h]) => { setLatest(l || []); setHistory(h || []) })
      .catch(e => { if (!(e instanceof UnauthorizedError)) setError(errorMessage(e)) })
  }
  useEffect(() => { if (versionId) load(versionId); else { setLatest([]); setHistory([]) } }, [versionId])

  const runCheck = async () => {
    setChecking(true)
    try {
      await checkDrift(versionId)
      load(versionId)
    } catch (e) {
      setError(errorMessage(e))
    } finally {
      setChecking(false)
    }
  }

  return (
    <div>
      <PageHeader title="Drift" action={versionId && <Button onClick={runCheck} disabled={checking}>Check now</Button>} />
      <VersionPicker catalog={catalog} value={versionId} onChange={id => setParams(id ? { version: id } : {})} />
      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      {!versionId && <p className="text-sm text-gray-400">Choose a model version. Drift compares the last 24 hours of its latency and confidence with the 7 days before (PSI).</p>}
      {versionId && (
        <>
          <div className="grid sm:grid-cols-2 gap-4 mb-6">
            {latest.length === 0 && <p className="text-sm text-gray-400">No drift checks yet. They run in the background, or use Check now.</p>}
            {latest.map(r => (
              <div key={r.metric} className="border border-gray-200 rounded-lg shadow-sm p-4">
                <div className="flex items-center justify-between mb-1">
                  <span className="text-sm font-medium text-gray-500">{r.metric}</span>
                  <StatusBadge status={r.severity} />
                </div>
                <div className="text-2xl font-semibold text-gray-900">PSI {r.psi_value.toFixed(3)}</div>
                <div className="text-xs text-gray-500 mt-1">{r.baseline_sample.toLocaleString()} baseline vs {r.current_sample.toLocaleString()} current events · checked {shortDate(r.checked_at)}</div>
              </div>
            ))}
          </div>
          <h3 className="text-sm font-semibold text-gray-900 mb-2">History</h3>
          <p className="text-xs text-gray-500 mb-3">A row is added when a metric's severity changes; repeated checks with the same severity update the latest row.</p>
          <Table head={['Checked', 'Metric', 'Severity', 'PSI', 'Baseline window', 'Current window', 'Events']}
            empty={history.length === 0 ? 'No history' : undefined}>
            {history.map(r => (
              <tr key={r.id} className="border-b border-gray-100 hover:bg-gray-50">
                <td className="px-4 py-3 text-sm text-gray-700 whitespace-nowrap">{shortDate(r.checked_at)}</td>
                <td className="px-4 py-3 text-sm text-gray-900">{r.metric}</td>
                <td className="px-4 py-3"><StatusBadge status={r.severity} /></td>
                <td className="px-4 py-3 text-sm font-mono text-gray-900">{r.psi_value.toFixed(4)}</td>
                <td className="px-4 py-3 text-sm text-gray-500">{fmtWindow(r.baseline_from, r.baseline_to)}</td>
                <td className="px-4 py-3 text-sm text-gray-500">{fmtWindow(r.current_from, r.current_to)}</td>
                <td className="px-4 py-3 text-sm text-gray-500">{r.baseline_sample.toLocaleString()} / {r.current_sample.toLocaleString()}</td>
              </tr>
            ))}
          </Table>
        </>
      )}
    </div>
  )
}
