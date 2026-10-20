//go:build integration

package deploy

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/dbtest"
)

func TestRepo_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo := NewRepo(pool)
	fx := dbtest.NewVersion(t, pool)

	d := &Deployment{ProjectID: fx.ProjectID, ModelVersionID: fx.VersionID, Name: "r", Strategy: "ROLLING", Status: "PENDING",
		TargetSelector: map[string]string{"fleet": "a"}, RollingConfig: &RollingConfig{BatchSize: 5, BatchInterval: "30s"},
		HealthGate: DefaultHealthGate(), CreatedBy: "test"}
	require.NoError(t, repo.Create(ctx, d))

	now := time.Now().Truncate(time.Microsecond)
	threshold := int64(12345)
	d.Status, d.CurrentStage, d.StageStartedAt, d.RolloutThreshold = "ROLLING_OUT", 2, &now, &threshold
	require.NoError(t, repo.Update(ctx, d))

	got, err := repo.GetByID(ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, "ROLLING_OUT", got.Status)
	assert.Equal(t, 2, got.CurrentStage)
	assert.Equal(t, threshold, *got.RolloutThreshold)
	assert.True(t, now.Equal(*got.StageStartedAt))
	assert.Equal(t, 5, got.RollingConfig.BatchSize)
	assert.Equal(t, 0.05, got.HealthGate.MaxErrorRate)
	assert.Equal(t, "a", got.TargetSelector["fleet"])

	list, total, err := repo.List(ctx, nil, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, threshold, *list[0].RolloutThreshold)
	_, total, err = repo.List(ctx, &d.ProjectID, 10, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total, "the deployment's own project")
	other := uuid.New()
	_, total, err = repo.List(ctx, &other, 10, 0)
	require.NoError(t, err)
	assert.Zero(t, total, "another project")
	active, err := repo.ListByStatus(ctx, []string{"ROLLING_OUT"})
	require.NoError(t, err)
	assert.Len(t, active, 1)

	_, err = repo.GetByID(ctx, uuid.New())
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, repo.Update(ctx, &Deployment{ID: uuid.New()}), ErrNotFound)
	require.NoError(t, repo.Delete(ctx, d.ID))
	assert.ErrorIs(t, repo.Delete(ctx, d.ID), ErrNotFound)
}

func TestAdapters_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	fx := dbtest.NewVersion(t, pool)
	v2, v2Variant := dbtest.NewVersionOf(t, pool, fx.ModelID, "2.0.0")
	v3, _ := dbtest.NewVersionOf(t, pool, fx.ModelID, "3.0.0")
	if _, err := pool.Exec(ctx, `UPDATE model_variants SET status = 'FAILED' WHERE version_id = $1`, v3); err != nil {
		t.Fatal(err)
	}

	versions := NewVersionAdapter(pool)
	n, err := versions.ReadyVariantCount(ctx, fx.VersionID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	n, err = versions.ReadyVariantCount(ctx, v3)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "FAILED variants do not count")

	cur, err := versions.CurrentVersion(ctx, fx.ProjectID, v2)
	require.NoError(t, err)
	assert.Nil(t, cur, "nothing deployed yet")
	repo := NewRepo(pool)
	v1Dep := &Deployment{ProjectID: fx.ProjectID, ModelVersionID: fx.VersionID, Strategy: "ALL_AT_ONCE", Status: "COMPLETE", CreatedBy: "t"}
	require.NoError(t, repo.Create(ctx, v1Dep))
	cur, err = versions.CurrentVersion(ctx, fx.ProjectID, v2)
	require.NoError(t, err)
	require.NotNil(t, cur)
	assert.Equal(t, fx.VersionID, *cur)

	devices := NewDeviceCountAdapter(pool)
	a := dbtest.NewProjectDevice(t, pool, fx.ProjectID, `{"fleet":"x","region":"eu"}`)
	b := dbtest.NewProjectDevice(t, pool, fx.ProjectID, `{"fleet":"x","region":"us"}`)
	dbtest.NewProjectDevice(t, pool, fx.ProjectID, `{"fleet":"y"}`)
	// Matching labels in another project, or without a project, do not count:
	// those devices never get this project's deployments.
	dbtest.NewProjectDevice(t, pool, dbtest.NewVersion(t, pool).ProjectID, `{"fleet":"x"}`)
	dbtest.NewDevice(t, pool, `{"fleet":"x"}`)
	ids, err := devices.ListMatchingDevices(ctx, fx.ProjectID, map[string]string{"fleet": "x"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []uuid.UUID{a, b}, ids)
	all, err := devices.ListMatchingDevices(ctx, fx.ProjectID, nil)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// a reports v2 loaded, b does not: one unavailable device.
	if _, err := pool.Exec(ctx, `UPDATE devices SET loaded_variant_ids = $2 WHERE id = $1`, a, `["`+v2Variant.String()+`"]`); err != nil {
		t.Fatal(err)
	}
	un, err := devices.CountUnavailable(ctx, []uuid.UUID{a, b}, v2, time.Now().Add(-time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 1, un)
	un, err = devices.CountUnavailable(ctx, []uuid.UUID{a, b}, v2, time.Now().Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, 0, un, "devices not seen recently are not counted")
	loaded, err := devices.CountLoaded(ctx, []uuid.UUID{a, b}, v2)
	require.NoError(t, err)
	assert.Equal(t, 1, loaded, "a reports v2 loaded")
	loaded, err = devices.CountLoaded(ctx, []uuid.UUID{a, b}, fx.VersionID)
	require.NoError(t, err)
	assert.Zero(t, loaded, "nobody reports 1.0.0")

	// Cohort stats count only the given devices and all error event types.
	tel := NewTelemetryAdapter(pool)
	if _, err := pool.Exec(ctx, `INSERT INTO telemetry_events (device_id, model_version_id, event_type, latency_ms, error_message, recorded_at) VALUES
		($1, $3, 'INFERENCE', 100, NULL, NOW()), ($1, $3, 'OOM', NULL, NULL, NOW()), ($1, $3, 'MODEL_LOAD_FAILED', NULL, '', NOW()),
		($1, $3, 'INFERENCE', 200, 'bad output', NOW()), ($2, $3, 'ERROR', NULL, 'outside the cohort', NOW())`, a, b, v2); err != nil {
		t.Fatal(err)
	}
	stats, err := tel.CohortStats(ctx, v2, []uuid.UUID{a}, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(4), stats.TotalEvents)
	assert.Equal(t, int64(3), stats.ErrorCount, "OOM, MODEL_LOAD_FAILED and the message-carrying event")
	assert.Greater(t, stats.P95LatencyMs, 100.0)
	empty, err := tel.CohortStats(ctx, v2, nil, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(0), empty.TotalEvents)
	p95, events, err := tel.VersionP95(ctx, v2, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), events)
	assert.Greater(t, p95, 100.0)
}
