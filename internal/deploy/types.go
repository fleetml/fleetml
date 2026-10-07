package deploy

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
	RollbackReason    string            `json:"rollback_reason,omitempty"`
	Status            string            `json:"status"`
	CreatedBy         string            `json:"created_by"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	CompletedAt       *time.Time        `json:"completed_at,omitempty"`
}

// CanaryStage defines a stage in a canary rollout.
type CanaryStage struct {
	Percentage  int        `json:"percentage"`
	MinDuration string     `json:"min_duration"`
	Status      string     `json:"status"` // PENDING, ACTIVE, PASSED, FAILED
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// RollingConfig configures rolling deployments.
type RollingConfig struct {
	BatchSize      int    `json:"batch_size"`
	BatchInterval  string `json:"batch_interval"`
	MaxUnavailable int    `json:"max_unavailable"`
}

// HealthGate defines conditions that must be met to continue a rollout.
type HealthGate struct {
	MaxErrorRate         float64 `json:"max_error_rate"`
	MaxLatencyRegression float64 `json:"max_latency_regression"`
	MinEvents            int     `json:"min_events"`
	OnFailure            string  `json:"on_failure"` // PAUSE or ROLLBACK
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
		if d.Status == "COMPLETE" {
			return 100
		}
		return 100 // Rolling uses batch logic, not percentage
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
