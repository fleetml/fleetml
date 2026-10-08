import { lazy, Suspense, useEffect, useState } from 'react'
import { Routes, Route, Link, useLocation } from 'react-router-dom'
import { LayoutDashboard, Cpu, Layers, Rocket, FlaskConical, Activity, AlertTriangle, Bell, Settings as SettingsIcon, ShieldAlert } from 'lucide-react'
import { AUTH_EVENT, getApiKey } from './api/auth'

// Each page is its own chunk, so the first load stays small (charts load with Telemetry).
const Dashboard = lazy(() => import('./pages/Dashboard'))
const Devices = lazy(() => import('./pages/Devices'))
const Models = lazy(() => import('./pages/Models'))
const Deployments = lazy(() => import('./pages/Deployments'))
const Experiments = lazy(() => import('./pages/Experiments'))
const Telemetry = lazy(() => import('./pages/Telemetry'))
const Drift = lazy(() => import('./pages/Drift'))
const Alerts = lazy(() => import('./pages/Alerts'))
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
  { path: '/settings', label: 'Settings', icon: SettingsIcon },
]

export default function App() {
  const location = useLocation()
  const [authError, setAuthError] = useState(getApiKey() ? '' : 'No API key set. Add your admin API key in Settings.')

  useEffect(() => {
    const onUnauthorized = (e: Event) => setAuthError((e as CustomEvent<string>).detail || 'API key missing or rejected.')
    window.addEventListener(AUTH_EVENT, onUnauthorized)
    return () => window.removeEventListener(AUTH_EVENT, onUnauthorized)
  }, [])

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
          {authError && (
            <div role="alert" className="mb-6 flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 px-4 py-3">
              <ShieldAlert size={18} className="mt-0.5 text-red-600 shrink-0" strokeWidth={1.75} />
              <div className="text-sm">
                <p className="font-medium text-red-800">Not authorized</p>
                <p className="text-red-700">{authError}</p>
                {location.pathname !== '/settings' && (
                  <Link to="/settings" className="mt-1 inline-block font-medium text-red-800 underline">Open Settings</Link>
                )}
              </div>
            </div>
          )}
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
            <Route path="/settings" element={<Settings onKeySaved={() => setAuthError('')} />} />
          </Routes>
          </Suspense>
        </div>
      </main>
    </div>
  )
}
