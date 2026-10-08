//go:build integration

package drift

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/dbtest"
)

func TestAdapter_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	a := NewAdapter(pool)
	fx := dbtest.NewVersion(t, pool)
	dev := dbtest.NewDevice(t, pool, "")

	if _, err := pool.Exec(ctx, `UPDATE model_versions SET status = 'ACTIVE' WHERE id = $1`, fx.VersionID); err != nil {
		t.Fatal(err)
	}
	ids, err := a.ListActiveVersionIDs(ctx)
	require.NoError(t, err)
	assert.Contains(t, ids, fx.VersionID)

	if _, err := pool.Exec(ctx, `INSERT INTO telemetry_events (device_id, model_version_id, event_type, latency_ms, confidence_score, recorded_at)
		SELECT $1, $2, 'INFERENCE', 40 + g % 10, 0.8, NOW() - (g || ' minutes')::interval FROM generate_series(1, 60) g`, dev, fx.VersionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO telemetry_events (device_id, model_version_id, event_type, latency_ms, confidence_score, recorded_at)
		VALUES ($1, $2, 'MODEL_LOAD', 900, 0.5, NOW())`, dev, fx.VersionID); err != nil {
		t.Fatal(err)
	}
	lat, err := a.GetLatencyValues(ctx, fx.VersionID, time.Now().Add(-2*time.Hour), time.Now(), 1000)
	require.NoError(t, err)
	assert.Len(t, lat, 60, "model load times are not inference latency")
	conf, err := a.GetConfidenceValues(ctx, fx.VersionID, time.Now().Add(-2*time.Hour), time.Now(), 10)
	require.NoError(t, err)
	assert.Len(t, conf, 10, "limit applies")

	r := &DriftResult{ModelVersionID: fx.VersionID, Metric: "latency", PSIValue: 0.3, Severity: "CRITICAL", CheckedAt: time.Now()}
	require.NoError(t, a.SaveDriftResult(ctx, r))
	latest, err := a.LatestResult(ctx, fx.VersionID, "latency")
	require.NoError(t, err)
	require.NotNil(t, latest)
	assert.Equal(t, r.ID, latest.ID)
	none, err := a.LatestResult(ctx, fx.VersionID, "confidence")
	require.NoError(t, err)
	assert.Nil(t, none)

	r.PSIValue, r.CheckedAt = 0.4, time.Now()
	require.NoError(t, a.RefreshDriftResult(ctx, r.ID, r))
	hist, err := a.GetHistory(ctx, fx.VersionID, 10)
	require.NoError(t, err)
	require.Len(t, hist, 1, "refresh updates in place")
	assert.InDelta(t, 0.4, hist[0].PSIValue, 1e-9)
	cur, err := a.GetLatestResults(ctx, fx.VersionID)
	require.NoError(t, err)
	assert.Len(t, cur, 1)
}
