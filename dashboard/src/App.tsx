import { Routes, Route, Link, useLocation } from 'react-router-dom'
import { LayoutDashboard, Cpu, Layers, Rocket, FlaskConical, Activity, AlertTriangle } from 'lucide-react'
import Dashboard from './pages/Dashboard'
import Devices from './pages/Devices'
import Models from './pages/Models'
import Deployments from './pages/Deployments'
import Experiments from './pages/Experiments'
import Telemetry from './pages/Telemetry'
import Drift from './pages/Drift'

const navItems = [
  { path: '/', label: 'Dashboard', icon: LayoutDashboard },
  { path: '/devices', label: 'Devices', icon: Cpu },
  { path: '/models', label: 'Models', icon: Layers },
  { path: '/deployments', label: 'Deployments', icon: Rocket },
  { path: '/experiments', label: 'Experiments', icon: FlaskConical },
  { path: '/telemetry', label: 'Telemetry', icon: Activity },
  { path: '/drift', label: 'Drift', icon: AlertTriangle },
]

export default function App() {
  const location = useLocation()
  return (
    <div className="flex h-screen bg-white">
      <nav className="w-56 border-r border-gray-200 bg-gray-50/50 flex flex-col">
        <div className="px-5 py-5">
          <h1 className="text-lg font-bold text-gray-900">FleetML</h1>
        </div>
        <div className="flex flex-col gap-0.5 px-3">
          {navItems.map(item => {
            const Icon = item.icon
            const active = location.pathname === item.path
            return (
              <Link key={item.path} to={item.path}
                className={`px-3 py-2 rounded-md text-sm font-medium flex items-center gap-2.5
                  ${active
                    ? 'bg-indigo-50 text-indigo-700 border-l-[3px] border-indigo-600 -ml-px'
                    : 'text-gray-600 hover:bg-gray-100 hover:text-gray-900'}`}>
                <Icon size={18} strokeWidth={1.75} />
                {item.label}
              </Link>
            )
          })}
        </div>
      </nav>
      <main className="flex-1 overflow-auto bg-white">
        <div className="max-w-7xl mx-auto p-8">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/devices" element={<Devices />} />
            <Route path="/models" element={<Models />} />
            <Route path="/deployments" element={<Deployments />} />
            <Route path="/experiments" element={<Experiments />} />
            <Route path="/telemetry" element={<Telemetry />} />
            <Route path="/drift" element={<Drift />} />
          </Routes>
        </div>
      </main>
    </div>
  )
}
