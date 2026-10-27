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
	stats       map[string]CohortStats       // cohort -> stats returned by GetCohortStats
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		experiments: make(map[uuid.UUID]*Experiment),
		names:       make(map[string]uuid.UUID),
		cohorts:     make(map[string]*CohortAssignment),
		stats:       make(map[string]CohortStats),
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

func (m *mockRepo) GetCohortStats(_ context.Context, _, _ uuid.UUID, _ time.Time, cohort string) (*CohortStats, error) {
	if s, ok := m.stats[cohort]; ok {
		cp := s
		return &cp, nil
	}
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
	assert.Equal(t, "COLLECTING_DATA", result.Recommendation)
}

func TestCreate_InvalidMetric(t *testing.T) {
	svc := newTestService()
	input := validInput()
	input.PrimaryMetric = "invalid"
	_, err := svc.Create(context.Background(), input)
	assert.ErrorIs(t, err, ErrInvalidInput)
}

type fakeVersions map[uuid.UUID]int

func (f fakeVersions) ReadyVariantCount(_ context.Context, id uuid.UUID) (int, error) {
	return f[id], nil
}
func (f fakeVersions) ProjectExists(context.Context, uuid.UUID) (bool, error) { return true, nil }

// VersionProject puts every version in the project of the input's project
// (tests set it with sameProject), so only the variant counts differ.
func (f fakeVersions) VersionProject(context.Context, uuid.UUID) (uuid.UUID, error) {
	return sameProject, nil
}

var sameProject = uuid.New()

func TestCreate_ArmsNeedReadyVariants(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.ProjectID = sameProject
	svc.SetVersionChecker(fakeVersions{in.ControlVersionID: 1})
	_, err := svc.Create(context.Background(), in)
	assert.ErrorIs(t, err, ErrNotDeployable, "treatment has no READY variant")

	svc.SetVersionChecker(fakeVersions{in.ControlVersionID: 1, in.TreatmentVersionID: 2})
	_, err = svc.Create(context.Background(), in)
	assert.NoError(t, err)
}

// Fixed horizon: no test (and no p-value) before both arms reach
// min_sample_size; one test when they do; the decision never changes after.
func TestEvaluate_FixedHorizon(t *testing.T) {
	svc := newTestService()
	repo := svc.repo.(*mockRepo)
	in := validInput()
	in.MinSampleSize = 100
	exp, err := svc.Create(context.Background(), in)
	require.NoError(t, err)
	_, err = svc.Start(context.Background(), exp.ID)
	require.NoError(t, err)

	// A striking but early difference: not tested yet.
	repo.stats["control"] = CohortStats{SampleSize: 99, AvgLatency: 50, StdDevLatency: 5}
	repo.stats["treatment"] = CohortStats{SampleSize: 150, AvgLatency: 30, StdDevLatency: 5}
	r, err := svc.EvaluateExperiment(context.Background(), exp.ID)
	require.NoError(t, err)
	assert.False(t, r.Final)
	assert.Nil(t, r.PValue, "no p-value before the horizon")
	assert.False(t, r.IsSignificant)
	assert.Equal(t, "COLLECTING_DATA", r.Recommendation)

	// Horizon reached: tested once.
	repo.stats["control"] = CohortStats{SampleSize: 100, AvgLatency: 50, StdDevLatency: 5}
	r, err = svc.EvaluateExperiment(context.Background(), exp.ID)
	require.NoError(t, err)
	assert.True(t, r.Final)
	require.NotNil(t, r.TestedAt)
	assert.True(t, r.IsSignificant)
	assert.Equal(t, "ADOPT_TREATMENT", r.Recommendation)

	// Later data cannot change the decision.
	repo.stats["treatment"] = CohortStats{SampleSize: 5000, AvgLatency: 50, StdDevLatency: 5}
	again, err := svc.EvaluateExperiment(context.Background(), exp.ID)
	require.NoError(t, err)
	assert.Equal(t, r.Recommendation, again.Recommendation)
	assert.Equal(t, *r.PValue, *again.PValue)
	assert.Equal(t, int64(150), again.TreatmentStats.SampleSize, "stats are those at the horizon")
}

