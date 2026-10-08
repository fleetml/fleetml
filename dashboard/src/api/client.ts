import type { Project, Model, ModelVersion, ModelVariant, Device, DeviceProfile, Deployment, Experiment, ExperimentResult, TelemetrySummary, TimeSeriesPoint, DriftResult, DriftPoint, AuditEntry, AlertSink, AlertDelivery, Session, SessionInfo } from '../types/api';
import { getCsrfToken, notifyUnauthorized, UnauthorizedError } from './auth';

interface ApiResponse<T> { data: T; error?: { code: string; message: string }; pagination?: { total: number; limit: number; offset: number } }

// The dashboard is served from the API's origin (nginx in production, the Vite
// proxy in development), so the session cookie is sent with every request.
async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  if (method !== 'GET') headers['X-CSRF-Token'] = getCsrfToken()
  const res = await fetch(path, { method, headers, credentials: 'same-origin', body: body !== undefined ? JSON.stringify(body) : undefined })

  let json: ApiResponse<T> | undefined
  try { json = await res.json() } catch { json = undefined }

  if (res.status === 401) {
    // Only a tab that was logged in has a CSRF token: tell it why it is back at the login page.
    const msg = getCsrfToken() ? 'Your session ended: it expired, was revoked, or you logged out in another tab. Log in again.' : ''
    notifyUnauthorized(msg)
    throw new UnauthorizedError(msg)
  }
  if (json?.error) throw new Error(json.error.message)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return json?.data as T
}

const apiGet = <T>(path: string) => request<T>('GET', path)
const apiPost = <T>(path: string, body?: unknown) => request<T>('POST', path, body)
const apiPatch = <T>(path: string, body: unknown) => request<T>('PATCH', path, body)
const apiDelete = (path: string) => request<void>('DELETE', path)

/** Exchanges an API key for a session cookie. A rejected key throws without signalling a logout. */
export async function login(apiKey: string): Promise<SessionInfo> {
  const res = await fetch('/api/v1/auth/login', {
    method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ api_key: apiKey.trim() }),
  })
  const json = await res.json().catch(() => undefined) as ApiResponse<SessionInfo> | undefined
  if (res.status === 401) throw new Error('The API rejected this key. Check that you copied it completely and that it has not been revoked.')
  if (!res.ok || !json) throw new Error(json?.error?.message || `HTTP ${res.status}`)
  return json.data
}

export const getSession = () => apiGet<SessionInfo>('/api/v1/auth/session');
export const logout = () => apiPost<void>('/api/v1/auth/logout');
export const getSessions = () => apiGet<(Session & { current: boolean })[]>('/api/v1/auth/sessions');
export const revokeSession = (id: string) => apiPost<void>(`/api/v1/auth/sessions/${id}/revoke`);

export const getProjects = () => apiGet<Project[]>('/api/v1/projects');
export const getModels = (projectId: string) => apiGet<Model[]>(`/api/v1/projects/${projectId}/models`);
export const getVersions = (modelId: string) => apiGet<ModelVersion[]>(`/api/v1/models/${modelId}/versions`);
export const getVariants = (versionId: string) => apiGet<ModelVariant[]>(`/api/v1/versions/${versionId}/variants`);
export const getDevices = () => apiGet<Device[]>('/api/v1/devices');
export const getDeployments = () => apiGet<Deployment[]>('/api/v1/deployments');
export const getExperiments = () => apiGet<Experiment[]>('/api/v1/experiments');
export const getTelemetrySummary = (id: string) => apiGet<TelemetrySummary>(`/api/v1/telemetry/summary/${id}`);
export const getTelemetryTimeSeries = (id: string, interval: string) => apiGet<TimeSeriesPoint[]>(`/api/v1/telemetry/timeseries/${id}?interval=${interval}`);
export const getDriftStatus = (id: string) => apiGet<DriftResult[]>(`/api/v1/drift/${id}`);
export const getDriftHistory = (id: string) => apiGet<DriftResult[]>(`/api/v1/drift/${id}/history`);
export const checkDrift = (id: string) => apiPost<DriftResult[]>(`/api/v1/drift/${id}/check`);
export const getDriftPoints = (id: string, days = 7) => apiGet<DriftPoint[]>(`/api/v1/drift/${id}/points?days=${days}`);
export const getDeviceProfile = (id: string) => apiGet<DeviceProfile>(`/api/v1/devices/${id}/profile`);
export const getExperimentResults = (id: string) => apiGet<ExperimentResult>(`/api/v1/experiments/${id}/results`);
export const startDeployment = (id: string) => apiPost<Deployment>(`/api/v1/deployments/${id}/start`);
export const pauseDeployment = (id: string) => apiPost<Deployment>(`/api/v1/deployments/${id}/pause`);
export const resumeDeployment = (id: string) => apiPost<Deployment>(`/api/v1/deployments/${id}/resume`);
export const rollbackDeployment = (id: string, reason: string) => apiPost<Deployment>(`/api/v1/deployments/${id}/rollback`, { reason });
export const cancelDeployment = (id: string) => apiPost<Deployment>(`/api/v1/deployments/${id}/cancel`);
export const createDeployment = (body: NewDeployment) => apiPost<Deployment>('/api/v1/deployments', body);

