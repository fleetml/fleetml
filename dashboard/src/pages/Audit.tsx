import { useEffect, useState } from 'react'
import { auditExportURL, getAudit, getProjects, type AuditFilter } from '../api/client'
import type { AuditEntry, Project } from '../types/api'
import { Button, Card, Field, PageHeader, Select, Table, TextInput, errorMessage, shortDate } from '../components/ui'

const outcomes = [{ value: '', label: 'All' }, { value: 'denied', label: 'Refused (403)' }, { value: 'allowed', label: 'Allowed' }]

function Outcome({ e }: { e: AuditEntry }) {
  const denied = e.status === 403
  return (
    <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium whitespace-nowrap ${denied ? 'bg-red-50 text-red-700' : e.status && e.status >= 400 ? 'bg-amber-50 text-amber-700' : 'bg-green-50 text-green-700'}`}>
      {e.status ?? '—'}{denied ? ' refused' : ''}
    </span>
  )
}

export default function Audit() {
  const [projects, setProjects] = useState<Project[]>([])
  const [filter, setFilter] = useState<AuditFilter>({ outcome: '', limit: 200 })
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [error, setError] = useState('')
  const projectName = (id?: string) => projects.find(p => p.id === id)?.name || (id ? id.slice(0, 8) : '—')

  useEffect(() => { getProjects().then(p => setProjects(p || [])).catch(() => undefined) }, [])
  const load = (f: AuditFilter) => {
    setError('')
    getAudit(f).then(e => setEntries(e || [])).catch(e => { setEntries([]); setError(errorMessage(e)) })
  }
  useEffect(() => { load(filter) }, [])
  const set = (patch: Partial<AuditFilter>) => setFilter(f => ({ ...f, ...patch }))

  return (
    <div>
      <PageHeader title="Audit log" action={<a href={auditExportURL(filter)} download className="px-4 py-2 rounded-md text-sm font-medium border border-gray-300 text-gray-700 hover:bg-gray-50">Export CSV</a>} />
      <p className="text-sm text-gray-500 mb-4">Every request that changes something, and every refused request. Project admins see their project's entries; global admins see all.</p>
      <form className="grid sm:grid-cols-5 gap-3 items-end mb-4" onSubmit={e => { e.preventDefault(); load(filter) }}>
        <Field label="Project">
          <Select name="project" value={filter.project_id || ''} onChange={v => set({ project_id: v || undefined })} placeholder="All"
            options={projects.map(p => ({ value: p.id, label: p.name }))} />
        </Field>
        <Field label="User"><TextInput name="actor" value={filter.actor || ''} onChange={e => set({ actor: e.target.value })} placeholder="any" /></Field>
        <Field label="Action contains"><TextInput name="action" value={filter.action || ''} onChange={e => set({ action: e.target.value })} placeholder="POST /deployments" /></Field>
        <Field label="Outcome"><Select name="outcome" value={filter.outcome || ''} onChange={v => set({ outcome: v as AuditFilter['outcome'] })} options={outcomes} /></Field>
        <div className="mb-3"><Button type="submit">Apply</Button></div>
      </form>
      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      <Table head={['Time', 'User', 'Action', 'Result', 'Project', 'Key', 'IP']} empty={entries.length === 0 ? 'No entries' : undefined}
        mobile={entries.map(e => (
          <Card key={e.id} title={e.action} subtitle={`${e.actor} · ${shortDate(e.recorded_at)}`} badge={<Outcome e={e} />}
            lines={[['Project', projectName(e.project_id)], ['Key', String(e.details.api_key ?? '—')]]} />
        ))}>
        {entries.map(e => (
          <tr key={e.id} className="border-b border-gray-100">
            <td className="px-4 py-3 text-sm text-gray-500 whitespace-nowrap">{shortDate(e.recorded_at)}</td>
            <td className="px-4 py-3 text-sm text-gray-900">{e.actor}</td>
            <td className="px-4 py-3 text-sm font-mono text-gray-700">{e.action}</td>
            <td className="px-4 py-3"><Outcome e={e} /></td>
            <td className="px-4 py-3 text-sm text-gray-700">{projectName(e.project_id)}</td>
            <td className="px-4 py-3 text-sm text-gray-500">{String(e.details.api_key ?? '—')}</td>
            <td className="px-4 py-3 text-sm text-gray-500">{e.ip || '—'}</td>
          </tr>
        ))}
      </Table>
    </div>
  )
}
