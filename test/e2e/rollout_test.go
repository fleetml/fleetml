//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

const zeroSHA = "0000000000000000000000000000000000000000000000000000000000000000"

// 3.5: a blob that does not match its declared sha256 is rejected, deleted,
// and its version cannot be deployed.
func TestE2E_ArtifactIntegrity(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)

	bad, _, status := e.newVersion(t, f, "1.0.0", zeroSHA)
	assert.Equal(t, 422, status, "finalize rejects the wrong hash")
	code := e.call(t, "POST", "/api/v1/deployments", map[string]any{"project_id": f.project, "model_version_id": bad}, nil)
	assert.Equal(t, 422, code, "a version without a READY variant is not deployable")

	good, sha, status := e.newVersion(t, f, "1.0.1", "")
	require.Equal(t, 200, status)
	e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": good})
	d := e.register(t, f.enrollKey, nil)
	e.heartbeat(t, d)
	got := d.loaded[f.model]
	require.NotNil(t, got)
	assert.Equal(t, good, got.VersionId)
	assert.Equal(t, sha, got.BlobSha256)
	assert.Equal(t, sha, fetchSHA(t, got.DownloadUrl), "the served blob is the verified one")
}

// 3.4: rolling back a COMPLETE deployment sends devices back to the previous
// version with a URL and sha; cancel and the status state machine work.
func TestE2E_RollbackFromComplete(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	v1, sha1, _ := e.newVersion(t, f, "1.0.0", "")
	v2, _, _ := e.newVersion(t, f, "2.0.0", "")
	labels := map[string]string{"suite": randHex(4)}

	e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v1, "target_selector": labels})
	d2 := e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v2, "target_selector": labels})
	var dep struct {
		PreviousVersionID string `json:"previous_version_id"`
	}
	e.call(t, "GET", "/api/v1/deployments/"+d2, nil, &dep)
	assert.Equal(t, v1, dep.PreviousVersionID)

	d := e.register(t, f.enrollKey, labels)
	e.heartbeat(t, d)
	require.Equal(t, v2, d.loaded[f.model].VersionId, "one version per model: the newest deployment wins")

	require.Equal(t, 200, e.call(t, "POST", "/api/v1/deployments/"+d2+"/rollback", map[string]string{"reason": "e2e"}, nil))
	resp := e.heartbeat(t, d)
	var update *fleetmlv1.ModelAssignment
	for _, a := range resp.Assignments {
		if a.Action == fleetmlv1.ModelAssignment_UPDATE {
			update = a
		}
	}
	require.NotNil(t, update, "rollback sends an UPDATE, not a bare UNLOAD")
	assert.Equal(t, v1, update.VersionId)
	assert.Equal(t, sha1, update.BlobSha256)
	assert.Equal(t, sha1, fetchSHA(t, update.DownloadUrl))

	assert.Equal(t, 409, e.call(t, "POST", "/api/v1/deployments/"+d2+"/rollback", nil, nil), "ROLLED_BACK is terminal")
	d3 := e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v2, "strategy": "CANARY", "target_selector": labels})
	assert.Equal(t, 200, e.call(t, "POST", "/api/v1/deployments/"+d3+"/cancel", nil, nil))
	assert.Equal(t, "CANCELLED", e.deploymentStatus(t, d3))
	assert.Equal(t, 409, e.call(t, "PATCH", "/api/v1/deployments/"+d3+"/status", map[string]string{"status": "ROLLING_OUT"}, nil))
}

