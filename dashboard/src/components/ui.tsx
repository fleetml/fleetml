import { useEffect, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { UnauthorizedError } from '../api/auth'

const badgeStyle: Record<string, string> = {
  // deployments
  PENDING: 'bg-gray-100 text-gray-600',
  ROLLING_OUT: 'bg-indigo-50 text-indigo-700',
  PAUSED: 'bg-amber-50 text-amber-700',
  COMPLETE: 'bg-green-50 text-green-700',
  ROLLED_BACK: 'bg-red-50 text-red-700',
  FAILED: 'bg-red-50 text-red-700',
  CANCELLED: 'bg-gray-100 text-gray-500',
  // experiments
  DRAFT: 'bg-gray-100 text-gray-600',
  RUNNING: 'bg-indigo-50 text-indigo-700',
  CONCLUDED: 'bg-green-50 text-green-700',
  // versions and variants
  STAGED: 'bg-amber-50 text-amber-700',
  ACTIVE: 'bg-green-50 text-green-700',
  ARCHIVED: 'bg-gray-100 text-gray-500',
  READY: 'bg-green-50 text-green-700',
  UPLOADING: 'bg-amber-50 text-amber-700',
  // drift
  NONE: 'bg-green-50 text-green-700',
  WARNING: 'bg-amber-50 text-amber-700',
  CRITICAL: 'bg-red-50 text-red-700',
}

export function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`inline-flex px-2 py-0.5 rounded text-xs font-medium whitespace-nowrap ${badgeStyle[status] || 'bg-gray-100 text-gray-600'}`}>
      {status}
    </span>
  )
}

export function PageHeader({ title, action }: { title: string; action?: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 mb-6">
      <h2 className="text-2xl font-semibold text-gray-900">{title}</h2>
      {action}
    </div>
  )
}

export function Button({ children, onClick, variant = 'primary', type = 'button', disabled }: {
  children: ReactNode; onClick?: () => void; variant?: 'primary' | 'secondary' | 'danger'; type?: 'button' | 'submit'; disabled?: boolean
}) {
  const style = {
    primary: 'bg-indigo-600 text-white hover:bg-indigo-700',
    secondary: 'border border-gray-300 text-gray-700 hover:bg-gray-50',
    danger: 'bg-red-600 text-white hover:bg-red-700',
  }[variant]
  return (
    <button type={type} onClick={onClick} disabled={disabled}
      className={`px-4 py-2 rounded-md text-sm font-medium whitespace-nowrap disabled:opacity-50 ${style}`}>
      {children}
    </button>
  )
}

/** A text button for table rows. */
export function RowAction({ children, onClick, tone = 'indigo' }: { children: ReactNode; onClick: () => void; tone?: 'indigo' | 'amber' | 'red' }) {
  const style = { indigo: 'text-indigo-600 hover:text-indigo-800', amber: 'text-amber-600 hover:text-amber-800', red: 'text-red-600 hover:text-red-800' }[tone]
  return <button onClick={onClick} className={`text-sm whitespace-nowrap ${style}`}>{children}</button>
}

export function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  return (
    <div className="fixed inset-0 z-50 flex items-start sm:items-center justify-center bg-black/30 p-4 overflow-y-auto" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label={title} className="w-full max-w-lg rounded-lg bg-white shadow-xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between border-b border-gray-200 px-5 py-4">
          <h3 className="text-base font-semibold text-gray-900">{title}</h3>
          <button onClick={onClose} aria-label="Close" className="text-gray-400 hover:text-gray-600"><X size={18} /></button>
        </div>
        <div className="px-5 py-4">{children}</div>
      </div>
    </div>
  )
}

/** Asks before a consequential action; the action's error is shown in the dialog. */
export function ConfirmDialog({ title, message, confirmLabel, danger, onConfirm, onClose, children, error, busy }: {
  title: string; message: ReactNode; confirmLabel: string; danger?: boolean; onConfirm: () => void; onClose: () => void
  children?: ReactNode; error?: string; busy?: boolean
}) {
  return (
    <Modal title={title} onClose={onClose}>
      <div className="text-sm text-gray-700">{message}</div>
      {children}
      {error && <p className="mt-3 text-sm text-red-600">{error}</p>}
      <div className="mt-5 flex justify-end gap-2">
        <Button variant="secondary" onClick={onClose}>Keep it</Button>
        <Button variant={danger ? 'danger' : 'primary'} onClick={onConfirm} disabled={busy}>{confirmLabel}</Button>
      </div>
    </Modal>
  )
}

