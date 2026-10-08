import { useEffect, useState } from 'react'
import { cancelExperiment, concludeExperiment, createExperiment, getExperimentResults, getExperiments, pauseExperiment, resumeExperiment, startExperiment } from '../api/client'
import type { CohortStats, Experiment, ExperimentResult } from '../types/api'
import { useCatalog, type Catalog } from '../hooks/useCatalog'
import { Button, Card, ConfirmDialog, Field, Modal, PageHeader, RowAction, Select, StatusBadge, Table, TextInput, errorMessage, formatLabels, parseLabels } from '../components/ui'

function NewExperimentForm({ catalog, onClose, onCreated }: { catalog: Catalog; onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('')
  const [control, setControl] = useState('')
  const [treatment, setTreatment] = useState('')
  const [labels, setLabels] = useState('')
  const [traffic, setTraffic] = useState('100')
  const [split, setSplit] = useState('50')
  const [minSample, setMinSample] = useState('100')
  const [maxDuration, setMaxDuration] = useState('7d')
  const [start, setStart] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const controlVersion = catalog.byVersionId[control]
  // Both arms are versions of the same model.
  const treatmentOptions = catalog.versions.filter(v => controlVersion && v.model_id === controlVersion.model_id && v.id !== control)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!controlVersion || !treatment) { setError('Choose a control and a treatment version.'); return }
    setBusy(true)
    try {
      const exp = await createExperiment({
        project_id: controlVersion.project.id, name: name || `${controlVersion.model.name} ${controlVersion.version} vs ${catalog.byVersionId[treatment].version}`,
        control_version_id: control, treatment_version_id: treatment, target_selector: parseLabels(labels),
        traffic_percent: Number(traffic), split_ratio: Number(split), min_sample_size: Number(minSample), max_duration: maxDuration || undefined,
      })
      if (start) await startExperiment(exp.id)
      onCreated()
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal title="New A/B experiment" onClose={onClose}>
      <form onSubmit={submit}>
        <Field label="Name"><TextInput name="name" value={name} onChange={e => setName(e.target.value)} placeholder="optional" /></Field>
        <Field label="Control version">
          <Select name="control" value={control} onChange={v => { setControl(v); setTreatment('') }} placeholder="Choose a version"
            options={catalog.versions.map(v => ({ value: v.id, label: `${v.label} (${v.project.name})` }))} />
        </Field>
        <Field label="Treatment version">
          <Select name="treatment" value={treatment} onChange={setTreatment} placeholder={controlVersion ? 'Choose a version' : 'Choose the control first'}
            options={treatmentOptions.map(v => ({ value: v.id, label: v.label }))} />
        </Field>
        <Field label="Target devices" hint="Labels as key=value, comma separated. Empty targets every device in the project.">
          <TextInput name="labels" value={labels} onChange={e => setLabels(e.target.value)} placeholder="app=sample" />
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Devices in the experiment (%)"><TextInput name="traffic" type="number" min={1} max={100} value={traffic} onChange={e => setTraffic(e.target.value)} /></Field>
          <Field label="Treatment share (%)"><TextInput name="split" type="number" min={1} max={99} value={split} onChange={e => setSplit(e.target.value)} /></Field>
          <Field label="Min samples per arm" hint="The test runs once both arms reach it."><TextInput name="min_sample" type="number" min={2} value={minSample} onChange={e => setMinSample(e.target.value)} /></Field>
          <Field label="Max duration" hint="e.g. 7d or 12h"><TextInput name="max_duration" value={maxDuration} onChange={e => setMaxDuration(e.target.value)} /></Field>
        </div>
        <label className="flex items-center gap-2 text-sm text-gray-700 mb-2">
          <input type="checkbox" checked={start} onChange={e => setStart(e.target.checked)} /> Start the experiment now
        </label>
        {error && <p className="text-sm text-red-600 mb-2">{error}</p>}
        <div className="flex justify-end gap-2 mt-4">
          <Button variant="secondary" onClick={onClose}>Close</Button>
          <Button type="submit" disabled={busy}>Create experiment</Button>
        </div>
      </form>
    </Modal>
  )
}

