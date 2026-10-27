package deploy

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A partial health gate keeps the defaults of the keys it leaves out; an
// explicit 0 stays 0; a key the gate does not have is an error naming it.
func TestHealthGateJSON(t *testing.T) {
	var g HealthGate
	require.NoError(t, json.Unmarshal([]byte(`{"min_events": 30}`), &g))
	assert.Equal(t, HealthGate{MaxErrorRate: 0.05, MinEvents: 30, OnFailure: "ROLLBACK"}, g)

	require.NoError(t, json.Unmarshal([]byte(`{"max_error_rate": 0, "on_failure": "PAUSE"}`), &g))
	assert.Equal(t, HealthGate{MaxErrorRate: 0, MinEvents: 20, OnFailure: "PAUSE"}, g)

	err := json.Unmarshal([]byte(`{"max_error": 0.1, "min_events": 5}`), &g)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `health_gate: unknown key "max_error"`)

	// What the repository stored before (every key) reads back unchanged.
	b, _ := json.Marshal(HealthGate{MaxErrorRate: 0.2, MaxLatencyRegression: 0.1, MinEvents: 7, OnFailure: "PAUSE"})
	var back HealthGate
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, HealthGate{MaxErrorRate: 0.2, MaxLatencyRegression: 0.1, MinEvents: 7, OnFailure: "PAUSE"}, back)
}

func TestRollingConfigJSON(t *testing.T) {
	var c RollingConfig
	require.NoError(t, json.Unmarshal([]byte(`{"batch_size": 2, "batch_interval": "1m"}`), &c))
	assert.Equal(t, RollingConfig{BatchSize: 2, BatchInterval: "1m"}, c)
	err := json.Unmarshal([]byte(`{"batch_size": 2, "interval": "1m"}`), &c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `rolling_config: unknown key "interval"`)
}
