//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/alert"
)

type sinkView struct {
	ID      string            `json:"id"`
	Kind    string            `json:"kind"`
	Config  map[string]string `json:"config"`
	Enabled bool              `json:"enabled"`
}

type deliveryView struct {
	AlertID   string `json:"alert_id"`
	AlertType string `json:"alert_type"`
	Attempt   int    `json:"attempt"`
	Status    string `json:"status"`
	Error     string `json:"error"`
}

// recorder is a webhook or Slack endpoint that keeps what it receives.
type recorder struct {
	mu     sync.Mutex
	bodies [][]byte
	sigs   []string
	srv    *httptest.Server
}

func newRecorder(t *testing.T) *recorder {
	r := &recorder{}
	r.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies, r.sigs = append(r.bodies, b), append(r.sigs, req.Header.Get(alert.HeaderSignature))
		r.mu.Unlock()
	}))
	// A server in a cluster (scripts/helm-e2e.sh) reaches this machine by another name.
	host := os.Getenv("FLEETML_E2E_CALLBACK_HOST")
	if host != "" {
		l, err := net.Listen("tcp", "0.0.0.0:0")
		require.NoError(t, err)
		r.srv.Listener = l
	}
	r.srv.Start()
	if host != "" {
		r.srv.URL = "http://" + net.JoinHostPort(host, strconv.Itoa(r.srv.Listener.Addr().(*net.TCPAddr).Port))
	}
	t.Cleanup(r.srv.Close)
	return r
}

func (r *recorder) received() (bodies [][]byte, sigs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]byte(nil), r.bodies...), append([]string(nil), r.sigs...)
}

// V2.2: a rollback reaches the project's webhook and Slack sinks exactly once
// each; a sink that cannot be reached gets five attempts and a dead letter.
func TestE2E_AlertSinks(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	hook, slack := newRecorder(t), newRecorder(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closed := "http://" + l.Addr().String() + "/gone"
	require.NoError(t, l.Close())

	sinks := "/api/v1/projects/" + f.project + "/alert-sinks"
	var wh, sl, dead sinkView
	require.Equal(t, 201, e.call(t, "POST", sinks, map[string]any{"name": "hook", "kind": "webhook", "config": map[string]string{"url": hook.srv.URL, "secret": "e2e-secret"}}, &wh))
	require.Equal(t, 201, e.call(t, "POST", sinks, map[string]any{"name": "slack", "kind": "slack", "config": map[string]string{"webhook_url": slack.srv.URL + "/services/T0/B0/x"}}, &sl))
	require.Equal(t, 201, e.call(t, "POST", sinks, map[string]any{"name": "closed", "kind": "webhook", "config": map[string]string{"url": closed}}, &dead))
	assert.Equal(t, "********", wh.Config["secret"], "secrets are not returned")
	assert.NotContains(t, sl.Config["webhook_url"], "/services/", "the Slack URL path is a secret")
	assert.Equal(t, 400, e.call(t, "POST", sinks, map[string]any{"name": "x", "kind": "pager", "config": map[string]string{}}, nil))
	assert.Equal(t, 400, e.call(t, "POST", sinks, map[string]any{"name": "x", "kind": "webhook", "config": map[string]string{"url": "not a url"}}, nil))
	assert.Equal(t, 404, e.call(t, "POST", "/api/v1/projects/00000000-0000-0000-0000-000000000001/alert-sinks",
		map[string]any{"name": "x", "kind": "webhook", "config": map[string]string{"url": hook.srv.URL}}, nil))

	// Sending the redacted secret back keeps the stored one.
	var patched sinkView
	require.Equal(t, 200, e.call(t, "PATCH", "/api/v1/alert-sinks/"+wh.ID, map[string]any{"name": "on-call", "config": wh.Config}, &patched))
	var test deliveryView
	require.Equal(t, 200, e.call(t, "POST", "/api/v1/alert-sinks/"+wh.ID+"/test", nil, &test))
	assert.Equal(t, "ok", test.Status)
	bodies, sigs := hook.received()
	require.Len(t, bodies, 1)
	assert.Equal(t, alert.Sign([]byte("e2e-secret"), bodies[0]), sigs[0], "the secret survived the PATCH")

	v1, _, _ := e.newVersion(t, f, "1.0.0", "")
	v2, _, _ := e.newVersion(t, f, "2.0.0", "")
	labels := map[string]string{"suite": randHex(4)}
	e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v1, "target_selector": labels})
	d2 := e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v2, "target_selector": labels})
	require.Equal(t, 200, e.call(t, "POST", "/api/v1/deployments/"+d2+"/rollback", map[string]string{"reason": "e2e alert"}, nil))

	waitFor(t, 10*time.Second, "webhook and Slack alerts", func() bool {
		b1, _ := hook.received()
		b2, _ := slack.received()
		return len(b1) == 2 && len(b2) == 1
	})
	bodies, sigs = hook.received()
	var a alert.Alert
	require.NoError(t, json.Unmarshal(bodies[1], &a))
	assert.Equal(t, alert.TypeRollback, a.Type)
	assert.Equal(t, f.project, a.ProjectID)
	assert.Equal(t, d2, a.Rollback.DeploymentID)
	require.NotNil(t, a.Model)
	assert.Equal(t, "2.0.0", a.Model.Version)
	assert.Equal(t, alert.Sign([]byte("e2e-secret"), bodies[1]), sigs[1])
	sb, _ := slack.received()
	assert.Contains(t, string(sb[0]), `"blocks"`)
	assert.Contains(t, string(sb[0]), "Rollback: clf 2.0.0")
	assert.Contains(t, string(sb[0]), "e2e alert")

	// The closed port: attempts 1-4 are errors, the fifth is the dead letter (1+2+4+8s of backoff).
	var log []deliveryView
	waitFor(t, 30*time.Second, "dead letter", func() bool {
		e.call(t, "GET", "/api/v1/alert-sinks/"+dead.ID+"/deliveries", nil, &log)
		return len(log) > 0 && log[0].Status == "dead"
	})
	require.Len(t, log, 5)
	for i, d := range log {
		assert.Equal(t, 5-i, d.Attempt, "newest first")
		assert.Equal(t, a.ID, d.AlertID)
		assert.True(t, strings.Contains(d.Error, "connection refused"), d.Error)
	}
	time.Sleep(time.Second)
	b1, _ := hook.received()
	b2, _ := slack.received()
	assert.Len(t, b1, 2, "exactly once")
	assert.Len(t, b2, 1, "exactly once")

	// Disabled sinks get nothing; deleting removes the sink and its log.
	require.Equal(t, 200, e.call(t, "PATCH", "/api/v1/alert-sinks/"+sl.ID, map[string]any{"enabled": false}, nil))
	require.Equal(t, 204, e.call(t, "DELETE", "/api/v1/alert-sinks/"+dead.ID, nil, nil))
	assert.Equal(t, 404, e.call(t, "GET", "/api/v1/alert-sinks/"+dead.ID, nil, nil))
	var list []sinkView
	require.Equal(t, 200, e.call(t, "GET", sinks, nil, &list))
	require.Len(t, list, 2)
	assert.False(t, list[1].Enabled)
}
