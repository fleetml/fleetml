export interface Project {
  ID: string;
  Name: string;
  Slug: string;
  Description: string;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface Device {
  ID: string;
  OS: string;
  OSVersion: string;
  RAMMB: number;
  Chipset: string;
  HasNPU: boolean;
  HasGPU: boolean;
  NPUTflops: number;
  AgentVersion: string;
  Labels: Record<string, string>;
  LastSeenAt: string | null;
  CreatedAt: string;
}

export interface DeviceProfile {
  DeviceID: string;
  DeviceClass: string;
  OS: string;
  Score: number;
  Compute: { CPUCores: number; Architecture: string; IntScore: number; FloatScore: number };
  Memory: { TotalRAMMB: number; AvailableRAMMB: number; StorageFreeMB: number };
  Accelerators: { HasNPU: boolean; HasGPU: boolean; NPUTflops: number; SupportedRuntimes: string[] };
  Connectivity: { NetworkType: string; BandwidthMbps: number; IsMetered: boolean };
}

export interface Deployment {
  id: string;
  project_id: string;
  model_version_id: string;
  name: string;
  strategy: string;
  status: string;
  current_stage: number;
  canary_stages?: CanaryStage[];
  health_gate?: { max_error_rate: number; min_events: number; on_failure: string };
  rollback_reason?: string;
  created_at: string;
}

export interface CanaryStage {
  percentage: number;
  min_duration: string;
  status: string;
  started_at?: string;
  completed_at?: string;
}

export interface Experiment {
  id: string;
  name: string;
  status: string;
  control_version_id: string;
  treatment_version_id: string;
  primary_metric: string;
  split_ratio: number;
  traffic_percent: number;
  min_sample_size: number;
  confidence_level: number;
  auto_conclude: boolean;
  result?: ExperimentResult;
  created_at: string;
}

export interface ExperimentResult {
  control: CohortStats;
  treatment: CohortStats;
  p_value: number;
  is_significant: boolean;
  effect_size: number;
  recommendation: string;
}

export interface CohortStats {
  cohort: string;
  version_id: string;
  sample_size: number;
  avg_latency_ms: number;
  stddev_latency_ms: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  error_count: number;
  error_rate: number;
  avg_confidence: number;
}

export interface TelemetrySummary {
  model_version_id: string;
  total_events: number;
  error_count: number;
  avg_latency_ms: number;
  p50_latency_ms: number;
  p95_latency_ms: number;
  p99_latency_ms: number;
  min_latency_ms: number;
  max_latency_ms: number;
  avg_confidence: number;
  avg_memory_mb: number;
}

export interface TimeSeriesPoint {
  time: string;
  avg_latency_ms: number;
  p95_latency_ms: number;
  count: number;
  error_count: number;
}

export interface DriftResult {
  id: string;
  model_version_id: string;
  metric: string;
  psi_value: number;
  severity: string;
  baseline_from: string;
  baseline_to: string;
  current_from: string;
  current_to: string;
  baseline_sample: number;
  current_sample: number;
  message: string;
  checked_at: string;
}
