import type { Project, Device, DeviceProfile, Deployment, Experiment, ExperimentResult, TelemetrySummary, TimeSeriesPoint, DriftResult } from '../types/api';

const BASE_URL = import.meta.env.VITE_API_URL || '';

interface ApiResponse<T> { data: T; error?: { code: string; message: string }; pagination?: { total: number; limit: number; offset: number } }

async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`);
  const json: ApiResponse<T> = await res.json();
  if (json.error) throw new Error(json.error.message);
  return json.data;
}

async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined });
  const json: ApiResponse<T> = await res.json();
  if (json.error) throw new Error(json.error.message);
  return json.data;
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
