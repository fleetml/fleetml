//go:build e2e

package e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

// V3.2: a feature whose input distribution shifts goes CRITICAL on its own,
// with exactly one alert; an unchanged feature stays NONE.
func TestE2E_FeatureDrift(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	v1, _, _ := e.newVersion(t, f, "1.0.0", "")

	features := map[string]any{"features": []map[string]any{
		{"name": "petal_length", "edges": []float64{1, 2, 3, 4, 5}},
		{"name": "sepal_width", "edges": []float64{2, 2.5, 3, 3.5}},
	}}
	assert.Equal(t, 400, e.call(t, "PUT", "/api/v1/models/"+f.model+"/features",
		map[string]any{"features": []map[string]any{{"name": "x", "edges": []float64{2, 1}}}}, nil), "edges must increase")
	assert.Equal(t, 404, e.call(t, "PUT", "/api/v1/models/00000000-0000-0000-0000-000000000001/features", features, nil))
	require.Equal(t, 200, e.call(t, "PUT", "/api/v1/models/"+f.model+"/features", features, nil))

	hook := newRecorder(t)
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/projects/"+f.project+"/alert-sinks",
		map[string]any{"name": "drift", "kind": "webhook", "config": map[string]string{"url": hook.srv.URL}}, nil))

	labels := map[string]string{"suite": randHex(4)}
	e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v1, "target_selector": labels})
	d := e.register(t, f.enrollKey, labels)
	resp := e.heartbeat(t, d)
	require.Len(t, resp.Config.FeatureBins, 2, "devices get the bin edges of their models' features")
	assert.Equal(t, f.model, resp.Config.FeatureBins[0].ModelId)

	send := func(at time.Time, petal, sepal []uint32) {
		events := make([]*fleetmlv1.TelemetryEvent, 10)
		for i := range events {
			events[i] = &fleetmlv1.TelemetryEvent{ModelVersionId: v1, Type: fleetmlv1.TelemetryEvent_INFERENCE, LatencyMs: 5,
				TimestampUnixMs: at.UnixMilli(), Features: []*fleetmlv1.FeatureHistogram{
					{Name: "petal_length", Counts: petal}, {Name: "sepal_width", Counts: sepal}}}
		}
		ack, err := e.tel.ReportBatch(d.ctx(), &fleetmlv1.TelemetryBatch{DeviceId: d.id, Events: events})
		require.NoError(t, err)
		require.Equal(t, uint32(10), ack.AcceptedCount)
	}
	sepal := []uint32{1, 2, 5, 2, 1}
	send(time.Now().Add(-72*time.Hour), []uint32{1, 2, 4, 2, 1, 0}, sepal) // the week before
	send(time.Now().Add(-time.Minute), []uint32{0, 0, 1, 2, 4, 3}, sepal)  // the last day: petal_length shifted up

	type result struct {
		Metric   string  `json:"metric"`
		Severity string  `json:"severity"`
		PSI      float64 `json:"psi_value"`
	}
	bySeverity := map[string]string{}
	waitFor(t, 20*time.Second, "both features checked", func() bool {
		var results []result
		e.call(t, "POST", "/api/v1/drift/"+v1+"/check", nil, &results)
		for _, r := range results {
			bySeverity[r.Metric] = r.Severity
		}
		return bySeverity["feature:sepal_width"] != ""
	})
	assert.Equal(t, "CRITICAL", bySeverity["feature:petal_length"])
	assert.Equal(t, "NONE", bySeverity["feature:sepal_width"])

	// Checking again changes nothing: still one alert.
	e.call(t, "POST", "/api/v1/drift/"+v1+"/check", nil, nil)
	waitFor(t, 10*time.Second, "the drift alert", func() bool { b, _ := hook.received(); return len(b) > 0 })
	time.Sleep(2 * time.Second)
	bodies, _ := hook.received()
	require.Len(t, bodies, 1, "exactly one alert")
	var a struct {
		Type  string `json:"type"`
		Drift struct {
			Metric string `json:"metric"`
		} `json:"drift"`
	}
	require.NoError(t, json.Unmarshal(bodies[0], &a))
	assert.Equal(t, "drift", a.Type)
	assert.Equal(t, "feature:petal_length", a.Drift.Metric)

	var points []struct {
		Metric string `json:"metric"`
	}
	require.Equal(t, 200, e.call(t, "GET", "/api/v1/drift/"+v1+"/points", nil, &points))
	n := 0
	for _, p := range points {
		if p.Metric == "feature:petal_length" {
			n++
		}
	}
	assert.GreaterOrEqual(t, n, 2, "every check adds a point to the PSI-over-time chart")
}
