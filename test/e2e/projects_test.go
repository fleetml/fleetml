//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

// apiError sends a request and returns the status and the error body's code
// and message.
func (e *env) apiError(t *testing.T, method, path string) (status int, code, message string) {
	t.Helper()
	req, err := http.NewRequest(method, e.api+path, http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+e.key)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &body)
	return resp.StatusCode, body.Error.Code, body.Error.Message
}

// A project whose device sent telemetry is deleted with everything in it, and
// the telemetry summary counts only INFERENCE events in its latency.
func TestE2E_DeleteProjectWithTelemetry(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	version, _, finalize := e.newVersion(t, f, "1.0.0", "")
	require.Equal(t, 200, finalize)
	labels := map[string]string{"suite": randHex(4)}
	d := e.register(t, f.enrollKey, labels)
	deployment := e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": version, "target_selector": labels, "strategy": "ALL_AT_ONCE"})
	e.heartbeat(t, d)
	e.report(t, d, version, "", 5, fleetmlv1.TelemetryEvent_INFERENCE, 10)
	e.report(t, d, version, "", 1, fleetmlv1.TelemetryEvent_MODEL_LOAD, 0)

	var sum struct {
		TotalEvents int64   `json:"total_events"`
		AvgLatency  float64 `json:"avg_latency_ms"`
		MinLatency  float64 `json:"min_latency_ms"`
	}
	waitFor(t, 20*time.Second, "the six events in the summary", func() bool {
		require.Equal(t, 200, e.call(t, "GET", "/api/v1/telemetry/summary/"+version, nil, &sum))
		return sum.TotalEvents == 6
	})
	assert.InDelta(t, 10, sum.AvgLatency, 0.01, "the MODEL_LOAD event is not in the latency")
	assert.InDelta(t, 10, sum.MinLatency, 0.01)

	require.Equal(t, 204, e.call(t, "DELETE", "/api/v1/projects/"+f.project, nil, nil))

	status, code, message := e.apiError(t, "GET", "/api/v1/projects/"+f.project)
	assert.Equal(t, 404, status)
	assert.Equal(t, "NOT_FOUND", code)
	assert.Equal(t, "project not found", message, "no internal function name")
	status, _, message = e.apiError(t, "GET", "/api/v1/deployments/"+deployment)
	assert.Equal(t, 404, status)
	assert.Equal(t, "deployment not found", message)
	status, _, _ = e.apiError(t, "GET", "/api/v1/devices/"+d.id)
	assert.Equal(t, 404, status, "the project's device is deleted")
	status, code, message = e.apiError(t, "GET", "/api/v1/telemetry/summary/"+version)
	assert.Equal(t, 404, status, "an unknown version is NOT_FOUND, not an empty summary")
	assert.Equal(t, "NOT_FOUND", code)
	assert.Equal(t, "model version not found", message)
	status, _, _ = e.apiError(t, "GET", "/api/v1/drift/"+version+"/history")
	assert.Equal(t, 404, status)
	status, _, _ = e.apiError(t, "DELETE", "/api/v1/projects/"+f.project)
	assert.Equal(t, 404, status, "a second delete")
}

// Status changes a resource's status does not allow answer 409
// INVALID_TRANSITION for deployments, experiments and versions alike; paging
// values that are not integers or out of range are refused; can_progress
// waits for the stage's minimum time; an unknown status is invalid input.
func TestE2E_StatusAndPagingErrors(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	v1, _, _ := e.newVersion(t, f, "1.0.0", "")
	v2, _, _ := e.newVersion(t, f, "2.0.0", "")

	// A version cannot go back to DRAFT once ACTIVE.
	require.Equal(t, 200, e.call(t, "PATCH", "/api/v1/versions/"+v1+"/status", map[string]string{"status": "STAGED"}, nil))
	require.Equal(t, 200, e.call(t, "PATCH", "/api/v1/versions/"+v1+"/status", map[string]string{"status": "ACTIVE"}, nil))
	assert.Equal(t, 409, e.call(t, "PATCH", "/api/v1/versions/"+v1+"/status", map[string]string{"status": "DRAFT"}, nil))

	// An experiment cannot be started twice.
	var exp idOnly
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/experiments", map[string]any{"project_id": f.project, "name": "twice-" + randHex(3),
		"control_version_id": v1, "treatment_version_id": v2}, &exp))
	require.Equal(t, 200, e.call(t, "POST", "/api/v1/experiments/"+exp.ID+"/start", nil, nil))
	status, code, message := e.apiError(t, "POST", "/api/v1/experiments/"+exp.ID+"/start")
	assert.Equal(t, 409, status)
	assert.Equal(t, "INVALID_TRANSITION", code)
	assert.Contains(t, message, "invalid status transition")

	// A canary in its first stage's hour: healthy, but not ready to progress.
	d := e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v2, "strategy": "CANARY",
		"canary_stages": []map[string]any{{"percentage": 50, "min_duration": "1h"}, {"percentage": 100}}})
	require.Equal(t, "ROLLING_OUT", e.deploymentStatus(t, d), "deploy starts it")
	var health struct {
		CanProgress bool   `json:"can_progress"`
		Reason      string `json:"reason"`
	}
	require.Equal(t, 200, e.call(t, "GET", "/api/v1/deployments/"+d+"/health", nil, &health))
	assert.False(t, health.CanProgress)
	assert.Contains(t, health.Reason, "stage 0 holds for its minimum time")

	// A deployment that is not ROLLING_OUT does not progress.
	require.Equal(t, 200, e.call(t, "POST", "/api/v1/deployments/"+d+"/rollback", map[string]string{"reason": "e2e"}, nil))
	status, code, _ = e.apiError(t, "POST", "/api/v1/deployments/"+d+"/progress")
	assert.Equal(t, 409, status)
	assert.Equal(t, "INVALID_TRANSITION", code)

	status, code, message = e.apiError(t, "GET", "/api/v1/deployments?project_id="+f.project+"&limit="+f.project)
	assert.Equal(t, 400, status)
	assert.Equal(t, "VALIDATION_ERROR", code)
	assert.Equal(t, `limit must be an integer, got "`+f.project+`"`, message)

	// A page larger than the server's is refused with the cap, not clamped.
	status, code, message = e.apiError(t, "GET", "/api/v1/devices?project_id="+f.project+"&limit=101")
	assert.Equal(t, 400, status)
	assert.Equal(t, "VALIDATION_ERROR", code)
	assert.Equal(t, "limit must be between 1 and 100, got 101", message)
	status, _, message = e.apiError(t, "GET", "/api/v1/telemetry/events?limit=1001")
	assert.Equal(t, 400, status)
	assert.Equal(t, "limit must be between 1 and 1000, got 1001", message)

	// An unknown status is invalid input, not a refused transition.
	assert.Equal(t, 400, e.call(t, "PATCH", "/api/v1/versions/"+v2+"/status", map[string]string{"status": "LIVE"}, nil))
}