// auto_conclude acts on the single fixed-horizon test, not on every tick.
func TestTicker_AutoConcludesAtHorizon(t *testing.T) {
	svc := newTestService()
	repo := svc.repo.(*mockRepo)
	in := validInput()
	in.MinSampleSize = 50
	in.AutoConclude = true
	exp, err := svc.Create(context.Background(), in)
	require.NoError(t, err)
	_, err = svc.Start(context.Background(), exp.ID)
	require.NoError(t, err)
	ticker := NewTicker(svc, time.Minute, nil)

	repo.stats["control"] = CohortStats{SampleSize: 10, AvgLatency: 50, StdDevLatency: 5}
	repo.stats["treatment"] = CohortStats{SampleSize: 10, AvgLatency: 30, StdDevLatency: 5}
	ticker.tick(context.Background())
	got, _ := svc.Get(context.Background(), exp.ID)
	assert.Equal(t, "RUNNING", got.Status, "a dramatic early difference does not conclude early")

	repo.stats["control"] = CohortStats{SampleSize: 60, AvgLatency: 50, StdDevLatency: 5}
	repo.stats["treatment"] = CohortStats{SampleSize: 60, AvgLatency: 30, StdDevLatency: 5}
	ticker.tick(context.Background())
	got, _ = svc.Get(context.Background(), exp.ID)
	assert.Equal(t, "CONCLUDED", got.Status)
	assert.Equal(t, exp.TreatmentVersionID, *got.WinnerVersionID)
}

// projects knows which projects exist and which project each version is in.
type projects struct {
	exist    map[uuid.UUID]bool
	versions map[uuid.UUID]uuid.UUID
}

func (p projects) ReadyVariantCount(context.Context, uuid.UUID) (int, error) { return 1, nil }
func (p projects) ProjectExists(_ context.Context, id uuid.UUID) (bool, error) {
	return p.exist[id], nil
}
func (p projects) VersionProject(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
	return p.versions[id], nil
}

// An experiment belongs to its project, as a deployment does: an arm of
// another project is refused, an unknown project or version is not found.
func TestCreate_ArmsMustBelongToTheProject(t *testing.T) {
	mine, other := uuid.New(), uuid.New()
	a, b, foreign := uuid.New(), uuid.New(), uuid.New()
	svc := newTestService()
	svc.SetVersionChecker(projects{exist: map[uuid.UUID]bool{mine: true, other: true},
		versions: map[uuid.UUID]uuid.UUID{a: mine, b: mine, foreign: other}})
	in := validInput()
	in.ProjectID, in.ControlVersionID, in.TreatmentVersionID = mine, a, foreign
	_, err := svc.Create(context.Background(), in)
	require.ErrorIs(t, err, ErrInvalidInput)
	assert.Contains(t, err.Error(), "belongs to another project")

	in.ProjectID, in.TreatmentVersionID = uuid.Nil, b
	_, err = svc.Create(context.Background(), in)
	assert.ErrorIs(t, err, ErrNotFound, "the all-zeros project is an unknown one")

	in.ProjectID, in.TreatmentVersionID = mine, uuid.New()
	_, err = svc.Create(context.Background(), in)
	assert.ErrorIs(t, err, ErrNotFound, "an unknown version")

	in.TreatmentVersionID = b
	_, err = svc.Create(context.Background(), in)
	assert.NoError(t, err)
}

// split_ratio left out is 50; outside 1-99 it is refused, not replaced.
func TestCreate_SplitRatio(t *testing.T) {
	svc := newTestService()
	for _, bad := range []int{-5, 100, 101} {
		in := validInput()
		in.SplitRatio = bad
		_, err := svc.Create(context.Background(), in)
		require.ErrorIs(t, err, ErrInvalidInput, "split %d", bad)
		assert.Contains(t, err.Error(), "split_ratio must be 1-99")
	}
	in := validInput()
	exp, err := svc.Create(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, 50, exp.SplitRatio)
}
