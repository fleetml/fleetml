import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { cancelDeployment, createDeployment, getDeployments, pauseDeployment, resumeDeployment, rollbackDeployment, startDeployment, type NewDeployment } from '../api/client'
import type { Deployment } from '../types/api'
import { useCatalog, type Catalog } from '../hooks/useCatalog'
import { Button, Card, ConfirmDialog, Field, Modal, PageHeader, RowAction, Select, StatusBadge, Table, TextInput, errorMessage, formatLabels, parseLabels, shortDate } from '../components/ui'

function progress(d: Deployment): string {
  if (d.strategy === 'CANARY' && d.canary_stages?.length) {
    const i = Math.min(d.current_stage, d.canary_stages.length - 1)
    return `stage ${i + 1}/${d.canary_stages.length} · ${d.canary_stages[i].percentage}%`
  }
  if (d.strategy === 'ROLLING' && d.rolling_config) {
    const r = d.rolling_config
    const size = r.batch_size ? `${r.batch_size} devices` : `${r.batch_size_percent}%`
    return `${size} every ${r.batch_interval}`
  }
  return '–'
}

function NewDeploymentForm({ catalog, initialVersion, onClose, onCreated }: {
  catalog: Catalog; initialVersion: string; onClose: () => void; onCreated: () => void
}) {
  const [versionId, setVersionId] = useState(initialVersion)
  const [strategy, setStrategy] = useState<NewDeployment['strategy']>('CANARY')
  const [labels, setLabels] = useState('')
  const [stages, setStages] = useState('10, 50, 100')
  const [stageTime, setStageTime] = useState('10m')
  const [batchPercent, setBatchPercent] = useState('20')
  const [batchInterval, setBatchInterval] = useState('1m')
  const [maxErrorRate, setMaxErrorRate] = useState('5')
  const [minEvents, setMinEvents] = useState('20')
  const [onFailure, setOnFailure] = useState('ROLLBACK')
  const [start, setStart] = useState(true)
  const [urgent, setUrgent] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    const v = catalog.byVersionId[versionId]
    if (!v) { setError('Choose a model version.'); return }
    const body: NewDeployment = {
      project_id: v.project.id, model_version_id: v.id, strategy, target_selector: parseLabels(labels),
      name: `${v.label} ${strategy.toLowerCase().replace(/_/g, ' ')}`,
      health_gate: { max_error_rate: Number(maxErrorRate) / 100, min_events: Number(minEvents), on_failure: onFailure },
      urgent,
    }
    if (strategy === 'CANARY') {
      body.canary_stages = stages.split(',').map(s => Number(s.trim())).filter(n => n > 0).map(percentage => ({ percentage, min_duration: stageTime }))
    }
    if (strategy === 'ROLLING') body.rolling_config = { batch_size_percent: Number(batchPercent), batch_interval: batchInterval }
    setBusy(true)
    try {
      const d = await createDeployment(body)
      if (start) await startDeployment(d.id)
      onCreated()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title="New deployment" onClose={onClose}>
      <form onSubmit={submit}>
        <Field label="Model version">
          <Select name="version" value={versionId} onChange={setVersionId} placeholder="Choose a version"
            options={catalog.versions.map(v => ({ value: v.id, label: `${v.label} (${v.project.name}, ${v.status})` }))} />
        </Field>
        <Field label="Strategy">
          <Select name="strategy" value={strategy} onChange={v => setStrategy(v as NewDeployment['strategy'])} options={[
            { value: 'CANARY', label: 'Canary: staged percentages with a health gate' },
            { value: 'ROLLING', label: 'Rolling: batches at an interval' },
            { value: 'ALL_AT_ONCE', label: 'All at once' },
          ]} />
        </Field>
        <Field label="Target devices" hint="Labels as key=value, comma separated. Empty targets every device in the project.">
          <TextInput name="labels" value={labels} onChange={e => setLabels(e.target.value)} placeholder="app=sample, region=eu" />
        </Field>
        {strategy === 'CANARY' && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Stages (%)"><TextInput name="stages" value={stages} onChange={e => setStages(e.target.value)} /></Field>
            <Field label="Minimum time per stage"><TextInput name="stage_time" value={stageTime} onChange={e => setStageTime(e.target.value)} /></Field>
          </div>
        )}
        {strategy === 'ROLLING' && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Batch size (% of devices)"><TextInput name="batch_percent" type="number" min={1} max={100} value={batchPercent} onChange={e => setBatchPercent(e.target.value)} /></Field>
            <Field label="Batch interval"><TextInput name="batch_interval" value={batchInterval} onChange={e => setBatchInterval(e.target.value)} /></Field>
          </div>
        )}
        {strategy !== 'ALL_AT_ONCE' && (
          <div className="grid grid-cols-3 gap-3">
            <Field label="Max error rate (%)"><TextInput name="max_error_rate" type="number" min={0} max={100} step="0.1" value={maxErrorRate} onChange={e => setMaxErrorRate(e.target.value)} /></Field>
            <Field label="Min events"><TextInput name="min_events" type="number" min={1} value={minEvents} onChange={e => setMinEvents(e.target.value)} /></Field>
            <Field label="On failure">
              <Select name="on_failure" value={onFailure} onChange={setOnFailure} options={[{ value: 'ROLLBACK', label: 'Roll back' }, { value: 'PAUSE', label: 'Pause' }]} />
            </Field>
          </div>
        )}
        <label className="flex items-center gap-2 text-sm text-gray-700 mb-2">
          <input type="checkbox" name="urgent" checked={urgent} onChange={e => setUrgent(e.target.checked)} />
          Urgent: devices download on any network, ignoring their download policy
        </label>
        <label className="flex items-center gap-2 text-sm text-gray-700 mb-2">
          <input type="checkbox" checked={start} onChange={e => setStart(e.target.checked)} /> Start the rollout now
        </label>
        {error && <p className="text-sm text-red-600 mb-2">{error}</p>}
        <div className="flex justify-end gap-2 mt-4">
          <Button variant="secondary" onClick={onClose}>Close</Button>
          <Button type="submit" disabled={busy}>Create deployment</Button>
        </div>
      </form>
    </Modal>
  )
}

