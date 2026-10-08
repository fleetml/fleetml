import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, ReferenceLine, Legend } from 'recharts'
import { checkDrift, getDriftHistory, getDriftPoints, getDriftStatus } from '../api/client'
import { UnauthorizedError } from '../api/auth'
import type { DriftPoint, DriftResult } from '../types/api'
import { useCatalog } from '../hooks/useCatalog'
import VersionPicker from '../components/VersionPicker'
import { Button, PageHeader, StatusBadge, Table, errorMessage, shortDate } from '../components/ui'

const fmtWindow = (from: string, to: string) =>
  `${new Date(from).toLocaleDateString()} – ${new Date(to).toLocaleDateString()}`

const colors = ['#4f46e5', '#0891b2', '#16a34a', '#d97706', '#db2777', '#7c3aed', '#64748b']

/** PSI of every check over time, one line per metric (latency, confidence, feature:<name>). */
function PSIChart({ points }: { points: DriftPoint[] }) {
  const metrics = [...new Set(points.map(p => p.metric))].sort()
  const rows: Record<string, number | string>[] = []
  for (const p of points) {
    const t = new Date(p.checked_at).getTime()
    let row = rows.find(r => r.t === t)
    if (!row) { row = { t }; rows.push(row) }
    row[p.metric] = p.psi
  }
  return (
    <div className="border border-gray-200 rounded-lg shadow-sm p-4 mb-6">
      <h3 className="text-sm font-semibold text-gray-900 mb-1">PSI over time</h3>
      <p className="text-xs text-gray-500 mb-3">Every check of the last 7 days. 0.1 and above is WARNING, 0.25 and above CRITICAL.</p>
      <ResponsiveContainer width="100%" height={260}>
        <LineChart data={rows}>
          <CartesianGrid strokeDasharray="3 3" stroke="#f3f4f6" />
          <XAxis dataKey="t" type="number" domain={['dataMin', 'dataMax']} tickFormatter={v => new Date(v).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })} tick={{ fontSize: 12 }} stroke="#9ca3af" />
          <YAxis tick={{ fontSize: 12 }} stroke="#9ca3af" />
          <Tooltip labelFormatter={v => new Date(Number(v)).toLocaleString()} formatter={(v: number) => v.toFixed(3)} />
          <Legend wrapperStyle={{ fontSize: 12 }} />
          <ReferenceLine y={0.1} stroke="#f59e0b" strokeDasharray="4 4" />
          <ReferenceLine y={0.25} stroke="#ef4444" strokeDasharray="4 4" />
          {/* A function, not the name: recharts reads a string dataKey as a path ("a.b"). */}
          {metrics.map((m, i) => <Line key={m} name={m} type="monotone" dataKey={(r: Record<string, number>) => r[m]} stroke={colors[i % colors.length]}
            strokeWidth={2} dot={{ r: 2 }} connectNulls isAnimationActive={false} />)}
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}

export default function Drift() {
  const catalog = useCatalog()
  const [params, setParams] = useSearchParams()
  const versionId = params.get('version') || ''
  const [latest, setLatest] = useState<DriftResult[]>([])
  const [history, setHistory] = useState<DriftResult[]>([])
  const [points, setPoints] = useState<DriftPoint[]>([])
  const [error, setError] = useState('')
  const [checking, setChecking] = useState(false)

  const load = (id: string) => {
    setError('')
    Promise.all([getDriftStatus(id), getDriftHistory(id), getDriftPoints(id)])
      .then(([l, h, p]) => { setLatest(l || []); setHistory(h || []); setPoints(p || []) })
      .catch(e => { if (!(e instanceof UnauthorizedError)) setError(errorMessage(e)) })
  }
  useEffect(() => { if (versionId) load(versionId); else { setLatest([]); setHistory([]); setPoints([]) } }, [versionId])

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
      {!versionId && <p className="text-sm text-gray-400">Choose a model version. Drift compares the last 24 hours of its latency, confidence and configured input features with the 7 days before (PSI).</p>}
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
          {points.length > 0 && <PSIChart points={points} />}
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
