import { useState } from 'react'
import { login } from '../api/client'
import type { SessionInfo } from '../types/api'

export default function Login({ notice, onLoggedIn }: { notice: string; onLoggedIn: (s: SessionInfo) => void }) {
  const [key, setKey] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const s = await login(key)
      setKey('')
      onLoggedIn(s)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center px-4">
      <form onSubmit={submit} className="w-full max-w-sm bg-white border border-gray-200 rounded-lg shadow-sm p-6">
        <h1 className="text-xl font-bold text-gray-900 mb-1">FleetML</h1>
        <p className="text-sm text-gray-500 mb-5">Log in with your API key. The key is exchanged for a session; it is not stored in this browser.</p>
        {notice && <p role="status" className="mb-4 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">{notice}</p>}
        <label className="block mb-4">
          <span className="block text-sm font-medium text-gray-700 mb-1">API key</span>
          <input type="password" name="api_key" autoComplete="off" autoFocus value={key} onChange={e => setKey(e.target.value)}
            className="w-full border border-gray-300 rounded-md px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-indigo-500" />
          <span className="block text-xs text-gray-500 mt-1">
            <code>FLEETML_ADMIN_KEY</code> from your <code>.env</code>, the server log on first start, or <code>POST /api/v1/api-keys</code>.
          </span>
        </label>
        {error && <p role="alert" className="mb-3 text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={!key.trim() || busy}
          className="w-full px-4 py-2 rounded-md bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-700 disabled:opacity-50">
          {busy ? 'Logging in…' : 'Log in'}
        </button>
      </form>
    </div>
  )
}