function Arm({ title, s }: { title: string; s: CohortStats }) {
  const rows: [string, string][] = [
    ['Samples', s.sample_size.toLocaleString()],
    ['Avg latency', `${s.avg_latency_ms.toFixed(1)} ms`],
    ['p95 latency', `${s.p95_latency_ms.toFixed(1)} ms`],
    ['Error rate', `${(s.error_rate * 100).toFixed(2)}%`],
    ['Avg confidence', s.avg_confidence.toFixed(3)],
  ]
  return (
    <div className="border border-gray-200 rounded-md p-3">
      <div className="text-sm font-semibold text-gray-900 mb-2">{title}</div>
      {rows.map(([k, v]) => <div key={k} className="flex justify-between text-sm"><span className="text-gray-500">{k}</span><span className="text-gray-900">{v}</span></div>)}
    </div>
  )
}

function Results({ exp, r, label }: { exp: Experiment; r: ExperimentResult; label: (id: string) => string }) {
  return (
    <div className="mt-4 border border-gray-200 rounded-lg shadow-sm p-5">
      <h3 className="text-sm font-semibold text-gray-900 mb-3">Results: {exp.name}</h3>
      <div className="grid sm:grid-cols-2 gap-3 mb-3">
        <Arm title={`Control · ${label(exp.control_version_id)}`} s={r.control} />
        <Arm title={`Treatment · ${label(exp.treatment_version_id)}`} s={r.treatment} />
      </div>
      <div className="text-sm text-gray-700 space-y-1">
        <p><span className="text-gray-500">Test: </span>{r.final ? `p ${r.p_value !== null && r.p_value < 0.0001 ? '< 0.0001' : `= ${r.p_value?.toFixed(4)}`}, ${r.is_significant ? 'significant' : 'not significant'}, effect size ${r.effect_size?.toFixed(3)}` : `waiting for ${r.min_sample_size} samples per arm`}</p>
        <p><span className="text-gray-500">Recommendation: </span>{r.recommendation}</p>
      </div>
    </div>
  )
}

type Pending = { kind: 'conclude' | 'cancel'; e: Experiment }

