import type { Project, Device, DeviceProfile, Deployment, Experiment, ExperimentResult, TelemetrySummary, TimeSeriesPoint, DriftResult } from '../types/api';
import { getApiKey, notifyUnauthorized, UnauthorizedError } from './auth';

const BASE_URL = import.meta.env.VITE_API_URL || '';

interface ApiResponse<T> { data: T; error?: { code: string; message: string }; pagination?: { total: number; limit: number; offset: number } }

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const key = getApiKey()
  if (!key) {
    const msg = 'No API key set. Add your admin API key in Settings.'
    notifyUnauthorized(msg)
    throw new UnauthorizedError(msg)
  }
  const headers: Record<string, string> = { Authorization: `Bearer ${key}` }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(`${BASE_URL}${path}`, { method, headers, body: body !== undefined ? JSON.stringify(body) : undefined })

  let json: ApiResponse<T> | undefined
  try { json = await res.json() } catch { json = undefined }

  if (res.status === 401) {
    const msg = `The API rejected your key (401${json?.error?.message ? `: ${json.error.message}` : ''}). Update it in Settings.`
    notifyUnauthorized(msg)
    throw new UnauthorizedError(msg)
  }
  if (json?.error) throw new Error(json.error.message)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return json?.data as T
}

const apiGet = <T>(path: string) => request<T>('GET', path)
const apiPost = <T>(path: string, body?: unknown) => request<T>('POST', path, body)

/** Checks a key without storing it. Resolves true if the API accepts it. */
export async function verifyApiKey(key: string): Promise<boolean> {
  const res = await fetch(`${BASE_URL}/api/v1/api-keys`, { headers: { Authorization: `Bearer ${key.trim()}` } })
  if (res.status === 401) return false
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return true
}

export const getProjects = () => apiGet<Project[]>('/api/v1/projects');
export const getDevices = () => apiGet<Device[]>('/api/v1/devices');
export const getDeployments = () => apiGet<Deployment[]>('/api/v1/deployments');
export const getExperiments = () => apiGet<Experiment[]>('/api/v1/experiments');
export const getTelemetrySummary = (id: string) => apiGet<TelemetrySummary>(`/api/v1/telemetry/summary/${id}`);
export const getTelemetryTimeSeries = (id: string, interval: string) => apiGet<TimeSeriesPoint[]>(`/api/v1/telemetry/timeseries/${id}?interval=${interval}`);
export const getDriftStatus = (id: string) => apiGet<DriftResult[]>(`/api/v1/drift/${id}`);
export const getDeviceProfile = (id: string) => apiGet<DeviceProfile>(`/api/v1/devices/${id}/profile`);
export const getExperimentResults = (id: string) => apiGet<ExperimentResult>(`/api/v1/experiments/${id}/results`);
export const startDeployment = (id: string) => apiPost<Deployment>(`/api/v1/deployments/${id}/start`);
export const pauseDeployment = (id: string) => apiPost<Deployment>(`/api/v1/deployments/${id}/pause`);
export const rollbackDeployment = (id: string) => apiPost<Deployment>(`/api/v1/deployments/${id}/rollback`, { reason: 'dashboard rollback' });