export interface NewDeployment {
  project_id: string;
  model_version_id: string;
  name?: string;
  strategy: 'ALL_AT_ONCE' | 'CANARY' | 'ROLLING';
  target_selector: Record<string, string>;
  canary_stages?: { percentage: number; min_duration: string }[];
  rolling_config?: { batch_size_percent: number; batch_interval: string };
  health_gate?: { max_error_rate: number; min_events: number; on_failure: string };
  urgent?: boolean;
}

export interface NewExperiment {
  project_id: string;
  name: string;
  control_version_id: string;
  treatment_version_id: string;
  target_selector: Record<string, string>;
  traffic_percent: number;
  split_ratio: number;
  min_sample_size: number;
  max_duration?: string;
}

export const createExperiment = (body: NewExperiment) => apiPost<Experiment>('/api/v1/experiments', body);
export const startExperiment = (id: string) => apiPost<Experiment>(`/api/v1/experiments/${id}/start`);
export const pauseExperiment = (id: string) => apiPost<Experiment>(`/api/v1/experiments/${id}/pause`);
export const resumeExperiment = (id: string) => apiPost<Experiment>(`/api/v1/experiments/${id}/resume`);
export const cancelExperiment = (id: string) => apiPost<Experiment>(`/api/v1/experiments/${id}/cancel`);
export const concludeExperiment = (id: string, winnerVersionId: string, note: string) =>
  apiPost<Experiment>(`/api/v1/experiments/${id}/conclude`, { winner_version_id: winnerVersionId, note });

export interface NewAlertSink {
  name: string;
  kind: AlertSink['kind'];
  config: Record<string, string>;
}

export const getAlertSinks = (projectId: string) => apiGet<AlertSink[]>(`/api/v1/projects/${projectId}/alert-sinks`);
export const createAlertSink = (projectId: string, body: NewAlertSink) => apiPost<AlertSink>(`/api/v1/projects/${projectId}/alert-sinks`, body);
export const updateAlertSink = (id: string, body: Partial<Pick<AlertSink, 'name' | 'enabled' | 'config'>>) => apiPatch<AlertSink>(`/api/v1/alert-sinks/${id}`, body);
export const deleteAlertSink = (id: string) => apiDelete(`/api/v1/alert-sinks/${id}`);
export const testAlertSink = (id: string) => apiPost<AlertDelivery>(`/api/v1/alert-sinks/${id}/test`);
export const getAlertDeliveries = (id: string) => apiGet<AlertDelivery[]>(`/api/v1/alert-sinks/${id}/deliveries`);

export interface AuditFilter { project_id?: string; actor?: string; action?: string; outcome?: '' | 'allowed' | 'denied'; limit?: number }

const auditQuery = (f: AuditFilter) =>
  new URLSearchParams(Object.entries(f).filter(([, v]) => v !== undefined && v !== '').map(([k, v]) => [k, String(v)])).toString()
export const getAudit = (f: AuditFilter) => apiGet<AuditEntry[]>(`/api/v1/audit?${auditQuery(f)}`);
/** The CSV export of the same filters, downloaded with the session cookie. */
export const auditExportURL = (f: AuditFilter) => `/api/v1/audit/export?${auditQuery(f)}`;
