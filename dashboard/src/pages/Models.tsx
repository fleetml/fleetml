import { useEffect, useState } from 'react'
import { getProjects } from '../api/client'
import type { Project } from '../types/api'
import CopyableId from '../components/CopyableId'

export default function Models() {
  const [projects, setProjects] = useState<Project[]>([])
  useEffect(() => { getProjects().then(p => setProjects(p || [])).catch(() => {}) }, [])

  return (
    <div>
      <h2 className="text-2xl font-semibold text-gray-900 mb-6">Models</h2>
      <div className="border border-gray-200 rounded-lg shadow-sm overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Project</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Slug</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">ID</th>
            </tr>
          </thead>
          <tbody>
            {projects.length === 0 && <tr><td colSpan={3} className="px-4 py-8 text-center text-sm text-gray-400">No projects</td></tr>}
            {projects.map(p => (
              <tr key={p.id} className="border-b border-gray-100 hover:bg-gray-50">
                <td className="px-4 py-3 text-sm font-medium text-gray-900">{p.name}</td>
                <td className="px-4 py-3 text-sm text-gray-500">{p.slug}</td>
                <td className="px-4 py-3"><CopyableId id={p.id} /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
