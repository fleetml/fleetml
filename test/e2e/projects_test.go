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
