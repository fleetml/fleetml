package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The example config is what install.sh installs and docs/LINUX_AGENT.md asks
// the operator to edit; it must load, and its tls must match the quickstart's
// plain-gRPC compose server (and the documented default).
func TestExampleConfig(t *testing.T) {
	c, err := LoadConfig("../../deploy/agent/config.example.yaml")
	require.NoError(t, err)
	require.NoError(t, c.Validate())
	assert.False(t, c.TLS, "config.example.yaml: tls must be false, as for the quickstart's compose server")
	assert.Equal(t, "fleetml.example.com:19090", c.Server)
	assert.Equal(t, "<project enrollment key>", c.EnrollmentKey, "install.sh waits for this placeholder to be replaced")
	assert.Equal(t, "/var/lib/fleetml-agent/models", c.ModelsDir)

	d, err := LoadConfig("")
	require.NoError(t, err)
	assert.Equal(t, d.TLS, c.TLS, "the example's tls is the default")
}
