export interface Project {
  id: string;
  name: string;
  slug: string;
  description: string;
  created_at: string;
  updated_at: string;
}

export interface Model {
  id: string;
  project_id: string;
  name: string;
  description: string;
  framework: string;
  task_type: string;
  created_at: string;
}

export interface ModelVersion {
  id: string;
  model_id: string;
  version: string;
  status: string;
  metadata: Record<string, unknown>;
  release_notes: string;
  created_at: string;
}

export interface ModelVariant {
  id: string;
  version_id: string;
  quantization: string;
  target_runtime: string;
  blob_size_bytes: number;
  blob_sha256: string;
  status: string;
  created_at: string;
}

export interface Device {
  id: string;
  project_id: string | null;
  device_class_id: string | null;
  os: string;
  os_version: string;
  ram_mb: number;
  chipset: string;
  has_npu: boolean;
  has_gpu: boolean;
  npu_tflops: number;
  agent_version: string;
  labels: Record<string, string>;
  last_seen_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface DeviceProfile {
  device_id: string;
  device_class: string;
  os: string;
  score: number;
  compute: { cpu_cores: number; architecture: string; int_score: number; float_score: number };
  memory: { total_ram_mb: number; available_ram_mb: number; storage_free_mb: number };
  accelerators: { has_npu: boolean; has_gpu: boolean; npu_tflops: number; gpu_tflops: number; supported_runtimes: string[] | null };
  connectivity: { network_type: string; bandwidth_mbps: number; is_metered: boolean };
}

export interface Deployment {
  id: string;
  project_id: string;
  model_version_id: string;
  name: string;
  strategy: string;
  status: string;
  current_stage: number;
  target_selector?: Record<string, string> | null;
  canary_stages?: CanaryStage[];
  rolling_config?: { batch_size?: number; batch_size_percent?: number; batch_interval: string; max_unavailable: number };
  previous_version_id?: string;
  health_gate?: { max_error_rate: number; min_events: number; on_failure: string };
  rollback_reason?: string;
  urgent?: boolean;
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
  project_id: string;
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
  target_selector?: Record<string, string> | null;
  winner_version_id?: string | null;
  result?: ExperimentResult;
  created_at: string;
}

export interface ExperimentResult {
  control: CohortStats;
  treatment: CohortStats;
  p_value: number | null;
  is_significant: boolean;
  effect_size: number | null;
  recommendation: string;
  min_sample_size: number;
  final: boolean;
  tested_at?: string;
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

export interface AlertSink {
  id: string;
  project_id: string;
  name: string;
  kind: 'webhook' | 'slack' | 'email';
  config: Record<string, string>;
  enabled: boolean;
  created_at: string;
}

export interface AlertDelivery {
  id: string;
  sink_id: string;
  alert_id: string;
  alert_type: string;
  attempt: number;
  status: 'ok' | 'error' | 'dead';
  http_status?: number;
  error?: string;
  created_at: string;
}
