package experiment

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
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

// VersionChecker reports how many READY variants a model version has.
type VersionChecker interface {
	ReadyVariantCount(ctx context.Context, versionID uuid.UUID) (int, error)
}

// Service provides business logic for experiments.
type Service struct {
	repo     Repo
	versions VersionChecker
	logger   *slog.Logger
}

// SetVersionChecker makes Create reject arms whose version has no READY variant.
func (s *Service) SetVersionChecker(v VersionChecker) {
	s.versions = v
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
	// That the IDs were sent is the REST layer's check: the all-zeros UUID is an
	// unknown version like any other, found below.
	if input.ControlVersionID == input.TreatmentVersionID {
		return nil, fmt.Errorf("%w: control and treatment must be different versions", ErrInvalidInput)
	}
	if s.versions != nil {
		for _, v := range []uuid.UUID{input.ControlVersionID, input.TreatmentVersionID} {
			n, err := s.versions.ReadyVariantCount(ctx, v)
			if err != nil {
				return nil, fmt.Errorf("checking variants: %w", err)
			}
			if n == 0 {
				return nil, fmt.Errorf("%w: model version %s has no READY variant", ErrNotDeployable, v)
			}
		}
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
	if _, err := ParseMaxDuration(input.MaxDuration); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
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

// EvaluateExperiment computes the experiment's result under the fixed-horizon
// rule documented on ExperimentResult: descriptive stats until both arms reach
// min_sample_size, then one significance test whose result is frozen.
func (s *Service) EvaluateExperiment(ctx context.Context, id uuid.UUID) (*ExperimentResult, error) {
	exp, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if exp.Result != nil && exp.Result.Final {
		return exp.Result, nil
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

	horizon := int64(exp.MinSampleSize)
	var result *ExperimentResult
	if controlStats.SampleSize >= horizon && treatmentStats.SampleSize >= horizon {
		result = Evaluate(*controlStats, *treatmentStats, exp.PrimaryMetric, exp.ConfidenceLevel)
		now := time.Now()
		result.Final = true
		result.TestedAt = &now
	} else {
		result = &ExperimentResult{
			ControlStats:   *controlStats,
			TreatmentStats: *treatmentStats,
			Recommendation: "COLLECTING_DATA",
		}
	}
	result.MinSampleSize = exp.MinSampleSize

	exp.Result = result
	if err := s.repo.Update(ctx, exp); err != nil {
		return nil, fmt.Errorf("storing result: %w", err)
	}
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

// ParseMaxDuration parses an experiment's max_duration: a Go duration
// ("36h", "90m") or a number of days ("7d").
func ParseMaxDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || days <= 0 {
			return 0, fmt.Errorf("max_duration %q: expected a positive number of days like \"7d\"", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("max_duration %q: expected a duration like \"7d\" or \"36h\"", s)
	}
	return d, nil
}

// Expired reports whether a running experiment has passed its max_duration.
func (e *Experiment) Expired(now time.Time) bool {
	if e.StartedAt == nil {
		return false
	}
	d, err := ParseMaxDuration(e.MaxDuration)
	if err != nil {
		return false
	}
	return now.Sub(*e.StartedAt) >= d
}

func validateTransition(from, to string) error {
	allowed, ok := validTransitions[from]
	if !ok || !allowed[to] {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, from, to)
	}
	return nil
}
