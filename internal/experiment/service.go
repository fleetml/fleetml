package experiment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

var validTransitions = map[string]map[string]bool{
	"DRAFT":   {"RUNNING": true, "CANCELLED": true},
	"RUNNING": {"PAUSED": true, "CONCLUDED": true, "CANCELLED": true},
	"PAUSED":  {"RUNNING": true, "CANCELLED": true},
}

var validMetrics = map[string]bool{"latency": true, "error_rate": true, "confidence": true}

// Service provides business logic for experiments.
// Repo defines the repository interface for experiments.
type Repo interface {
	Create(ctx context.Context, e *Experiment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Experiment, error)
	List(ctx context.Context, projectID *uuid.UUID, limit, offset int) ([]Experiment, int, error)
	Update(ctx context.Context, e *Experiment) error
	Delete(ctx context.Context, id uuid.UUID) error
	ListByStatus(ctx context.Context, statuses []string) ([]Experiment, error)
	AssignCohort(ctx context.Context, a *CohortAssignment) error
	GetCohortAssignment(ctx context.Context, deviceID, experimentID uuid.UUID) (*CohortAssignment, error)
	ListCohortAssignments(ctx context.Context, experimentID uuid.UUID) ([]CohortAssignment, error)
	GetCohortStats(ctx context.Context, experimentID, versionID uuid.UUID, since time.Time, cohort string) (*CohortStats, error)
}

// Service provides business logic for experiments.
type Service struct {
	repo   Repo
	logger *slog.Logger
}

// NewService creates a new experiment service.
func NewService(repo Repo, logger *slog.Logger) *Service {
	return &Service{repo: repo, logger: logger}
}

// CreateInput holds parameters for creating an experiment.
type CreateInput struct {
	ProjectID          uuid.UUID
	Name               string
	Description        string
	ControlVersionID   uuid.UUID
	TreatmentVersionID uuid.UUID
	TrafficPercent     int
	SplitRatio         int
	TargetSelector     map[string]string
	PrimaryMetric      string
	MinSampleSize      int
	ConfidenceLevel    float64
	AutoConclude       bool
	MaxDuration        string
	CreatedBy          string
}

// Create validates and creates an experiment.
func (s *Service) Create(ctx context.Context, input CreateInput) (*Experiment, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if input.ControlVersionID == uuid.Nil || input.TreatmentVersionID == uuid.Nil {
		return nil, fmt.Errorf("%w: control and treatment version IDs are required", ErrInvalidInput)
	}
	if input.ControlVersionID == input.TreatmentVersionID {
		return nil, fmt.Errorf("%w: control and treatment must be different versions", ErrInvalidInput)
	}
	if input.TrafficPercent <= 0 {
		input.TrafficPercent = 100
	}
	if input.SplitRatio <= 0 || input.SplitRatio >= 100 {
		input.SplitRatio = 50
	}
	metric := strings.ToLower(strings.TrimSpace(input.PrimaryMetric))
	if metric == "" {
		metric = "latency"
	}
	if !validMetrics[metric] {
		return nil, fmt.Errorf("%w: primary_metric must be latency, error_rate, or confidence", ErrInvalidInput)
	}
	if input.MinSampleSize <= 0 {
		input.MinSampleSize = 100
	}
	if input.ConfidenceLevel <= 0 || input.ConfidenceLevel >= 1 {
		input.ConfidenceLevel = 0.95
	}
	if input.MaxDuration == "" {
		input.MaxDuration = "7d"
	}
	if input.TargetSelector == nil {
		input.TargetSelector = map[string]string{}
	}
	if input.CreatedBy == "" {
		input.CreatedBy = "system"
	}

	exp := &Experiment{
		ProjectID:          input.ProjectID,
		Name:               input.Name,
		Description:        input.Description,
		Status:             "DRAFT",
		ControlVersionID:   input.ControlVersionID,
		TreatmentVersionID: input.TreatmentVersionID,
		TrafficPercent:     input.TrafficPercent,
		SplitRatio:         input.SplitRatio,
		TargetSelector:     input.TargetSelector,
		PrimaryMetric:      metric,
		MinSampleSize:      input.MinSampleSize,
		ConfidenceLevel:    input.ConfidenceLevel,
		AutoConclude:       input.AutoConclude,
		MaxDuration:        input.MaxDuration,
		CreatedBy:          input.CreatedBy,
	}

	if err := s.repo.Create(ctx, exp); err != nil {
		return nil, err
	}
	return exp, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Experiment, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, projectID *uuid.UUID, limit, offset int) ([]Experiment, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.repo.List(ctx, projectID, limit, offset)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) Start(ctx context.Context, id uuid.UUID) (*Experiment, error) {
	exp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateTransition(exp.Status, "RUNNING"); err != nil {
		return nil, err
	}
	exp.Status = "RUNNING"
	now := time.Now()
	exp.StartedAt = &now
	if err := s.repo.Update(ctx, exp); err != nil {
		return nil, err
	}
	return exp, nil
}