type Pending = { kind: 'rollback' | 'cancel'; d: Deployment }

export default function Deployments() {
  const catalog = useCatalog()
  const [params, setParams] = useSearchParams()
  const [deps, setDeps] = useState<Deployment[]>([])
  const [error, setError] = useState('')
  const [pending, setPending] = useState<Pending | null>(null)
  const [reason, setReason] = useState('')
  const [dialogError, setDialogError] = useState('')
  const newVersion = params.get('new')
  const [creating, setCreating] = useState(newVersion !== null)

  const reload = () => getDeployments().then(d => setDeps(d || [])).catch(e => setError(errorMessage(e)))
  useEffect(() => {
    reload()
    const t = setInterval(reload, 5000)
    return () => clearInterval(t)
  }, [])

  const act = (fn: () => Promise<unknown>) => fn().then(reload).catch(e => setError(errorMessage(e)))
  const closeForm = () => { setCreating(false); if (newVersion !== null) setParams({}) }
  const confirm = async () => {
    if (!pending) return
    try {
      if (pending.kind === 'rollback') await rollbackDeployment(pending.d.id, reason || 'rolled back from the dashboard')
      else await cancelDeployment(pending.d.id)
      setPending(null)
      reload()
    } catch (e) {
      setDialogError(errorMessage(e))
    }
  }
  const ask = (kind: Pending['kind'], d: Deployment) => { setPending({ kind, d }); setReason(''); setDialogError('') }
  const name = (d: Deployment) => catalog.byVersionId[d.model_version_id]?.label || d.name || d.model_version_id.slice(0, 8)
  const actions = (d: Deployment) => (
    <>
      {d.status === 'PENDING' && <RowAction onClick={() => act(() => startDeployment(d.id))}>Start</RowAction>}
      {d.status === 'ROLLING_OUT' && <RowAction tone="amber" onClick={() => act(() => pauseDeployment(d.id))}>Pause</RowAction>}
      {d.status === 'PAUSED' && <RowAction onClick={() => act(() => resumeDeployment(d.id))}>Resume</RowAction>}
      {['ROLLING_OUT', 'PAUSED', 'COMPLETE'].includes(d.status) && <RowAction tone="red" onClick={() => ask('rollback', d)}>Rollback</RowAction>}
      {['PENDING', 'ROLLING_OUT', 'PAUSED'].includes(d.status) && <RowAction tone="red" onClick={() => ask('cancel', d)}>Cancel</RowAction>}
    </>
  )

  return (
    <div>
      <PageHeader title="Deployments" action={<Button onClick={() => setCreating(true)}>New deployment</Button>} />
      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      <Table head={['Model', 'Strategy', 'Status', 'Progress', 'Target', 'Created', '']} empty={deps.length === 0 ? 'No deployments' : undefined}
        mobile={deps.map(d => (
          <Card key={d.id} title={name(d)} subtitle={d.name !== name(d) ? d.name : undefined} badge={<StatusBadge status={d.status} />}
            lines={[['Strategy', d.strategy.replace(/_/g, ' ') + (d.urgent ? ' (urgent)' : '')], ['Progress', progress(d)], ['Target', formatLabels(d.target_selector)], ['Created', shortDate(d.created_at)],
              ...(d.rollback_reason ? [['Reason', d.rollback_reason] as [string, string]] : [])]}
            actions={actions(d)} />
        ))}>
        {deps.map(d => (
          <tr key={d.id} className="border-b border-gray-100 hover:bg-gray-50 align-top">
            <td className="px-4 py-3 text-sm font-medium text-gray-900 whitespace-nowrap">
              {name(d)}
              {d.name && d.name !== name(d) && <div className="text-xs font-normal text-gray-500">{d.name}</div>}
            </td>
            <td className="px-4 py-3 text-sm text-gray-700">
              {d.strategy.replace(/_/g, ' ')}
              {d.urgent && <div className="text-xs font-medium text-amber-700">urgent</div>}
            </td>
            <td className="px-4 py-3">
              <StatusBadge status={d.status} />
              {d.rollback_reason && <div className="mt-1 max-w-56 text-xs text-gray-500">{d.rollback_reason}</div>}
            </td>
            <td className="px-4 py-3 text-sm text-gray-700">{progress(d)}</td>
            <td className="px-4 py-3 text-sm text-gray-500">{formatLabels(d.target_selector)}</td>
            <td className="px-4 py-3 text-sm text-gray-500 whitespace-nowrap">{shortDate(d.created_at)}</td>
            <td className="px-4 py-3"><div className="flex gap-3">{actions(d)}</div></td>
          </tr>
        ))}
      </Table>

      {creating && (
        <NewDeploymentForm catalog={catalog} initialVersion={newVersion || ''} onClose={closeForm}
          onCreated={() => { closeForm(); reload() }} />
      )}
      {pending?.kind === 'rollback' && (
        <ConfirmDialog title="Roll back this deployment?" confirmLabel="Roll back" danger onConfirm={confirm} onClose={() => setPending(null)} error={dialogError}
          message={<p>Devices on <strong>{name(pending.d)}</strong> go back to the previous version on their next heartbeat (or unload the model if there was none), and a rollback alert is sent. This cannot be undone; deploy again to retry.</p>}>
          <div className="mt-3"><Field label="Reason"><TextInput name="reason" value={reason} onChange={e => setReason(e.target.value)} placeholder="e.g. accuracy regression reported" /></Field></div>
        </ConfirmDialog>
      )}
      {pending?.kind === 'cancel' && (
        <ConfirmDialog title="Cancel this deployment?" confirmLabel="Cancel deployment" danger onConfirm={confirm} onClose={() => setPending(null)} error={dialogError}
          message={<p>The rollout of <strong>{name(pending.d)}</strong> stops and devices it reached return to whatever deployment applied before. No alert is sent.</p>} />
      )}
    </div>
  )
}
