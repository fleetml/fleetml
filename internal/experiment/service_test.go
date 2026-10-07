package experiment

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Repository ---

type mockRepo struct {
	experiments map[uuid.UUID]*Experiment
	names       map[string]uuid.UUID
	cohorts     map[string]*CohortAssignment // key: "deviceID:expID"
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		experiments: make(map[uuid.UUID]*Experiment),
		names:       make(map[string]uuid.UUID),
		cohorts:     make(map[string]*CohortAssignment),
	}
}

func (m *mockRepo) Create(_ context.Context, e *Experiment) error {
	key := e.ProjectID.String() + ":" + e.Name
	if _, exists := m.names[key]; exists {
		return fmt.Errorf("experiment.Create: %w: name %q in project", ErrConflict, e.Name)
	}
	e.ID = uuid.New()
	e.CreatedAt = time.Now()
	e.UpdatedAt = time.Now()
	m.experiments[e.ID] = e
	m.names[key] = e.ID
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id uuid.UUID) (*Experiment, error) {
	e, ok := m.experiments[id]
	if !ok {
		return nil, fmt.Errorf("experiment.GetByID: %w", ErrNotFound)
	}
	return e, nil
}

func (m *mockRepo) List(_ context.Context, _ *uuid.UUID, limit, offset int) ([]Experiment, int, error) {
	var all []Experiment
	for _, e := range m.experiments {
		all = append(all, *e)
	}
	total := len(all)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, nil
}

func (m *mockRepo) Update(_ context.Context, e *Experiment) error {
	if _, ok := m.experiments[e.ID]; !ok {
		return ErrNotFound
	}
	m.experiments[e.ID] = e
	return nil
}

func (m *mockRepo) Delete(_ context.Context, id uuid.UUID) error {
	e, ok := m.experiments[id]
	if !ok {
		return fmt.Errorf("experiment.Delete: %w", ErrNotFound)
	}
	delete(m.names, e.ProjectID.String()+":"+e.Name)
	delete(m.experiments, id)
	return nil
}

func (m *mockRepo) ListByStatus(_ context.Context, statuses []string) ([]Experiment, error) {
	set := make(map[string]bool)
	for _, s := range statuses {
		set[s] = true
	}
	var result []Experiment
	for _, e := range m.experiments {
		if set[e.Status] {
			result = append(result, *e)
		}
	}
	return result, nil
}

func (m *mockRepo) AssignCohort(_ context.Context, a *CohortAssignment) error {
	key := a.DeviceID.String() + ":" + a.ExperimentID.String()
	m.cohorts[key] = a
	return nil
}

func (m *mockRepo) GetCohortAssignment(_ context.Context, deviceID, experimentID uuid.UUID) (*CohortAssignment, error) {
	key := deviceID.String() + ":" + experimentID.String()
	a, ok := m.cohorts[key]
	if !ok {
		return nil, nil
	}
	return a, nil
}

func (m *mockRepo) ListCohortAssignments(_ context.Context, _ uuid.UUID) ([]CohortAssignment, error) {
	return nil, nil
}

func (m *mockRepo) GetCohortStats(_ context.Context, _, _ uuid.UUID, _ time.Time, _ string) (*CohortStats, error) {
	return &CohortStats{SampleSize: 0}, nil
}

// --- Tests ---

func newTestService() *Service {
	return &Service{repo: newMockRepo(), logger: nil}
}

func validInput() CreateInput {
	return CreateInput{
		ProjectID:          uuid.New(),
		Name:               "test-exp",
		ControlVersionID:   uuid.New(),
		TreatmentVersionID: uuid.New(),
		PrimaryMetric:      "latency",
	}
}

func TestCreate_Valid(t *testing.T) {
	svc := newTestService()
	exp, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err)
	assert.Equal(t, "DRAFT", exp.Status)
	assert.Equal(t, 50, exp.SplitRatio)
	assert.Equal(t, 100, exp.TrafficPercent)
}

func TestCreate_MissingName(t *testing.T) {
	svc := newTestService()
	input := validInput()
	input.Name = ""
	_, err := svc.Create(context.Background(), input)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreate_SameVersions(t *testing.T) {
	svc := newTestService()
	input := validInput()
	input.TreatmentVersionID = input.ControlVersionID
	_, err := svc.Create(context.Background(), input)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreate_DuplicateName(t *testing.T) {
	svc := newTestService()
	input := validInput()
	_, err := svc.Create(context.Background(), input)
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), input)
	assert.ErrorIs(t, err, ErrConflict)
}

func TestStart_FromDraft(t *testing.T) {
	svc := newTestService()
	exp, _ := svc.Create(context.Background(), validInput())
	result, err := svc.Start(context.Background(), exp.ID)
	require.NoError(t, err)
	assert.Equal(t, "RUNNING", result.Status)
	assert.NotNil(t, result.StartedAt)
}

func TestStart_FromRunning_Fails(t *testing.T) {
	svc := newTestService()
	exp, _ := svc.Create(context.Background(), validInput())
	_, _ = svc.Start(context.Background(), exp.ID)
	_, err := svc.Start(context.Background(), exp.ID)
	assert.ErrorIs(t, err, ErrInvalidTransition)
}

func TestPause_FromRunning(t *testing.T) {
	svc := newTestService()
	exp, _ := svc.Create(context.Background(), validInput())
	_, _ = svc.Start(context.Background(), exp.ID)
	result, err := svc.Pause(context.Background(), exp.ID)
	require.NoError(t, err)
	assert.Equal(t, "PAUSED", result.Status)
}

func TestCancel_FromAnyActive(t *testing.T) {
	svc := newTestService()
	exp, _ := svc.Create(context.Background(), validInput())
	result, err := svc.Cancel(context.Background(), exp.ID)
	require.NoError(t, err)
	assert.Equal(t, "CANCELLED", result.Status)
}

func TestEvaluate_EmptyTelemetry(t *testing.T) {
	svc := newTestService()
	exp, _ := svc.Create(context.Background(), validInput())
	_, _ = svc.Start(context.Background(), exp.ID)
	result, err := svc.EvaluateExperiment(context.Background(), exp.ID)
	require.NoError(t, err)
	assert.Equal(t, "INCONCLUSIVE", result.Recommendation)
}

func TestCreate_InvalidMetric(t *testing.T) {
	svc := newTestService()
	input := validInput()
	input.PrimaryMetric = "invalid"
	_, err := svc.Create(context.Background(), input)
	assert.ErrorIs(t, err, ErrInvalidInput)
}
