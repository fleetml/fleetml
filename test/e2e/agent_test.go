//go:build e2e

package e2e

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/agent"
)

// An agent whose server hands out download URLs for an address the agent
// cannot use gets its model through download_host_override: the object store
// checks the presigned signature against the Host header, which the agent
// keeps. Sending the request with the override as its Host is refused.
func TestE2E_AgentDownloadHostOverride(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	v1, sha, _ := e.newVersion(t, f, "1.0.0", "")
	labels := map[string]string{"suite": randHex(4)}
	e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v1, "target_selector": labels})

	probe := e.register(t, f.enrollKey, labels)
	as := e.heartbeat(t, probe).Assignments
	require.Len(t, as, 1)
	u, err := url.Parse(as[0].DownloadUrl)
	require.NoError(t, err)
	_, port, err := net.SplitHostPort(u.Host)
	require.NoError(t, err)
	other := net.JoinHostPort("127.0.0.1", port) // the same store under another name
	require.NotEqual(t, u.Host, other)

	rewritten := *u
	rewritten.Host = other
	resp, err := http.Get(rewritten.String()) //nolint:gosec,noctx // presigned URL from the server under test
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "a plain host rewrite breaks the signature")

	grpcAddr := getenv("FLEETML_E2E_GRPC", "localhost:19090")
	cfg := &agent.Config{Server: grpcAddr, EnrollmentKey: f.enrollKey, Labels: labels, StateDir: t.TempDir(), DownloadHostOverride: other}
	a, err := agent.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer func() { _ = a.Close() }()
	require.True(t, a.Sync(context.Background()))
	models := a.ActiveModels()
	require.Len(t, models, 1, "the model arrived through the override")
	assert.Equal(t, sha, models[0].Active.SHA256)
	_, err = os.Stat(models[0].Active.Dir + "/model.bin")
	assert.NoError(t, err)
}
