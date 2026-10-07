import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { getProjects, getDevices, getDeployments, getExperiments } from '../api/client'
import { Layers, Cpu, Rocket, FlaskConical } from 'lucide-react'

export default function Dashboard() {
  const [stats, setStats] = useState({ projects: 0, devices: 0, deployments: 0, experiments: 0 })
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.all([getProjects(), getDevices(), getDeployments(), getExperiments()])
      .then(([p, d, dep, exp]) => setStats({ projects: p?.length || 0, devices: d?.length || 0, deployments: dep?.length || 0, experiments: exp?.length || 0 }))
      .catch(e => setError(e.message))
  }, [])

  const cards = [
    { label: 'Projects', value: stats.projects, icon: Layers, to: '/models' },
    { label: 'Devices', value: stats.devices, icon: Cpu, to: '/devices' },
    { label: 'Deployments', value: stats.deployments, icon: Rocket, to: '/deployments' },
    { label: 'Experiments', value: stats.experiments, icon: FlaskConical, to: '/experiments' },
  ]

  return (
    <div>
      <h2 className="text-2xl font-semibold text-gray-900 mb-6">Overview</h2>
      {error && <p className="text-sm text-red-600 mb-4">{error}</p>}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {cards.map(c => {
          const Icon = c.icon
          return (
            <Link key={c.label} to={c.to} className="border border-gray-200 rounded-lg shadow-sm p-5 cursor-pointer hover:border-indigo-300">
              <div className="flex items-center gap-2 mb-1">
                <Icon size={16} className="text-gray-400" strokeWidth={1.75} />
                <span className="text-sm font-medium text-gray-500">{c.label}</span>
              </div>
              <div className="text-3xl font-semibold text-gray-900">{c.value}</div>
            </Link>
          )
        })}
      </div>
    </div>
  )
}
