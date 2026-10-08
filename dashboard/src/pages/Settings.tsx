import { useState } from 'react'
import { clearApiKey, getApiKey, setApiKey } from '../api/auth'
import { verifyApiKey } from '../api/client'

type Status = { kind: 'idle' } | { kind: 'checking' } | { kind: 'ok'; text: string } | { kind: 'error'; text: string }

function mask(key: string): string {
  if (!key) return 'not set'
  return key.length <= 8 ? '••••' : `${key.slice(0, 4)}…${key.slice(-4)}`
}

export default function Settings({ onKeySaved }: { onKeySaved: () => void }) {
  const [stored, setStored] = useState(getApiKey())
  const [draft, setDraft] = useState('')
  const [status, setStatus] = useState<Status>({ kind: 'idle' })

  const save = async () => {
    const key = draft.trim()
    if (!key) return
    setStatus({ kind: 'checking' })
    try {
      if (!(await verifyApiKey(key))) {
        setStatus({ kind: 'error', text: 'The API rejected this key (401). Check that you copied it completely and that it has not been revoked.' })
        return
      }
    } catch (e) {
      setStatus({ kind: 'error', text: `Could not reach the API: ${(e as Error).message}` })
      return
    }
    setApiKey(key)
    setStored(key)
    setDraft('')
    setStatus({ kind: 'ok', text: 'Key verified and saved in this browser.' })
    onKeySaved()
  }

  const clear = () => {
    clearApiKey()
    setStored('')
    setStatus({ kind: 'idle' })
  }

  return (
    <div className="max-w-xl">
      <h2 className="text-2xl font-semibold text-gray-900 mb-6">Settings</h2>
      <div className="border border-gray-200 rounded-lg shadow-sm p-5">
        <h3 className="text-sm font-semibold text-gray-900 mb-1">Admin API key</h3>
        <p className="text-sm text-gray-500 mb-4">
          Every API request is sent with this key. It is stored only in this browser's local storage.
          Get one from <code className="text-xs bg-gray-100 px-1 rounded">FLEETML_ADMIN_KEY</code> in your <code className="text-xs bg-gray-100 px-1 rounded">.env</code>,
          the server log on first start, or <code className="text-xs bg-gray-100 px-1 rounded">POST /api/v1/api-keys</code>.
        </p>
        <div className="text-sm mb-3">
          <span className="text-gray-500">Current key: </span>
          <span className="font-mono text-gray-900">{mask(stored)}</span>
        </div>
        <form onSubmit={e => { e.preventDefault(); void save() }} className="flex gap-2">
          <input
            type="password"
            autoComplete="off"
            value={draft}
            onChange={e => setDraft(e.target.value)}
            placeholder="Paste API key"
            aria-label="API key"
            className="flex-1 border border-gray-300 rounded-md px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500"
          />
          <button type="submit" disabled={!draft.trim() || status.kind === 'checking'}
            className="px-4 py-2 rounded-md bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-700 disabled:opacity-50">
            {status.kind === 'checking' ? 'Checking…' : 'Save'}
          </button>
          {stored && (
            <button type="button" onClick={clear} className="px-3 py-2 rounded-md border border-gray-300 text-sm text-gray-700 hover:bg-gray-50">
              Clear
            </button>
          )}
        </form>
        {status.kind === 'ok' && <p className="mt-3 text-sm text-green-700">{status.text}</p>}
        {status.kind === 'error' && <p role="alert" className="mt-3 text-sm text-red-600">{status.text}</p>}
      </div>
    </div>
  )
}
