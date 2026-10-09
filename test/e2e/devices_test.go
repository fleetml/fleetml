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

// Deleting a device keeps its telemetry: an experiment's results and a
// version's drift are the same after the delete, for events that carry the
// arm ID and for events that do not (counted by the device's cohort).
func TestE2E_DeleteDeviceKeepsTelemetry(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	control, _, _ := e.newVersion(t, f, "1.0.0", "")
	treatment, _, _ := e.newVersion(t, f, "2.0.0", "")
	labels := map[string]string{"suite": randHex(4)}
	var exp idOnly
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/experiments", map[string]any{"project_id": f.project, "name": "keep-" + randHex(3),
		"control_version_id": control, "treatment_version_id": treatment, "target_selector": labels, "min_sample_size": 1000}, &exp))
	require.Equal(t, 200, e.call(t, "POST", "/api/v1/experiments/"+exp.ID+"/start", nil, nil))

	arms := map[string]*device{}
	armIDs := map[string]string{}
	for i := 0; i < 30 && len(arms) < 2; i++ {
		d := e.register(t, f.enrollKey, labels)
		resp := e.heartbeat(t, d)
		require.Len(t, resp.Experiments, 1)
		if arm := resp.Experiments[0].ArmName; arms[arm] == nil {
			arms[arm], armIDs[arm] = d, resp.Experiments[0].ArmId
		}
	}
	require.Len(t, arms, 2)

	send := func(d *device, version, armID string, at time.Time, confidence func(i int) float32) {
		events := make([]*fleetmlv1.TelemetryEvent, 60)
		for i := range events {
			events[i] = &fleetmlv1.TelemetryEvent{ModelVersionId: version, ExperimentArmId: armID, Type: fleetmlv1.TelemetryEvent_INFERENCE,
				LatencyMs: uint32(10 + i%7), ConfidenceScore: confidence(i), TimestampUnixMs: at.UnixMilli()}
		}
		ack, err := e.tel.ReportBatch(d.ctx(), &fleetmlv1.TelemetryBatch{DeviceId: d.id, Events: events})
		require.NoError(t, err)
		require.Equal(t, uint32(60), ack.AcceptedCount)
	}
	// The control device sends without an arm ID, as an SDK before arm IDs did:
	// a baseline three days before the experiment, then shifted values during it.
	send(arms["control"], control, "", time.Now().Add(-72*time.Hour), func(i int) float32 { return 0.5 + float32(i%5)/10 })
	send(arms["control"], control, "", time.Now(), func(i int) float32 { return 0.3 + float32(i%3)/10 })
	send(arms["treatment"], treatment, armIDs["treatment"], time.Now(), func(i int) float32 { return 0.9 })

	type results struct {
		Control   json.RawMessage `json:"control"`
		Treatment json.RawMessage `json:"treatment"`
	}
	sampleSize := func(raw json.RawMessage) int64 {
		var s struct {
			SampleSize int64 `json:"sample_size"`
		}
		_ = json.Unmarshal(raw, &s)
		return s.SampleSize
	}
	var before results
	waitFor(t, 20*time.Second, "results for both arms", func() bool {
		e.call(t, "GET", "/api/v1/experiments/"+exp.ID+"/results", nil, &before)
		return sampleSize(before.Control) == 60 && sampleSize(before.Treatment) == 60
	})

	type drift struct {
		Metric         string  `json:"metric"`
		PSI            float64 `json:"psi_value"`
		Severity       string  `json:"severity"`
		BaselineSample int     `json:"baseline_sample"`
		CurrentSample  int     `json:"current_sample"`
	}
	check := func() []drift {
		var out []drift
		require.Equal(t, 200, e.call(t, "POST", "/api/v1/drift/"+control+"/check", nil, &out))
		return out
	}
	history := func() []drift {
		var out []drift
		require.Equal(t, 200, e.call(t, "GET", "/api/v1/drift/"+control+"/history", nil, &out))
		return out
	}
	var driftBefore []drift
	waitFor(t, 20*time.Second, "a drift check with both windows", func() bool {
		driftBefore = check()
		return len(driftBefore) == 2
	})
	historyBefore := history()
	require.NotEmpty(t, historyBefore)

	for _, d := range arms {
		assert.Equal(t, 204, e.call(t, "DELETE", "/api/v1/devices/"+d.id, nil, nil))
	}

	var after results
	e.call(t, "GET", "/api/v1/experiments/"+exp.ID+"/results", nil, &after)
	assert.JSONEq(t, string(before.Control), string(after.Control), "control arm (events without an arm ID)")
	assert.JSONEq(t, string(before.Treatment), string(after.Treatment), "treatment arm (events with the arm ID)")
	assert.Equal(t, driftBefore, check(), "drift check")
	assert.Equal(t, historyBefore, history(), "drift history")
}
