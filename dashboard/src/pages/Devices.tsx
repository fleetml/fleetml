import { useEffect, useState } from 'react'
import { getDevices, getDeviceProfile } from '../api/client'
import type { Device, DeviceProfile } from '../types/api'
import CopyableId from '../components/CopyableId'

export default function Devices() {
  const [devices, setDevices] = useState<Device[]>([])
  const [selected, setSelected] = useState<DeviceProfile | null>(null)

  useEffect(() => { getDevices().then(d => setDevices(d || [])).catch(() => {}) }, [])

  return (
    <div>
      <h2 className="text-2xl font-semibold text-gray-900 mb-6">Devices</h2>
      <div className="border border-gray-200 rounded-lg shadow-sm overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="bg-gray-50 border-b border-gray-200">
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">ID</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">OS</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">RAM (MB)</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Chipset</th>
              <th className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider"></th>
            </tr>
          </thead>
          <tbody>
            {devices.length === 0 && <tr><td colSpan={5} className="px-4 py-8 text-center text-sm text-gray-400">No devices registered</td></tr>}
            {devices.map(d => (
              <tr key={d.ID} className="border-b border-gray-100 hover:bg-gray-50">
                <td className="px-4 py-3"><CopyableId id={d.ID} /></td>
                <td className="px-4 py-3 text-sm text-gray-900">{d.OS}</td>
                <td className="px-4 py-3 text-sm text-gray-900">{d.RAMMB}</td>
                <td className="px-4 py-3 text-sm text-gray-500">{d.Chipset || '—'}</td>
                <td className="px-4 py-3"><button onClick={() => getDeviceProfile(d.ID).then(setSelected)} className="text-indigo-600 hover:text-indigo-800 text-sm">Profile</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {selected && (
        <div className="mt-4 border border-gray-200 rounded-lg shadow-sm p-5">
          <div className="flex items-center gap-3 mb-3">
            <h3 className="text-sm font-semibold text-gray-900">Device Profile</h3>
            <CopyableId id={selected.DeviceID} />
          </div>
          <div className="grid grid-cols-2 gap-x-8 gap-y-2 text-sm">
            <div className="flex justify-between"><span className="text-gray-500">Class</span><span className="text-gray-900">{selected.DeviceClass}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">Score</span><span className="text-gray-900">{selected.Score}/100</span></div>
            <div className="flex justify-between"><span className="text-gray-500">OS</span><span className="text-gray-900">{selected.OS}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">RAM</span><span className="text-gray-900">{selected.Memory.TotalRAMMB} MB</span></div>
            <div className="flex justify-between"><span className="text-gray-500">NPU</span><span className="text-gray-900">{selected.Accelerators.HasNPU ? `Yes (${selected.Accelerators.NPUTflops} TFLOPS)` : 'No'}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">GPU</span><span className="text-gray-900">{selected.Accelerators.HasGPU ? 'Yes' : 'No'}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">Architecture</span><span className="text-gray-900">{selected.Compute.Architecture}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">Runtimes</span><span className="text-gray-900">{selected.Accelerators.SupportedRuntimes?.join(', ')}</span></div>
          </div>
          <button onClick={() => setSelected(null)} className="mt-3 text-sm text-gray-400 hover:text-gray-600">Dismiss</button>
        </div>
      )}
    </div>
  )
}