// 3.3: a canary created without a gate rolls back when its cohort reports
// errors, and errors from devices outside the cohort do not trigger it.
func TestE2E_HealthGateIsCohortScoped(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	v, _, _ := e.newVersion(t, f, "1.0.0", "")
	labels := map[string]string{"suite": randHex(4)}
	var devs []*device
	for i := 0; i < 12; i++ {
		devs = append(devs, e.register(t, f.enrollKey, labels))
	}
	id := e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v, "strategy": "CANARY", "target_selector": labels,
		"canary_stages": []map[string]any{{"percentage": 50, "min_duration": "1h"}, {"percentage": 100}}})

	var in, out []*device
	for _, d := range devs {
		e.heartbeat(t, d)
		if d.loaded[f.model] != nil {
			in = append(in, d)
		} else {
			out = append(out, d)
		}
	}
	require.NotEmpty(t, in)
	require.NotEmpty(t, out)

	for _, d := range out {
		e.report(t, d, v, "", 10, fleetmlv1.TelemetryEvent_ERROR, 0)
	}
	time.Sleep(8 * time.Second) // telemetry flush (5s) + several deploy ticks
	assert.Equal(t, "ROLLING_OUT", e.deploymentStatus(t, id), "errors outside the cohort are ignored")

	for _, d := range in {
		e.report(t, d, v, "", 25/len(in)+1, fleetmlv1.TelemetryEvent_OOM, 0)
	}
	waitFor(t, 20*time.Second, "automatic rollback", func() bool { return e.deploymentStatus(t, id) == "ROLLED_BACK" })
}

// 3.2: ROLLING exposes N x batch_size devices per stage on the hash ring,
// advancing after batch_interval, and completes.
func TestE2E_RollingBatches(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	v, _, _ := e.newVersion(t, f, "1.0.0", "")
	labels := map[string]string{"suite": randHex(4)}
	var devs []*device
	for i := 0; i < 20; i++ {
		devs = append(devs, e.register(t, f.enrollKey, labels))
	}
	id := e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v, "strategy": "ROLLING", "target_selector": labels,
		"rolling_config": map[string]any{"batch_size": 5, "batch_interval": "3s"}})

	onNew := func() int {
		n := 0
		for _, d := range devs {
			e.heartbeat(t, d)
			if d.loaded[f.model] != nil {
				n++
			}
		}
		return n
	}
	var seen []int
	waitFor(t, 60*time.Second, "rollout to complete", func() bool {
		if n := onNew(); len(seen) == 0 || seen[len(seen)-1] != n {
			seen = append(seen, n)
		}
		return e.deploymentStatus(t, id) == "COMPLETE"
	})
	assert.Equal(t, []int{5, 10, 15, 20}, seen)
}

// 3.1: devices in different arms each get their own arm's model with URL and
// sha, report tagged telemetry, and the results show both arms.
func TestE2E_ABExperiment(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	control, ctrlSHA, _ := e.newVersion(t, f, "1.0.0", "")
	treatment, treatSHA, _ := e.newVersion(t, f, "2.0.0", "")
	labels := map[string]string{"suite": randHex(4)}
	var exp idOnly
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/experiments", map[string]any{"project_id": f.project, "name": "ab-" + randHex(3),
		"control_version_id": control, "treatment_version_id": treatment, "target_selector": labels, "min_sample_size": 20}, &exp))
	require.Equal(t, 200, e.call(t, "POST", "/api/v1/experiments/"+exp.ID+"/start", nil, nil))

	arms := map[string]*device{}
	armIDs := map[string]string{}
	for i := 0; i < 30 && len(arms) < 2; i++ {
		d := e.register(t, f.enrollKey, labels)
		resp := e.heartbeat(t, d)
		require.Len(t, resp.Experiments, 1)
		arm := resp.Experiments[0].ArmName
		if arms[arm] == nil {
			arms[arm], armIDs[arm] = d, resp.Experiments[0].ArmId
		}
	}
	require.Len(t, arms, 2)

	for arm, d := range arms {
		a := d.loaded[f.model]
		require.NotNil(t, a, arm)
		wantVersion, wantSHA := control, ctrlSHA
		if arm == "treatment" {
			wantVersion, wantSHA = treatment, treatSHA
		}
		assert.Equal(t, wantVersion, a.VersionId, arm)
		assert.Equal(t, wantSHA, fetchSHA(t, a.DownloadUrl), arm)
		resp := e.heartbeat(t, d)
		for _, x := range resp.Assignments {
			assert.NotEqual(t, fleetmlv1.ModelAssignment_UNLOAD, x.Action, "the arm model is kept")
		}
		e.report(t, d, a.VersionId, armIDs[arm], 25, fleetmlv1.TelemetryEvent_INFERENCE, 20)
	}

	var res struct {
		Control struct {
			SampleSize int64 `json:"sample_size"`
		} `json:"control"`
		Treatment struct {
			SampleSize int64 `json:"sample_size"`
		} `json:"treatment"`
		Final bool `json:"final"`
	}
	waitFor(t, 20*time.Second, "results for both arms", func() bool {
		e.call(t, "GET", "/api/v1/experiments/"+exp.ID+"/results", nil, &res)
		return res.Control.SampleSize == 25 && res.Treatment.SampleSize == 25
	})
	assert.True(t, res.Final, "both arms reached min_sample_size")
}

