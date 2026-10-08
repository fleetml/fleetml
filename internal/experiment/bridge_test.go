package experiment

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/device"
	"github.com/fleetml/fleetml/internal/intelligence"
)

// fakeResolver resolves each version to one variant of the same model.
type fakeResolver struct {
	modelID  uuid.UUID
	variants map[uuid.UUID]string // version -> variant id
}

func (f fakeResolver) ResolveVersion(_ context.Context, _ *device.Device, versionID uuid.UUID) (*intelligence.Assignment, error) {
	v, ok := f.variants[versionID]
	if !ok {
		return nil, nil
	}
	return &intelligence.Assignment{
		ModelID: f.modelID.String(), VersionID: versionID.String(), ModelName: "clf", Version: "arm-" + v,
		VariantID: v, DownloadURL: "https://s3.example.com/" + v, BlobSizeBytes: 42, BlobSHA256: "sha-" + v, Action: "LOAD",
	}, nil
}

func (f fakeResolver) GetProfiler() *intelligence.Profiler { return intelligence.NewProfiler(nil) }

func runningExperiment(t *testing.T, svc *Service) *Experiment {
	t.Helper()
	exp, err := svc.Create(context.Background(), validInput())
	require.NoError(t, err)
	exp, err = svc.Start(context.Background(), exp.ID)
	require.NoError(t, err)
	return exp
}

func TestArmID(t *testing.T) {
	e := uuid.New()
	assert.Equal(t, ArmID(e, "control"), ArmID(e, "control"))
	assert.NotEqual(t, ArmID(e, "control"), ArmID(e, "treatment"))
	assert.NotEqual(t, ArmID(e, "control"), ArmID(uuid.New(), "control"))
}

// Each device gets its own arm's model with a real URL, sha and version, and
// learns its arm ID; an already-loaded arm model is kept, not re-sent.
func TestBridge_PlanExperiments(t *testing.T) {
	svc := newTestService()
	exp := runningExperiment(t, svc)
	res := fakeResolver{modelID: uuid.New(), variants: map[uuid.UUID]string{
		exp.ControlVersionID: "ctrl-variant", exp.TreatmentVersionID: "treat-variant",
	}}
	bridge := NewBridge(svc, res, nil)

	arms := map[string]int{}
	for i := 0; i < 40; i++ {
		dev := &device.Device{ID: uuid.New(), ProjectID: &exp.ProjectID, OS: "ANDROID", RAMMB: 4096, Labels: map[string]string{}}
		plan, err := bridge.PlanExperiments(context.Background(), dev, nil)
		require.NoError(t, err)
		require.Len(t, plan.Experiments, 1)
		require.Len(t, plan.Assignments, 1)
		ea, a := plan.Experiments[0], plan.Assignments[0]
		arms[ea.ArmName]++
		assert.Equal(t, ArmID(exp.ID, ea.ArmName).String(), ea.ArmID)
		assert.Equal(t, ea.ModelVersionID, a.VersionID)
		assert.NotEmpty(t, a.DownloadURL)
		assert.Equal(t, "sha-"+a.VariantID, a.BlobSHA256)
		assert.Equal(t, "arm-"+a.VariantID, a.Version, "the real version string, not the arm name")
		assert.True(t, plan.Keep[a.VariantID])
		assert.True(t, plan.Models[res.modelID.String()])

		// Next heartbeat with the arm model loaded: nothing to send, still kept.
		again, err := bridge.PlanExperiments(context.Background(), dev,
			[]device.ModelStatus{{ModelID: a.ModelID, VariantID: a.VariantID}})
		require.NoError(t, err)
		assert.Empty(t, again.Assignments)
		assert.True(t, again.Keep[a.VariantID])
	}
	assert.Positive(t, arms["control"])
	assert.Positive(t, arms["treatment"])
}

// Experiments never reach devices of another project, nor devices without one.
func TestBridge_PlanExperimentsStaysInItsProject(t *testing.T) {
	svc := newTestService()
	exp := runningExperiment(t, svc)
	res := fakeResolver{modelID: uuid.New(), variants: map[uuid.UUID]string{
		exp.ControlVersionID: "ctrl-variant", exp.TreatmentVersionID: "treat-variant",
	}}
	bridge := NewBridge(svc, res, nil)
	other := uuid.New()
	for _, project := range []*uuid.UUID{&other, nil} {
		dev := &device.Device{ID: uuid.New(), ProjectID: project, OS: "ANDROID", RAMMB: 4096, Labels: map[string]string{}}
		plan, err := bridge.PlanExperiments(context.Background(), dev, nil)
		require.NoError(t, err)
		assert.Empty(t, plan.Experiments)
		assert.Empty(t, plan.Assignments)
	}
}

func TestMaxDuration(t *testing.T) {
	d, err := ParseMaxDuration("7d")
	require.NoError(t, err)
	assert.Equal(t, 7*24*time.Hour, d)
	d, err = ParseMaxDuration("90m")
	require.NoError(t, err)
	assert.Equal(t, 90*time.Minute, d)
	for _, bad := range []string{"", "0d", "-1h", "soon", "1w"} {
		_, err := ParseMaxDuration(bad)
		assert.Error(t, err, bad)
	}

	in := validInput()
	in.MaxDuration = "forever"
	_, err = newTestService().Create(context.Background(), in)
	assert.ErrorIs(t, err, ErrInvalidInput)

	started := time.Now().Add(-2 * time.Hour)
	exp := &Experiment{MaxDuration: "1h", StartedAt: &started}
	assert.True(t, exp.Expired(time.Now()))
	exp.MaxDuration = "3h"
	assert.False(t, exp.Expired(time.Now()))
}

// The ticker concludes a running experiment once max_duration has passed.
func TestTicker_ConcludesAtMaxDuration(t *testing.T) {
	svc := newTestService()
	exp := runningExperiment(t, svc)
	repo := svc.repo.(*mockRepo)
	past := time.Now().Add(-8 * 24 * time.Hour)
	repo.experiments[exp.ID].StartedAt = &past

	NewTicker(svc, time.Minute, nil).tick(context.Background())

	got, err := svc.Get(context.Background(), exp.ID)
	require.NoError(t, err)
	assert.Equal(t, "CONCLUDED", got.Status)
	require.NotNil(t, got.WinnerVersionID)
	assert.Equal(t, exp.ControlVersionID, *got.WinnerVersionID, "no significant result: keep control")
	assert.Contains(t, got.ConclusionNote, "max_duration")
}
