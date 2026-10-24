//go:build integration

package registry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/dbtest"
)

// A project whose devices sent telemetry is deleted with everything in it,
// on the real TimescaleDB hypertable; another project's rows stay.
func TestProjectDelete_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo := NewProjectRepo(pool)

	fx := dbtest.NewVersion(t, pool)
	v2, _ := dbtest.NewVersionOf(t, pool, fx.ModelID, "2.0.0")
	other := dbtest.NewVersion(t, pool)
	dev := dbtest.NewProjectDevice(t, pool, fx.ProjectID, "")
	otherDev := dbtest.NewProjectDevice(t, pool, other.ProjectID, "")

	exec := func(q string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, q, args...)
		require.NoError(t, err)
	}
	var hypertable bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'telemetry_events')`).Scan(&hypertable))
	require.True(t, hypertable, "telemetry_events is a hypertable")

	var dep, exp uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO deployments (project_id, model_version_id, previous_version_id) VALUES ($1, $2, $3) RETURNING id`, fx.ProjectID, v2, fx.VersionID).Scan(&dep))
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO experiments (project_id, name, control_version_id, treatment_version_id) VALUES ($1, 'ab', $2, $3) RETURNING id`, fx.ProjectID, fx.VersionID, v2).Scan(&exp))
	exec(`INSERT INTO cohort_assignments (device_id, experiment_id, cohort) VALUES ($1, $2, 'control')`, dev, exp)
	exec(`INSERT INTO enrollment_keys (project_id, name, key_hash) VALUES ($1, 'k', $2)`, fx.ProjectID, uuid.NewString()+"0000000000000000000000000000")
	// Telemetry in two weekly chunks, drift results and histograms, for both
	// projects.
	for _, r := range []struct{ dev, ver uuid.UUID }{{dev, fx.VersionID}, {dev, v2}, {otherDev, other.VersionID}} {
		for _, at := range []time.Time{time.Now(), time.Now().Add(-10 * 24 * time.Hour)} {
			exec(`INSERT INTO telemetry_events (device_id, model_version_id, event_type, latency_ms, recorded_at) VALUES ($1, $2, 'INFERENCE', 5, $3)`, r.dev, r.ver, at)
		}
		exec(`INSERT INTO drift_results (model_version_id, metric, psi_value, severity, baseline_from, baseline_to, current_from, current_to) VALUES ($1, 'latency', 0.3, 'CRITICAL', NOW(), NOW(), NOW(), NOW())`, r.ver)
		exec(`INSERT INTO drift_points (model_version_id, metric, psi, severity) VALUES ($1, 'latency', 0.3, 'CRITICAL')`, r.ver)
		exec(`INSERT INTO feature_histograms (recorded_at, model_version_id, feature, counts) VALUES (NOW(), $1, 'f', '{1,2}')`, r.ver)
	}

	require.NoError(t, repo.Delete(ctx, fx.ProjectID))

	count := func(q string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, pool.QueryRow(ctx, q, args...).Scan(&n))
		return n
	}
	for _, table := range []string{"models", "deployments", "experiments", "enrollment_keys", "devices"} {
		assert.Zero(t, count(`SELECT COUNT(*) FROM `+table+` WHERE project_id = $1`, fx.ProjectID), table)
	}
	assert.Zero(t, count(`SELECT COUNT(*) FROM projects WHERE id = $1`, fx.ProjectID))
	assert.Zero(t, count(`SELECT COUNT(*) FROM model_versions WHERE model_id = $1`, fx.ModelID))
	versions := []uuid.UUID{fx.VersionID, v2}
	for _, table := range []string{"telemetry_events", "drift_results", "drift_points", "feature_histograms"} {
		assert.Zero(t, count(`SELECT COUNT(*) FROM `+table+` WHERE model_version_id = ANY($1)`, versions), table)
	}
	assert.Zero(t, count(`SELECT COUNT(*) FROM devices WHERE id = $1`, dev), "the project's device is deleted")

	// The other project is untouched.
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM devices WHERE id = $1`, otherDev))
	assert.Equal(t, 2, count(`SELECT COUNT(*) FROM telemetry_events WHERE model_version_id = $1`, other.VersionID))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM drift_results WHERE model_version_id = $1`, other.VersionID))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM feature_histograms WHERE model_version_id = $1`, other.VersionID))

	err := repo.Delete(ctx, fx.ProjectID)
	assert.True(t, errors.Is(err, ErrNotFound), "a second delete is NOT_FOUND: %v", err)

	// A version with telemetry can be deleted on its own too (migration 000019).
	exec(`DELETE FROM model_versions WHERE id = $1`, other.VersionID)
	assert.Zero(t, count(`SELECT COUNT(*) FROM telemetry_events WHERE model_version_id = $1`, other.VersionID))
}
