import { useState } from 'react'
import { getTelemetrySummary, getTelemetryTimeSeries } from '../api/client'
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts'
import type { TelemetrySummary, TimeSeriesPoint } from '../types/api'

export default function Telemetry() {
  const [versionId, setVersionId] = useState('')
  const [summary, setSummary] = useState<TelemetrySummary | null>(null)
  const [timeSeries, setTimeSeries] = useState<TimeSeriesPoint[]>([])
  const [error, setError] = useState('')

  const load = async () => {
    if (!versionId) return
    setError('')
    try {
      const [s, ts] = await Promise.all([
        getTelemetrySummary(versionId),
        getTelemetryTimeSeries(versionId, '1h'),
      ])
      setSummary(s)
      setTimeSeries(ts || [])
    } catch (e: any) { setError(e.message) }
  }

  return (
    <div>
      <h2 className="text-2xl font-semibold text-gray-900 mb-6">Telemetry</h2>
      <div className="flex gap-2 mb-6">
        <input
          value={versionId}
          onChange={e => setVersionId(e.target.value)}
          placeholder="Model Version ID"
          className="border border-gray-300 rounded-md px-3 py-2 text-sm flex-1 focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent"
        />
        <button onClick={load} className="bg-indigo-600 text-white px-4 py-2 rounded-md text-sm font-medium hover:bg-indigo-700">Load</button>
      </div>
      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      {summary && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          {[
            { label: 'Total Events', value: summary.total_events.toLocaleString() },
            { label: 'Avg Latency', value: `${summary.avg_latency_ms?.toFixed(1)}ms` },
            { label: 'P95 Latency', value: `${summary.p95_latency_ms?.toFixed(1)}ms` },
            { label: 'Error Rate', value: `${((summary.error_count / (summary.total_events || 1)) * 100).toFixed(2)}%` },
          ].map(s => (
            <div key={s.label} className="border border-gray-200 rounded-lg shadow-sm p-4">
              <div className="text-sm font-medium text-gray-500 mb-1">{s.label}</div>
              <div className="text-2xl font-semibold text-gray-900">{s.value}</div>
            </div>
          ))}
        </div>
      )}
      {timeSeries.length > 0 && (
        <div className="border border-gray-200 rounded-lg shadow-sm p-5">
          <h3 className="text-sm font-semibold text-gray-900 mb-4">Latency over time</h3>
          <ResponsiveContainer width="100%" height={300}>
            <LineChart data={timeSeries}>
              <CartesianGrid strokeDasharray="3 3" stroke="#f0f0f0" />
              <XAxis dataKey="time" tickFormatter={v => new Date(v).toLocaleTimeString()} tick={{ fontSize: 12 }} stroke="#9ca3af" />
              <YAxis tick={{ fontSize: 12 }} stroke="#9ca3af" />
              <Tooltip />
              <Line type="monotone" dataKey="avg_latency_ms" stroke="#4f46e5" strokeWidth={2} dot={false} name="Avg" />
              <Line type="monotone" dataKey="p95_latency_ms" stroke="#a5b4fc" strokeWidth={1.5} dot={false} name="P95" />
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
      {!summary && !error && <p className="text-sm text-gray-400">Enter a model version ID to view telemetry.</p>}
    </div>
  )
}