// Deployments and experiments reach only devices of their own project, even
// when another project's device has exactly the labels they target.
func TestE2E_ProjectIsolation(t *testing.T) {
	e := newEnv(t)
	a, b := e.newFixture(t), e.newFixture(t)
	labels := map[string]string{"suite": randHex(4)}
	va, _, _ := e.newVersion(t, a, "1.0.0", "")
	e.deploy(t, map[string]any{"project_id": a.project, "model_version_id": va, "target_selector": labels})
	control, _, _ := e.newVersion(t, a, "2.0.0", "")
	treatment, _, _ := e.newVersion(t, a, "3.0.0", "")
	var exp idOnly
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/experiments", map[string]any{"project_id": a.project, "name": "iso-" + randHex(3),
		"control_version_id": control, "treatment_version_id": treatment, "target_selector": labels, "min_sample_size": 20}, &exp))
	require.Equal(t, 200, e.call(t, "POST", "/api/v1/experiments/"+exp.ID+"/start", nil, nil))

	inA := e.register(t, a.enrollKey, labels)
	respA := e.heartbeat(t, inA)
	assert.NotEmpty(t, respA.Assignments, "project A's device gets project A's model")
	assert.Len(t, respA.Experiments, 1)

	inB := e.register(t, b.enrollKey, labels)
	respB := e.heartbeat(t, inB)
	assert.Empty(t, respB.Assignments, "project B's device must not see project A's deployment")
	assert.Empty(t, respB.Experiments, "project B's device must not join project A's experiment")
}

// A device cannot report telemetry against another project's model version.
func TestE2E_TelemetryStaysInItsProject(t *testing.T) {
	e := newEnv(t)
	a, b := e.newFixture(t), e.newFixture(t)
	va, _, _ := e.newVersion(t, a, "1.0.0", "")
	intruder := e.register(t, b.enrollKey, nil)
	ack, err := e.tel.ReportBatch(intruder.ctx(), &fleetmlv1.TelemetryBatch{Events: []*fleetmlv1.TelemetryEvent{
		{ModelVersionId: va, Type: fleetmlv1.TelemetryEvent_ERROR, ErrorMessage: "forged"},
	}})
	require.NoError(t, err)
	assert.Zero(t, ack.AcceptedCount)
	assert.Equal(t, uint32(1), ack.RejectedCount)

	owner := e.register(t, a.enrollKey, nil)
	ack, err = e.tel.ReportBatch(owner.ctx(), &fleetmlv1.TelemetryBatch{Events: []*fleetmlv1.TelemetryEvent{
		{ModelVersionId: va, Type: fleetmlv1.TelemetryEvent_INFERENCE, LatencyMs: 5},
	}})
	require.NoError(t, err)
	assert.Equal(t, uint32(1), ack.AcceptedCount)
}
