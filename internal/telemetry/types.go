package telemetry

import (
	"time"

	"github.com/google/uuid"
)

// Event represents a single telemetry event from a device.
// Maps to the telemetry_events table.
type Event struct {
	// EventID makes storing idempotent: an event is stored once per ID.
	EventID        uuid.UUID `json:"event_id"`
	DeviceID       uuid.UUID `json:"device_id"`
	ModelVersionID uuid.UUID `json:"model_version_id"`
	// ExperimentArmID attributes the event to an experiment arm (nil if none).
	ExperimentArmID *uuid.UUID `json:"experiment_arm_id,omitempty"`
	EventType       string     `json:"event_type"` // INFERENCE, MODEL_LOAD, MODEL_LOAD_FAILED, OOM, PREDICTION_DRIFT, ERROR
	LatencyMs       int        `json:"latency_ms"`
	Confidence      float64    `json:"confidence_score"`
	PredictionLabel string     `json:"prediction_label"`
	MemoryUsageMB   int        `json:"memory_usage_mb"`
	BatteryPercent  float64    `json:"battery_percent"`
	ErrorMessage    string     `json:"error_message"`
	RecordedAt      time.Time  `json:"recorded_at"`
	// Features are input histograms (counts per bin) by feature name.
	Features map[string][]int32 `json:"features,omitempty"`
}

// Summary provides aggregated telemetry stats over a time range.
type Summary struct {
	ModelVersionID uuid.UUID `json:"model_version_id"`
	DeviceClass    string    `json:"device_class,omitempty"`
	EventType      string    `json:"event_type,omitempty"`
	TotalEvents    int64     `json:"total_events"`
	ErrorCount     int64     `json:"error_count"`
	AvgLatency     float64   `json:"avg_latency_ms"`
	P50Latency     float64   `json:"p50_latency_ms"`
	P95Latency     float64   `json:"p95_latency_ms"`
	P99Latency     float64   `json:"p99_latency_ms"`
	MinLatency     float64   `json:"min_latency_ms"`
	MaxLatency     float64   `json:"max_latency_ms"`
	AvgConfidence  float64   `json:"avg_confidence"`
	AvgMemoryMB    float64   `json:"avg_memory_mb"`
	From           time.Time `json:"from"`
	To             time.Time `json:"to"`
}

// TimeSeriesPoint represents a single data point in a time series.
type TimeSeriesPoint struct {
	Time       time.Time `json:"time"`
	AvgLatency float64   `json:"avg_latency_ms"`
	P95Latency float64   `json:"p95_latency_ms"`
	Count      int64     `json:"count"`
	ErrorCount int64     `json:"error_count"`
}

// EventFilter provides filtering for event queries.
type EventFilter struct {
	DeviceID       *uuid.UUID
	ModelVersionID *uuid.UUID
	EventType      string
	ErrorOnly      bool
	From           time.Time
	To             time.Time
	Limit          int
	Offset         int
}