const inputClass = 'w-full border border-gray-300 rounded-md px-3 py-2 text-sm bg-white focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent'

export function Field({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <label className="block mb-3">
      <span className="block text-sm font-medium text-gray-700 mb-1">{label}</span>
      {children}
      {hint && <span className="block text-xs text-gray-500 mt-1">{hint}</span>}
    </label>
  )
}

export function TextInput(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={inputClass} />
}

export function Select({ value, onChange, options, placeholder, name }: {
  value: string; onChange: (v: string) => void; options: { value: string; label: string }[]; placeholder?: string; name?: string
}) {
  return (
    <select name={name} value={value} onChange={e => onChange(e.target.value)} className={inputClass}>
      {placeholder !== undefined && <option value="">{placeholder}</option>}
      {options.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
    </select>
  )
}

/** One record as a card, for phones (pass cards to Table as `mobile`). */
export function Card({ title, subtitle, badge, lines, actions }: {
  title: ReactNode; subtitle?: ReactNode; badge?: ReactNode; lines: [string, ReactNode][]; actions?: ReactNode
}) {
  return (
    <div className="border border-gray-200 rounded-lg shadow-sm p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="text-sm font-semibold text-gray-900">{title}</div>
          {subtitle && <div className="text-xs text-gray-500">{subtitle}</div>}
        </div>
        {badge}
      </div>
      <dl className="mt-2 space-y-1 text-sm">
        {lines.map(([k, v]) => <div key={k} className="flex gap-2"><dt className="w-20 shrink-0 text-gray-500">{k}</dt><dd className="text-gray-700 min-w-0 break-words">{v}</dd></div>)}
      </dl>
      {actions && <div className="mt-3 flex flex-wrap gap-4 empty:hidden">{actions}</div>}
    </div>
  )
}

/** A bordered table that scrolls sideways on narrow screens, or shows `mobile` cards there instead. */
export function Table({ head, children, empty, mobile }: { head: string[]; children: ReactNode; empty?: string; mobile?: ReactNode }) {
  if (mobile !== undefined) {
    return (
      <>
        <div className="md:hidden space-y-3">
          {mobile}
          {empty && <p className="py-6 text-center text-sm text-gray-400">{empty}</p>}
        </div>
        <div className="hidden md:block"><Table head={head} empty={empty}>{children}</Table></div>
      </>
    )
  }
  return (
    <div className="border border-gray-200 rounded-lg shadow-sm overflow-x-auto">
      <table className="w-full">
        <thead>
          <tr className="bg-gray-50 border-b border-gray-200">
            {head.map((h, i) => <th key={i} className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">{h}</th>)}
          </tr>
        </thead>
        <tbody>
          {children}
          {empty && <tr><td colSpan={head.length} className="px-4 py-8 text-center text-sm text-gray-400">{empty}</td></tr>}
        </tbody>
      </table>
    </div>
  )
}

/** Parses "key=value, key2=value2" into a label map. */
export function parseLabels(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const part of text.split(',')) {
    const [k, ...v] = part.split('=')
    if (k.trim() && v.length) out[k.trim()] = v.join('=').trim()
  }
  return out
}

export function formatLabels(labels?: Record<string, string> | null): string {
  const entries = Object.entries(labels || {})
  return entries.length ? entries.map(([k, v]) => `${k}=${v}`).join(', ') : 'all devices'
}

/** The message to show for a failed request; empty for auth errors, which the app banner reports. */
/** A compact local date and time, e.g. "Oct 8, 16:49". */
export function shortDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

export function errorMessage(e: unknown): string {
  if (e instanceof UnauthorizedError) return ''
  return e instanceof Error ? e.message : String(e)
}
