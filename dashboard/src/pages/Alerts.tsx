import { useEffect, useState } from 'react'
import { createAlertSink, deleteAlertSink, getAlertDeliveries, getAlertSinks, getProjects, testAlertSink, updateAlertSink } from '../api/client'
import type { AlertDelivery, AlertSink, Project } from '../types/api'
import { Button, Card, ConfirmDialog, Field, Modal, PageHeader, RowAction, Select, StatusBadge, Table, TextInput, errorMessage, shortDate } from '../components/ui'

const kinds: { value: AlertSink['kind']; label: string }[] = [
  { value: 'webhook', label: 'Webhook' },
  { value: 'slack', label: 'Slack' },
  { value: 'email', label: 'Email' },
]

/** The sink's destination, as the API shows it (secrets redacted). */
const target = (s: AlertSink) => s.config.url || s.config.webhook_url || s.config.to || ''

function NewSinkForm({ projectId, onClose, onCreated }: { projectId: string; onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('')
  const [kind, setKind] = useState<AlertSink['kind']>('webhook')
  const [url, setUrl] = useState('')
  const [secret, setSecret] = useState('')
  const [to, setTo] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    const config: Record<string, string> =
      kind === 'webhook' ? { url, ...(secret ? { secret } : {}) } : kind === 'slack' ? { webhook_url: url } : { to }
    setBusy(true)
    try {
      await createAlertSink(projectId, { name, kind, config })
      onCreated()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title="New alert sink" onClose={onClose}>
      <form onSubmit={submit}>
        <Field label="Name"><TextInput name="name" value={name} onChange={e => setName(e.target.value)} placeholder="on-call" required /></Field>
        <Field label="Kind"><Select name="kind" value={kind} onChange={v => setKind(v as AlertSink['kind'])} options={kinds} /></Field>
        {kind === 'webhook' && <>
          <Field label="URL" hint="Receives the alert JSON as a POST."><TextInput name="url" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://example.com/fleetml" /></Field>
          <Field label="Signing secret" hint="Optional. Signs the body: X-FleetML-Signature: sha256=<HMAC>."><TextInput name="secret" value={secret} onChange={e => setSecret(e.target.value)} /></Field>
        </>}
        {kind === 'slack' && (
          <Field label="Incoming webhook URL" hint="From a Slack app with incoming webhooks enabled.">
            <TextInput name="webhook_url" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://hooks.slack.com/services/…" />
          </Field>
        )}
        {kind === 'email' && (
          <Field label="Recipients" hint="Comma separated. Needs SMTP_HOST and SMTP_FROM on the server.">
            <TextInput name="to" value={to} onChange={e => setTo(e.target.value)} placeholder="oncall@example.com" />
          </Field>
        )}
        {error && <p className="text-sm text-red-600 mb-2">{error}</p>}
        <div className="flex justify-end gap-2 mt-4">
          <Button variant="secondary" onClick={onClose}>Close</Button>
          <Button type="submit" disabled={busy}>Create sink</Button>
        </div>
      </form>
    </Modal>
  )
}

function DeliveryLog({ sink, deliveries }: { sink: AlertSink; deliveries: AlertDelivery[] }) {
  return (
    <div className="mt-6">
      <h3 className="text-sm font-semibold text-gray-900 mb-2">Delivery log: {sink.name}</h3>
      <p className="text-xs text-gray-500 mb-3">The last 50 attempts. An alert is tried up to 5 times; a final failure is marked dead.</p>
      <Table head={['Time', 'Alert', 'Attempt', 'Status', 'Error']} empty={deliveries.length === 0 ? 'No deliveries yet' : undefined}
        mobile={deliveries.map(d => (
          <Card key={d.id} title={`${d.alert_type} · attempt ${d.attempt}`} subtitle={shortDate(d.created_at)} badge={<StatusBadge status={d.status} />}
            lines={[['Alert', d.alert_id.slice(0, 8)], ['Error', d.error || '']]} />
        ))}>
        {deliveries.map(d => (
          <tr key={d.id} className="border-b border-gray-100">
            <td className="px-4 py-3 text-sm text-gray-500 whitespace-nowrap">{shortDate(d.created_at)}</td>
            <td className="px-4 py-3 text-sm text-gray-700">{d.alert_type} <span className="text-gray-400">{d.alert_id.slice(0, 8)}</span></td>
            <td className="px-4 py-3 text-sm text-gray-700">{d.attempt}</td>
            <td className="px-4 py-3"><StatusBadge status={d.status} /></td>
            <td className="px-4 py-3 text-sm text-gray-500 break-words">{d.error}</td>
          </tr>
        ))}
      </Table>
    </div>
  )
}

export default function Alerts() {
  const [projects, setProjects] = useState<Project[]>([])
  const [projectId, setProjectId] = useState('')
  const [sinks, setSinks] = useState<AlertSink[]>([])
  const [selected, setSelected] = useState<AlertSink | null>(null)
  const [deliveries, setDeliveries] = useState<AlertDelivery[]>([])
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<AlertSink | null>(null)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    getProjects().then(p => { setProjects(p || []); if (p?.length) setProjectId(p[0].id) }).catch(e => setError(errorMessage(e)))
  }, [])
  const reload = () => {
    if (projectId) getAlertSinks(projectId).then(s => setSinks(s || [])).catch(e => setError(errorMessage(e)))
  }
  useEffect(() => { reload(); setSelected(null) }, [projectId])
  const showLog = (s: AlertSink) => getAlertDeliveries(s.id).then(d => { setSelected(s); setDeliveries(d || []) }).catch(e => setError(errorMessage(e)))

  const test = async (s: AlertSink) => {
    setNotice(''); setError('')
    try {
      const d = await testAlertSink(s.id)
      setNotice(d.status === 'ok' ? `Test alert delivered to ${s.name}.` : `Test alert to ${s.name} failed: ${d.error}`)
      showLog(s)
    } catch (e) {
      setError(errorMessage(e))
    }
  }
  const toggle = (s: AlertSink) => updateAlertSink(s.id, { enabled: !s.enabled }).then(reload).catch(e => setError(errorMessage(e)))
  const remove = async () => {
    if (!deleting) return
    try {
      await deleteAlertSink(deleting.id)
      if (selected?.id === deleting.id) setSelected(null)
      setDeleting(null)
      reload()
    } catch (e) {
      setError(errorMessage(e))
    }
  }
  const actions = (s: AlertSink) => (
    <>
      <RowAction onClick={() => test(s)}>Send test</RowAction>
      <RowAction onClick={() => showLog(s)}>Log</RowAction>
      <RowAction tone="amber" onClick={() => toggle(s)}>{s.enabled ? 'Disable' : 'Enable'}</RowAction>
      <RowAction tone="red" onClick={() => setDeleting(s)}>Delete</RowAction>
    </>
  )

  return (
    <div>
      <PageHeader title="Alerts" action={projectId && <Button onClick={() => setCreating(true)}>New sink</Button>} />
      <p className="text-sm text-gray-500 mb-4">Rollbacks and drift that becomes CRITICAL are sent to every enabled sink of the project.</p>
      {projects.length > 1 && (
        <div className="max-w-xs"><Field label="Project"><Select name="project" value={projectId} onChange={setProjectId} options={projects.map(p => ({ value: p.id, label: p.name }))} /></Field></div>
      )}
      {notice && <p className="text-sm text-gray-700 mb-4" role="status">{notice}</p>}
      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      <Table head={['Name', 'Kind', 'Destination', 'Enabled', '']} empty={sinks.length === 0 ? 'No alert sinks' : undefined}
        mobile={sinks.map(s => (
          <Card key={s.id} title={s.name} subtitle={s.kind} badge={s.enabled ? undefined : <StatusBadge status="DISABLED" />}
            lines={[['Target', target(s)]]} actions={actions(s)} />
        ))}>
        {sinks.map(s => (
          <tr key={s.id} className="border-b border-gray-100 hover:bg-gray-50">
            <td className="px-4 py-3 text-sm font-medium text-gray-900">{s.name}</td>
            <td className="px-4 py-3 text-sm text-gray-700">{s.kind}</td>
            <td className="px-4 py-3 text-sm text-gray-500 break-all">{target(s)}</td>
            <td className="px-4 py-3 text-sm text-gray-700">{s.enabled ? 'Yes' : 'No'}</td>
            <td className="px-4 py-3"><div className="flex gap-3">{actions(s)}</div></td>
          </tr>
        ))}
      </Table>
      {selected && <DeliveryLog sink={selected} deliveries={deliveries} />}

      {creating && <NewSinkForm projectId={projectId} onClose={() => setCreating(false)} onCreated={() => { setCreating(false); reload() }} />}
      {deleting && (
        <ConfirmDialog title="Delete this sink?" confirmLabel="Delete sink" danger onConfirm={remove} onClose={() => setDeleting(null)}
          message={<p><strong>{deleting.name}</strong> stops receiving alerts and its delivery log is deleted.</p>} />
      )}
    </div>
  )
}