func (s *Service) Pause(ctx context.Context, id uuid.UUID) (*Experiment, error) {
	exp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateTransition(exp.Status, "PAUSED"); err != nil {
		return nil, err
	}
	exp.Status = "PAUSED"
	if err := s.repo.Update(ctx, exp); err != nil {
		return nil, err
	}
	return exp, nil
}

func (s *Service) Resume(ctx context.Context, id uuid.UUID) (*Experiment, error) {
	exp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateTransition(exp.Status, "RUNNING"); err != nil {
		return nil, err
	}
	exp.Status = "RUNNING"
	if err := s.repo.Update(ctx, exp); err != nil {
		return nil, err
	}
	return exp, nil
}

func (s *Service) Cancel(ctx context.Context, id uuid.UUID) (*Experiment, error) {
	exp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if exp.Status == "CONCLUDED" || exp.Status == "CANCELLED" {
		return nil, fmt.Errorf("%w: cannot cancel from %s", ErrInvalidTransition, exp.Status)
	}
	exp.Status = "CANCELLED"
	now := time.Now()
	exp.ConcludedAt = &now
	if err := s.repo.Update(ctx, exp); err != nil {
		return nil, err
	}
	return exp, nil
}

func (s *Service) Conclude(ctx context.Context, id, winnerVersionID uuid.UUID, note string) (*Experiment, error) {
	exp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateTransition(exp.Status, "CONCLUDED"); err != nil {
		return nil, err
	}
	exp.Status = "CONCLUDED"
	exp.WinnerVersionID = &winnerVersionID
	exp.ConclusionNote = note
	now := time.Now()
	exp.ConcludedAt = &now
	if err := s.repo.Update(ctx, exp); err != nil {
		return nil, err
	}
	return exp, nil
}

// EvaluateExperiment computes statistical results.
func (s *Service) EvaluateExperiment(ctx context.Context, id uuid.UUID) (*ExperimentResult, error) {
	exp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	since := exp.CreatedAt
	if exp.StartedAt != nil {
		since = *exp.StartedAt
	}

	controlStats, err := s.repo.GetCohortStats(ctx, exp.ID, exp.ControlVersionID, since, "control")
	if err != nil {
		return nil, fmt.Errorf("querying control stats: %w", err)
	}
	controlStats.Cohort = "control"
	controlStats.VersionID = exp.ControlVersionID.String()

	treatmentStats, err := s.repo.GetCohortStats(ctx, exp.ID, exp.TreatmentVersionID, since, "treatment")
	if err != nil {
		return nil, fmt.Errorf("querying treatment stats: %w", err)
	}
	treatmentStats.Cohort = "treatment"
	treatmentStats.VersionID = exp.TreatmentVersionID.String()

	result := Evaluate(*controlStats, *treatmentStats, exp.PrimaryMetric, exp.ConfidenceLevel)

	exp.Result = result
	_ = s.repo.Update(ctx, exp)

	return result, nil
}

// AssignDeviceCohort assigns a device to a cohort.
func (s *Service) AssignDeviceCohort(ctx context.Context, deviceID, experimentID uuid.UUID) (cohort string, included bool, err error) {
	existing, err := s.repo.GetCohortAssignment(ctx, deviceID, experimentID)
	if err == nil && existing != nil {
		return existing.Cohort, true, nil
	}

	exp, err := s.repo.GetByID(ctx, experimentID)
	if err != nil {
		return "", false, err
	}

	cohort, included = AssignCohort(deviceID, experimentID, exp.SplitRatio, exp.TrafficPercent)
	if !included {
		return "", false, nil
	}

	assignment := &CohortAssignment{
		DeviceID:     deviceID,
		ExperimentID: experimentID,
		Cohort:       cohort,
	}
	if err := s.repo.AssignCohort(ctx, assignment); err != nil {
		return "", false, err
	}
	return cohort, true, nil
}

// GetCohortAssignments returns all cohort assignments for an experiment.
func (s *Service) GetCohortAssignments(ctx context.Context, experimentID uuid.UUID) ([]CohortAssignment, error) {
	return s.repo.ListCohortAssignments(ctx, experimentID)
}

// ListRunning returns RUNNING experiments (for ticker).
func (s *Service) ListRunning(ctx context.Context) ([]Experiment, error) {
	return s.repo.ListByStatus(ctx, []string{"RUNNING"})
}

func validateTransition(from, to string) error {
	allowed, ok := validTransitions[from]
	if !ok || !allowed[to] {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, from, to)
	}
	return nil
}