export default function Experiments() {
  const catalog = useCatalog()
  const [exps, setExps] = useState<Experiment[]>([])
  const [selected, setSelected] = useState<{ exp: Experiment; r: ExperimentResult } | null>(null)
  const [creating, setCreating] = useState(false)
  const [pending, setPending] = useState<Pending | null>(null)
  const [winner, setWinner] = useState('')
  const [note, setNote] = useState('')
  const [error, setError] = useState('')
  const [dialogError, setDialogError] = useState('')

  const reload = () => getExperiments().then(e => setExps(e || [])).catch(e => setError(errorMessage(e)))
  useEffect(() => { reload() }, [])
  const label = (id: string) => catalog.byVersionId[id]?.label || id.slice(0, 8)
  const act = (fn: () => Promise<unknown>) => fn().then(reload).catch(e => setError(errorMessage(e)))
  const arm = (e: Experiment, id: string) => `${label(id)}${e.winner_version_id === id ? ' (winner)' : ''}`
  const actions = (e: Experiment) => (
    <>
      <RowAction onClick={() => showResults(e)}>Results</RowAction>
      {e.status === 'DRAFT' && <RowAction onClick={() => act(() => startExperiment(e.id))}>Start</RowAction>}
      {e.status === 'RUNNING' && <RowAction tone="amber" onClick={() => act(() => pauseExperiment(e.id))}>Pause</RowAction>}
      {e.status === 'PAUSED' && <RowAction onClick={() => act(() => resumeExperiment(e.id))}>Resume</RowAction>}
      {['RUNNING', 'PAUSED'].includes(e.status) && <RowAction onClick={() => ask('conclude', e)}>Conclude</RowAction>}
      {['DRAFT', 'RUNNING', 'PAUSED'].includes(e.status) && <RowAction tone="red" onClick={() => ask('cancel', e)}>Cancel</RowAction>}
    </>
  )
  const showResults = (exp: Experiment) => getExperimentResults(exp.id).then(r => setSelected({ exp, r })).catch(e => setError(errorMessage(e)))

  const ask = async (kind: Pending['kind'], e: Experiment) => {
    setPending({ kind, e }); setNote(''); setDialogError(''); setWinner(e.control_version_id)
    if (kind === 'conclude') {
      // Preselect the arm the statistics favour, if the test has run.
      const r = await getExperimentResults(e.id).catch(() => null)
      if (r?.final && r.is_significant && /treatment/i.test(r.recommendation)) setWinner(e.treatment_version_id)
    }
  }
  const confirm = async () => {
    if (!pending) return
    try {
      if (pending.kind === 'conclude') await concludeExperiment(pending.e.id, winner, note)
      else await cancelExperiment(pending.e.id)
      setPending(null)
      reload()
    } catch (e) {
      setDialogError(errorMessage(e))
    }
  }

  return (
    <div>
      <PageHeader title="Experiments" action={<Button onClick={() => setCreating(true)}>New experiment</Button>} />
      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      <Table head={['Name', 'Status', 'Control', 'Treatment', 'Target', '']} empty={exps.length === 0 ? 'No experiments' : undefined}
        mobile={exps.map(e => (
          <Card key={e.id} title={e.name} badge={<StatusBadge status={e.status} />}
            lines={[['Control', arm(e, e.control_version_id)], ['Treatment', arm(e, e.treatment_version_id)], ['Target', formatLabels(e.target_selector)]]}
            actions={actions(e)} />
        ))}>
        {exps.map(e => (
          <tr key={e.id} className="border-b border-gray-100 hover:bg-gray-50">
            <td className="px-4 py-3 text-sm font-medium text-gray-900">{e.name}</td>
            <td className="px-4 py-3"><StatusBadge status={e.status} /></td>
            <td className="px-4 py-3 text-sm text-gray-700">{arm(e, e.control_version_id)}</td>
            <td className="px-4 py-3 text-sm text-gray-700">{arm(e, e.treatment_version_id)}</td>
            <td className="px-4 py-3 text-sm text-gray-500">{formatLabels(e.target_selector)}</td>
            <td className="px-4 py-3"><div className="flex gap-3">{actions(e)}</div></td>
          </tr>
        ))}
      </Table>
      {selected && <Results exp={selected.exp} r={selected.r} label={label} />}

      {creating && <NewExperimentForm catalog={catalog} onClose={() => setCreating(false)} onCreated={() => { setCreating(false); reload() }} />}
      {pending?.kind === 'conclude' && (
        <ConfirmDialog title="Conclude this experiment?" confirmLabel="Conclude" onConfirm={confirm} onClose={() => setPending(null)} error={dialogError}
          message={<p>Devices leave the experiment on their next heartbeat and return to their deployments. To roll the winner out, create a deployment for it.</p>}>
          <div className="mt-3">
            <Field label="Winner">
              <Select name="winner" value={winner} onChange={setWinner} options={[
                { value: pending.e.control_version_id, label: `Control · ${label(pending.e.control_version_id)}` },
                { value: pending.e.treatment_version_id, label: `Treatment · ${label(pending.e.treatment_version_id)}` },
              ]} />
            </Field>
            <Field label="Note"><TextInput name="note" value={note} onChange={e => setNote(e.target.value)} placeholder="optional" /></Field>
          </div>
        </ConfirmDialog>
      )}
      {pending?.kind === 'cancel' && (
        <ConfirmDialog title="Cancel this experiment?" confirmLabel="Cancel experiment" danger onConfirm={confirm} onClose={() => setPending(null)} error={dialogError}
          message={<p>Devices leave <strong>{pending.e.name}</strong> on their next heartbeat. Its results are kept, but it cannot be restarted.</p>} />
      )}
    </div>
  )
}
