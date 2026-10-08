import { useEffect, useState } from 'react'
import { getSessions, logout, revokeSession } from '../api/client'
import type { Session } from '../types/api'
import { Button, Card, PageHeader, RowAction, Table, errorMessage, shortDate } from '../components/ui'

type Row = Session & { current: boolean }

/** A short browser description from a user agent string. */
function browserName(ua: string): string {
  const m = ua.match(/(Edg|Firefox|Chrome|Safari)\/[\d.]+/)
  const os = ua.match(/\(([^;)]+)/)?.[1]
  return [m ? m[0].replace('Edg', 'Edge').split('.')[0] : ua.slice(0, 40) || 'unknown', os].filter(Boolean).join(' · ')
}

export default function Settings({ session, onLoggedOut }: { session: Session; onLoggedOut: () => void }) {
  const [sessions, setSessions] = useState<Row[]>([])
  const [error, setError] = useState('')

  const reload = () => getSessions().then(s => setSessions(s || [])).catch(e => setError(errorMessage(e)))
  useEffect(() => { reload() }, [])

  const signOut = () => logout().then(onLoggedOut).catch(e => setError(errorMessage(e)))
  const revoke = (s: Row) => {
    if (s.current) { signOut(); return }
    revokeSession(s.id).then(reload).catch(e => setError(errorMessage(e)))
  }
  const label = (s: Row) => s.current ? 'Log out' : 'Revoke'

  return (
    <div>
      <PageHeader title="Settings" action={<Button variant="secondary" onClick={signOut}>Log out</Button>} />
      <div className="border border-gray-200 rounded-lg shadow-sm p-5 mb-6 max-w-xl">
        <h3 className="text-sm font-semibold text-gray-900 mb-1">This session</h3>
        <p className="text-sm text-gray-600">
          Logged in with the API key <strong>{session.api_key_name}</strong> since {shortDate(session.created_at)}; the session ends {shortDate(session.expires_at)}.
          Revoking that key ends its sessions too.
        </p>
      </div>
      <h3 className="text-sm font-semibold text-gray-900 mb-2">Active sessions</h3>
      {error && <p className="text-sm text-red-600 mb-3">{error}</p>}
      <Table head={['Browser', 'API key', 'IP', 'Logged in', 'Last seen', '']} empty={sessions.length === 0 ? 'No active sessions' : undefined}
        mobile={sessions.map(s => (
          <Card key={s.id} title={browserName(s.user_agent)} subtitle={s.current ? 'This browser' : undefined}
            lines={[['Key', s.api_key_name], ['IP', s.ip], ['Logged in', shortDate(s.created_at)], ['Last seen', shortDate(s.last_seen_at)]]}
            actions={<RowAction tone="red" onClick={() => revoke(s)}>{label(s)}</RowAction>} />
        ))}>
        {sessions.map(s => (
          <tr key={s.id} className="border-b border-gray-100">
            <td className="px-4 py-3 text-sm text-gray-900">{browserName(s.user_agent)}{s.current && <span className="ml-2 text-xs text-indigo-700">this browser</span>}</td>
            <td className="px-4 py-3 text-sm text-gray-700">{s.api_key_name}</td>
            <td className="px-4 py-3 text-sm text-gray-500">{s.ip}</td>
            <td className="px-4 py-3 text-sm text-gray-500 whitespace-nowrap">{shortDate(s.created_at)}</td>
            <td className="px-4 py-3 text-sm text-gray-500 whitespace-nowrap">{shortDate(s.last_seen_at)}</td>
            <td className="px-4 py-3"><RowAction tone="red" onClick={() => revoke(s)}>{label(s)}</RowAction></td>
          </tr>
        ))}
      </Table>
    </div>
  )
}
