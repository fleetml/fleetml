import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { getVariants } from '../api/client'
import type { ModelVariant } from '../types/api'
import { useCatalog, type CatalogVersion } from '../hooks/useCatalog'
import { PageHeader, StatusBadge, errorMessage } from '../components/ui'
import CopyableId from '../components/CopyableId'

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

function VersionRow({ v }: { v: CatalogVersion }) {
  const [variants, setVariants] = useState<ModelVariant[] | null>(null)
  const [open, setOpen] = useState(false)
  const [error, setError] = useState('')
  const toggle = () => {
    setOpen(!open)
    if (!variants) getVariants(v.id).then(r => setVariants(r || [])).catch(e => setError(errorMessage(e)))
  }
  return (
    <div className="border-t border-gray-100">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
        <button onClick={toggle} className="flex items-center gap-1.5 text-sm font-medium text-gray-900" aria-expanded={open}>
          {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          {v.version}
        </button>
        <StatusBadge status={v.status} />
        <CopyableId id={v.id} />
        <div className="ml-auto flex gap-4 text-sm">
          {v.status !== 'ARCHIVED' && (
            <Link to={`/deployments?new=${v.id}`} className="text-indigo-600 hover:text-indigo-800">Deploy</Link>
          )}
          <Link to={`/telemetry?version=${v.id}`} className="text-indigo-600 hover:text-indigo-800">Telemetry</Link>
          <Link to={`/drift?version=${v.id}`} className="text-indigo-600 hover:text-indigo-800">Drift</Link>
        </div>
      </div>
      {open && (
        <div className="px-4 pb-3 pl-10">
          {error && <p className="text-sm text-red-600">{error}</p>}
          {variants?.length === 0 && <p className="text-sm text-gray-400">No variants uploaded.</p>}
          {variants && variants.length > 0 && (
            <div className="overflow-x-auto">
              <table className="min-w-max text-sm">
                <thead>
                  <tr className="text-left text-xs uppercase tracking-wider text-gray-500">
                    <th className="py-1 pr-6 font-medium">Runtime</th><th className="py-1 pr-6 font-medium">Quantization</th>
                    <th className="py-1 pr-6 font-medium">Size</th><th className="py-1 pr-6 font-medium">Status</th><th className="py-1 font-medium">SHA-256</th>
                  </tr>
                </thead>
                <tbody>
                  {variants.map(x => (
                    <tr key={x.id}>
                      <td className="py-1 pr-6 text-gray-900">{x.target_runtime}</td>
                      <td className="py-1 pr-6 text-gray-700">{x.quantization}</td>
                      <td className="py-1 pr-6 text-gray-700">{formatBytes(x.blob_size_bytes)}</td>
                      <td className="py-1 pr-6"><StatusBadge status={x.status} /></td>
                      <td className="py-1 font-mono text-xs text-gray-500">{x.blob_sha256.slice(0, 16)}…</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export default function Models() {
  const catalog = useCatalog()
  return (
    <div>
      <PageHeader title="Models" />
      {!catalog.loading && catalog.models.length === 0 && (
        <p className="text-sm text-gray-400">No models yet. Push one with <code className="font-mono">fleetml push</code> or the REST API.</p>
      )}
      <div className="space-y-6">
        {catalog.projects.map(p => {
          const models = catalog.models.filter(m => m.project_id === p.id)
          if (models.length === 0) return null
          return (
            <section key={p.id}>
              <h3 className="text-sm font-semibold uppercase tracking-wider text-gray-500 mb-2">{p.name}</h3>
              <div className="space-y-3">
                {models.map(m => {
                  const versions = catalog.versions.filter(v => v.model_id === m.id)
                  return (
                    <div key={m.id} className="border border-gray-200 rounded-lg shadow-sm">
                      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-4 py-3">
                        <span className="text-base font-semibold text-gray-900">{m.name}</span>
                        <span className="text-sm text-gray-500">{m.framework} · {m.task_type}</span>
                        <span className="text-sm text-gray-400">{versions.length} version{versions.length === 1 ? '' : 's'}</span>
                      </div>
                      {versions.map(v => <VersionRow key={v.id} v={v} />)}
                    </div>
                  )
                })}
              </div>
            </section>
          )
        })}
      </div>
    </div>
  )
}
