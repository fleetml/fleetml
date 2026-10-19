package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureServer answers every request with {"data": {...}} and records the
// last request body per path.
func captureServer(t *testing.T) (url string, bodies map[string]map[string]any) {
	t.Helper()
	bodies = map[string]map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies[r.URL.Path] = body
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "x", "status": "CONCLUDED"}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL, bodies
}

// The tour's canary and A/B experiment run from the CLI: the stages, the
// experiment's target and sample size reach the API as the REST tour sent them.
func TestDeployAndExperimentFlags(t *testing.T) {
	url, bodies := captureServer(t)

	require.NoError(t, runCLI(t, url, "k", "deploy", "create", "--project", "p", "--version", "v", "--strategy", "CANARY",
		"--target", `{"app":"tour"}`,
		"--canary-stages", `[{"percentage":25,"min_duration":"10m"},{"percentage":100,"min_duration":"0s"}]`,
		"--health-gate", `{"max_error_rate":0.05,"min_events":30,"on_failure":"ROLLBACK"}`))
	d := bodies["/api/v1/deployments"]
	assert.Equal(t, "CANARY", d["strategy"])
	assert.Equal(t, map[string]any{"app": "tour"}, d["target_selector"])
	assert.Equal(t, []any{
		map[string]any{"percentage": float64(25), "min_duration": "10m"},
		map[string]any{"percentage": float64(100), "min_duration": "0s"},
	}, d["canary_stages"])
	assert.Equal(t, 0.05, d["health_gate"].(map[string]any)["max_error_rate"])
	assert.NotContains(t, d, "rolling_config")

	require.NoError(t, runCLI(t, url, "k", "deploy", "create", "--project", "p", "--version", "v", "--strategy", "ROLLING",
		"--rolling-config", `{"batch_size_percent":20,"batch_interval":"1m"}`))
	assert.Equal(t, map[string]any{"batch_size_percent": float64(20), "batch_interval": "1m"}, bodies["/api/v1/deployments"]["rolling_config"])
	assert.ErrorContains(t, runCLI(t, url, "k", "deploy", "create", "--project", "p", "--version", "v", "--canary-stages", "25"),
		"invalid --canary-stages JSON")

	require.NoError(t, runCLI(t, url, "k", "experiment", "create", "--project", "p", "--name", "n", "--control", "a", "--treatment", "b",
		"--target", `{"app":"tour"}`, "--min-sample-size", "300"))
	e := bodies["/api/v1/experiments"]
	assert.Equal(t, map[string]any{"app": "tour"}, e["target_selector"])
	assert.Equal(t, float64(300), e["min_sample_size"])

	require.NoError(t, runCLI(t, url, "k", "experiment", "create", "--project", "p", "--name", "n", "--control", "a", "--treatment", "b"))
	assert.NotContains(t, bodies["/api/v1/experiments"], "min_sample_size", "0 leaves the server's default")
	assert.NotContains(t, bodies["/api/v1/experiments"], "target_selector")

	// A short winner ID no longer panics.
	require.NoError(t, runCLI(t, url, "k", "experiment", "conclude", "x", "--winner", "v2"))
	assert.Equal(t, "v2", bodies["/api/v1/experiments/x/conclude"]["winner_version_id"])
}
