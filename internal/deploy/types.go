package deploy

import (
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/rollout"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("already exists")
	ErrInvalidInput      = errors.New("invalid input")
	ErrInvalidTransition = errors.New("invalid status transition")
	// ErrNotDeployable means the model version has no READY (verified) variant.
	ErrNotDeployable = errors.New("model version not deployable")
)

// Deployment represents a model rollout to a set of devices.
type Deployment struct {
	ID                uuid.UUID         `json:"id"`
	ProjectID         uuid.UUID         `json:"project_id"`
	ModelVersionID    uuid.UUID         `json:"model_version_id"`
	Name              string            `json:"name"`
	TargetSelector    map[string]string `json:"target_selector"`
	Strategy          string            `json:"strategy"`
	CanaryPercentage  *int              `json:"canary_percentage,omitempty"`
	CanaryStages      []CanaryStage     `json:"canary_stages,omitempty"`
	CurrentStage      int               `json:"current_stage"`
	RollingConfig     *RollingConfig    `json:"rolling_config,omitempty"`
	HealthGate        *HealthGate       `json:"health_gate,omitempty"`
	PreviousVersionID *uuid.UUID        `json:"previous_version_id,omitempty"`
	StageStartedAt    *time.Time        `json:"stage_started_at,omitempty"`
	// RolloutThreshold is the hash-ring position below which a rolling
	// deployment's devices are exposed (see internal/rollout).
	RolloutThreshold *int64     `json:"-"`
	RollbackReason   string     `json:"rollback_reason,omitempty"`
	Status           string     `json:"status"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
}

// CanaryStage defines a stage in a canary rollout.
type CanaryStage struct {
	Percentage  int        `json:"percentage"`
	MinDuration string     `json:"min_duration"`
	Status      string     `json:"status"` // PENDING, ACTIVE, PASSED, FAILED
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// RollingConfig configures rolling deployments. Stage N exposes N batches:
// either N x BatchSizePercent percent of the targeted devices, or
// N x BatchSize devices. Set exactly one of the two.
type RollingConfig struct {
	BatchSize        int    `json:"batch_size,omitempty"`
	BatchSizePercent int    `json:"batch_size_percent,omitempty"`
	BatchInterval    string `json:"batch_interval"`
	// MaxUnavailable holds the rollout while more than this many exposed,
	// recently seen devices have not yet reported the new version loaded.
	// 0 disables the check.
	MaxUnavailable int `json:"max_unavailable"`
}

// DefaultRollingConfig is used when a rolling deployment has no config.
func DefaultRollingConfig() *RollingConfig {
	return &RollingConfig{BatchSizePercent: 10, BatchInterval: "5m"}
}

// HealthGate defines conditions that must be met to continue a rollout.
// It is evaluated on telemetry from the devices in the current cohort only,
// over the current stage's window.
type HealthGate struct {
	// MaxErrorRate is the highest allowed share of error events (ERROR, OOM,
	// MODEL_LOAD_FAILED, or any event with an error message), 0-1.
	MaxErrorRate float64 `json:"max_error_rate"`
	// MaxLatencyRegression is the highest allowed increase of p95 inference
	// latency over the previous version's p95, as a fraction (0.2 = +20%).
	// 0 disables the check.
	MaxLatencyRegression float64 `json:"max_latency_regression"`
	// MinEvents is the number of cohort events needed before the gate judges.
	MinEvents int    `json:"min_events"`
	OnFailure string `json:"on_failure"` // PAUSE or ROLLBACK
}

// DefaultHealthGate applies to every canary and rolling deployment created
// without one: roll back if more than 5% of at least 20 cohort events are errors.
func DefaultHealthGate() *HealthGate {
	return &HealthGate{MaxErrorRate: 0.05, MinEvents: 20, OnFailure: "ROLLBACK"}
}

// Exposes reports whether the deployment currently delivers to a device.
// It must agree with the orchestrator's exposure rule.
func (d *Deployment) Exposes(deviceID uuid.UUID) bool {
	if d.Status == "COMPLETE" || d.Strategy == "ALL_AT_ONCE" {
		return true
	}
	if d.Strategy == "CANARY" {
		pct := d.EffectiveCanaryPercent()
		return pct <= 0 || pct >= 100 || rollout.InCanary(deviceID, d.ID, pct)
	}
	if d.Strategy == "ROLLING" {
		return d.RolloutThreshold != nil && rollout.Exposed(deviceID, d.ID, *d.RolloutThreshold)
	}
	return true
}

// stageStartedAt is when the current stage began: the start of the gate's window.
func (d *Deployment) stageStartedAt() time.Time {
	if d.Strategy == "CANARY" && d.CurrentStage < len(d.CanaryStages) && d.CanaryStages[d.CurrentStage].StartedAt != nil {
		return *d.CanaryStages[d.CurrentStage].StartedAt
	}
	if d.StageStartedAt != nil {
		return *d.StageStartedAt
	}
	return d.UpdatedAt
}

// EffectiveCanaryPercent returns the canary percentage for the current stage.
func (d *Deployment) EffectiveCanaryPercent() int {
	switch d.Strategy {
	case "ALL_AT_ONCE":
		return 100
	case "CANARY":
		if d.Status == "COMPLETE" {
			return 100
		}
		if d.CurrentStage < len(d.CanaryStages) {
			return d.CanaryStages[d.CurrentStage].Percentage
		}
		if d.CanaryPercentage != nil {
			return *d.CanaryPercentage
		}
		return 100
	case "ROLLING":
		if d.Status == "COMPLETE" || d.RolloutThreshold == nil {
			if d.Status == "COMPLETE" {
				return 100
			}
			return 0
		}
		return int((*d.RolloutThreshold*100 + rollout.RingSize/2) / rollout.RingSize)
	default:
		return 100
	}
}

// DefaultCanaryStages returns the default canary progression.
func DefaultCanaryStages() []CanaryStage {
	return []CanaryStage{
		{Percentage: 1, MinDuration: "5m", Status: "PENDING"},
		{Percentage: 10, MinDuration: "15m", Status: "PENDING"},
		{Percentage: 50, MinDuration: "30m", Status: "PENDING"},
		{Percentage: 100, MinDuration: "0s", Status: "PENDING"},
	}
}
