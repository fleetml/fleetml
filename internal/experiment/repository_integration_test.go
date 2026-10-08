//go:build integration

package experiment

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/dbtest"
)

func TestRepository_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	fx := dbtest.NewVersion(t, pool)
	treatment, _ := dbtest.NewVersionOf(t, pool, fx.ModelID, "2.0.0")

	svc := NewService(repo, nil)
	exp, err := svc.Create(ctx, CreateInput{ProjectID: fx.ProjectID, Name: "e", ControlVersionID: fx.VersionID,
		TreatmentVersionID: treatment, MinSampleSize: 2, TargetSelector: map[string]string{"ab": "1"}})
	require.NoError(t, err)
	_, err = svc.Create(ctx, CreateInput{ProjectID: fx.ProjectID, Name: "e", ControlVersionID: fx.VersionID, TreatmentVersionID: treatment})
	assert.ErrorIs(t, err, ErrConflict)
	_, err = svc.Start(ctx, exp.ID)
	require.NoError(t, err)

	got, err := repo.GetByID(ctx, exp.ID)
	require.NoError(t, err)
	assert.Equal(t, "RUNNING", got.Status)
	assert.Equal(t, "1", got.TargetSelector["ab"])
	list, total, err := repo.List(ctx, &fx.ProjectID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, list, 1)
	running, err := repo.ListByStatus(ctx, []string{"RUNNING"})
	require.NoError(t, err)
	assert.Len(t, running, 1)
	_, err = repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, ErrNotFound)

	// Two devices, one per arm. The control device also sends one event
	// tagged with the treatment arm (e.g. after a cohort change): it counts
	// for treatment, because tagged events follow their tag.
	ctrlDev, treatDev := dbtest.NewDevice(t, pool, ""), dbtest.NewDevice(t, pool, "")
	require.NoError(t, repo.AssignCohort(ctx, &CohortAssignment{DeviceID: ctrlDev, ExperimentID: exp.ID, Cohort: "control"}))
	require.NoError(t, repo.AssignCohort(ctx, &CohortAssignment{DeviceID: treatDev, ExperimentID: exp.ID, Cohort: "treatment"}))
	ca, err := repo.GetCohortAssignment(ctx, ctrlDev, exp.ID)
	require.NoError(t, err)
	assert.Equal(t, "control", ca.Cohort)
	none, err := repo.GetCohortAssignment(ctx, uuid.New(), exp.ID)
	require.NoError(t, err)
	assert.Nil(t, none)
	all, err := repo.ListCohortAssignments(ctx, exp.ID)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	ctrlArm, treatArm := ArmID(exp.ID, "control"), ArmID(exp.ID, "treatment")
	if _, err := pool.Exec(ctx, `INSERT INTO telemetry_events (device_id, model_version_id, experiment_arm_id, event_type, latency_ms, confidence_score, recorded_at) VALUES
		($1, $3, $5, 'INFERENCE', 10, 0.8, NOW()), ($1, $3, NULL, 'INFERENCE', 20, 0.9, NOW()), ($1, $3, $5, 'OOM', NULL, NULL, NOW()),
		($2, $4, $6, 'INFERENCE', 30, 0.7, NOW()), ($1, $4, $6, 'INFERENCE', 40, 0.6, NOW()),
		($1, $3, $5, 'MODEL_LOAD', 900, 0, NOW()), ($1, $3, $5, 'MODEL_LOAD_FAILED', 0, 0, NOW())`,
		ctrlDev, treatDev, fx.VersionID, treatment, ctrlArm, treatArm); err != nil {
		t.Fatal(err)
	}
	since := time.Now().Add(-time.Hour)
	c, err := repo.GetCohortStats(ctx, exp.ID, fx.VersionID, since, "control")
	require.NoError(t, err)
	assert.Equal(t, int64(3), c.SampleSize, "two tagged events plus one untagged from a control device; model load events are not inferences")
	assert.Equal(t, int64(1), c.ErrorCount, "OOM counts as an error")
	assert.InDelta(t, 15, c.AvgLatency, 1e-9, "the 900 ms model load is not inference latency")
	assert.InDelta(t, 0.85, c.AvgConfidence, 1e-6)
	assert.Greater(t, c.StdDevConfidence, 0.0)
	tr, err := repo.GetCohortStats(ctx, exp.ID, treatment, since, "treatment")
	require.NoError(t, err)
	assert.Equal(t, int64(2), tr.SampleSize, "the tagged event from the control device follows its tag")

	result, err := svc.EvaluateExperiment(ctx, exp.ID)
	require.NoError(t, err)
	assert.True(t, result.Final, "both arms reached min_sample_size 2")
	stored, _ := repo.GetByID(ctx, exp.ID)
	require.NotNil(t, stored.Result)
	assert.True(t, stored.Result.Final, "the frozen result is persisted")

	require.NoError(t, repo.Delete(ctx, exp.ID))
	assert.ErrorIs(t, repo.Delete(ctx, exp.ID), ErrNotFound)
}
