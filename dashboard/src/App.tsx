import { lazy, Suspense, useEffect, useState } from 'react'
import { Routes, Route, Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { LayoutDashboard, Cpu, Layers, Rocket, FlaskConical, Activity, AlertTriangle, Bell, ScrollText, Settings as SettingsIcon } from 'lucide-react'
import { AUTH_EVENT, setCsrfToken } from './api/auth'
import { getSession } from './api/client'
import type { SessionInfo } from './types/api'
import Login from './pages/Login'

// Each page is its own chunk, so the first load stays small (charts load with Telemetry).
const Dashboard = lazy(() => import('./pages/Dashboard'))
const Devices = lazy(() => import('./pages/Devices'))
const Models = lazy(() => import('./pages/Models'))
const Deployments = lazy(() => import('./pages/Deployments'))
const Experiments = lazy(() => import('./pages/Experiments'))
const Telemetry = lazy(() => import('./pages/Telemetry'))
const Drift = lazy(() => import('./pages/Drift'))
const Alerts = lazy(() => import('./pages/Alerts'))
const Audit = lazy(() => import('./pages/Audit'))
const Settings = lazy(() => import('./pages/Settings'))

const navItems = [
  { path: '/', label: 'Dashboard', icon: LayoutDashboard },
  { path: '/devices', label: 'Devices', icon: Cpu },
  { path: '/models', label: 'Models', icon: Layers },
  { path: '/deployments', label: 'Deployments', icon: Rocket },
  { path: '/experiments', label: 'Experiments', icon: FlaskConical },
  { path: '/telemetry', label: 'Telemetry', icon: Activity },
  { path: '/drift', label: 'Drift', icon: AlertTriangle },
  { path: '/alerts', label: 'Alerts', icon: Bell },
  { path: '/audit', label: 'Audit', icon: ScrollText },
  { path: '/settings', label: 'Settings', icon: SettingsIcon },
]

// How often an open dashboard checks that its session is still valid, so a
// session revoked elsewhere sends this tab to the login page.
const SESSION_CHECK_MS = 15_000

export default function App() {
  const location = useLocation()
  const navigate = useNavigate()
  const [session, setSession] = useState<SessionInfo | null | 'loading'>('loading')
  const [notice, setNotice] = useState('')

  const signedIn = (s: SessionInfo) => { setCsrfToken(s.csrf_token); setSession(s); setNotice('') }

  useEffect(() => {
    getSession().then(signedIn).catch(() => setSession(null))
    const onUnauthorized = (e: Event) => {
      setCsrfToken('')
      setSession(null)
      setNotice((e as CustomEvent<string>).detail || '')
    }
    window.addEventListener(AUTH_EVENT, onUnauthorized)
    return () => window.removeEventListener(AUTH_EVENT, onUnauthorized)
  }, [])

  useEffect(() => {
    if (!session || session === 'loading') return
    const check = () => { getSession().catch(() => undefined) } // a 401 fires AUTH_EVENT
    const onVisible = () => { if (document.visibilityState === 'visible') check() }
    const timer = window.setInterval(check, SESSION_CHECK_MS)
    window.addEventListener('focus', check)
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      window.clearInterval(timer)
      window.removeEventListener('focus', check)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [session])

  if (session === 'loading') return <p className="p-8 text-sm text-gray-400">Loading…</p>
  if (!session) {
    if (location.pathname !== '/login') return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />
    return <Login notice={notice} onLoggedIn={s => {
      signedIn(s)
      navigate((location.state as { from?: string } | null)?.from || '/', { replace: true })
    }} />
  }
  if (location.pathname === '/login') return <Navigate to="/" replace />

  return (
    <div className="flex flex-col md:flex-row h-screen bg-white">
      <nav className="md:w-56 shrink-0 border-b md:border-b-0 md:border-r border-gray-200 bg-gray-50/50 flex md:flex-col items-center md:items-stretch">
        <div className="pl-3 pr-1 md:px-5 py-3 md:py-5">
          <h1 className="text-base md:text-lg font-bold text-gray-900">FleetML</h1>
        </div>
        <div className="flex md:flex-col gap-0.5 pr-2 md:px-3 overflow-x-auto">
          {navItems.map(item => {
            const Icon = item.icon
            const active = location.pathname === item.path
            return (
              <Link key={item.path} to={item.path} aria-label={item.label} title={item.label}
                className={`px-2 md:px-3 py-2 rounded-md text-sm font-medium flex items-center gap-2.5 whitespace-nowrap
                  ${active
                    ? 'bg-indigo-50 text-indigo-700 md:border-l-[3px] md:border-indigo-600 md:-ml-px'
                    : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900'}`}>
                <Icon size={18} strokeWidth={1.75} />
                <span className="hidden md:inline">{item.label}</span>
              </Link>
            )
          })}
        </div>
      </nav>
      <main className="flex-1 overflow-auto bg-white">
        <div className="max-w-7xl mx-auto p-4 md:p-8">
          <Suspense fallback={<p className="text-sm text-gray-400">Loading…</p>}>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/devices" element={<Devices />} />
            <Route path="/models" element={<Models />} />
            <Route path="/deployments" element={<Deployments />} />
            <Route path="/experiments" element={<Experiments />} />
            <Route path="/telemetry" element={<Telemetry />} />
            <Route path="/drift" element={<Drift />} />
            <Route path="/alerts" element={<Alerts />} />
            <Route path="/audit" element={<Audit />} />
            <Route path="/settings" element={<Settings session={session.session} onLoggedOut={() => { setCsrfToken(''); setSession(null); setNotice('You are logged out.') }} />} />
          </Routes>
          </Suspense>
        </div>
      </main>
    </div>
  )
}
