package experiment

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("already exists")
	ErrInvalidInput      = errors.New("invalid input")
	ErrInvalidTransition = errors.New("invalid status transition")
	// ErrNotDeployable means an arm's model version has no READY (verified) variant.
	ErrNotDeployable = errors.New("model version not deployable")
)

// Experiment represents an A/B test between two model versions.
type Experiment struct {
	ID                 uuid.UUID         `json:"id"`
	ProjectID          uuid.UUID         `json:"project_id"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Status             string            `json:"status"`
	ControlVersionID   uuid.UUID         `json:"control_version_id"`
	TreatmentVersionID uuid.UUID         `json:"treatment_version_id"`
	TrafficPercent     int               `json:"traffic_percent"`
	SplitRatio         int               `json:"split_ratio"`
	TargetSelector     map[string]string `json:"target_selector"`
	PrimaryMetric      string            `json:"primary_metric"`
	MinSampleSize      int               `json:"min_sample_size"`
	ConfidenceLevel    float64           `json:"confidence_level"`
	AutoConclude       bool              `json:"auto_conclude"`
	MaxDuration        string            `json:"max_duration"`
	Result             *ExperimentResult `json:"result,omitempty"`
	WinnerVersionID    *uuid.UUID        `json:"winner_version_id,omitempty"`
	ConclusionNote     string            `json:"conclusion_note,omitempty"`
	CreatedBy          string            `json:"created_by"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	StartedAt          *time.Time        `json:"started_at,omitempty"`
	ConcludedAt        *time.Time        `json:"concluded_at,omitempty"`
}

// ExperimentResult holds the statistical comparison.
type ExperimentResult struct {
	ControlStats   CohortStats `json:"control"`
	TreatmentStats CohortStats `json:"treatment"`
	PValue         float64     `json:"p_value"`
	IsSignificant  bool        `json:"is_significant"`
	EffectSize     float64     `json:"effect_size"`
	Recommendation string      `json:"recommendation"`
}

// CohortStats holds aggregated metrics for one cohort.
type CohortStats struct {
	Cohort        string  `json:"cohort"`
	VersionID     string  `json:"version_id"`
	SampleSize    int64   `json:"sample_size"`
	AvgLatency    float64 `json:"avg_latency_ms"`
	StdDevLatency float64 `json:"stddev_latency_ms"`
	P50Latency    float64 `json:"p50_latency_ms"`
	P95Latency    float64 `json:"p95_latency_ms"`
	ErrorCount    int64   `json:"error_count"`
	ErrorRate     float64 `json:"error_rate"`
	AvgConfidence float64 `json:"avg_confidence"`
}

// CohortAssignment represents which cohort a device belongs to.
type CohortAssignment struct {
	DeviceID     uuid.UUID `json:"device_id"`
	ExperimentID uuid.UUID `json:"experiment_id"`
	Cohort       string    `json:"cohort"`
	AssignedAt   time.Time `json:"assigned_at"`
}
