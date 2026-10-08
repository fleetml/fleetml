//go:build e2e

package e2e

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Probes, metrics and gRPC health are served without credentials.
func TestE2E_OpsEndpoints(t *testing.T) {
	e := newEnv(t)
	get := func(path string) (int, string) {
		resp, err := http.Get(e.api + path)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	code, _ := get("/livez")
	assert.Equal(t, 200, code)
	code, body := get("/readyz")
	assert.Equal(t, 200, code)
	assert.Contains(t, body, `"db":"connected"`)

	f := e.newFixture(t) // some API traffic to count
	d := e.register(t, f.enrollKey, nil)
	e.heartbeat(t, d)
	code, body = get("/metrics")
	assert.Equal(t, 200, code)
	for _, m := range []string{
		`fleetml_http_requests_total{method="POST",route="/api/v1/projects",status="201"}`,
		`fleetml_grpc_requests_total{code="OK",method="/fleetml.device.v1.DeviceService/Heartbeat"}`,
		`fleetml_ticker_runs_total{ticker="deploy"}`,
		"fleetml_telemetry_events_accepted_total",
		"go_goroutines",
	} {
		assert.True(t, strings.Contains(body, m), "missing %s", m)
	}

	conn, err := grpc.NewClient(getenv("FLEETML_E2E_GRPC", "localhost:19090"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	for _, svc := range []string{"", "fleetml.device.v1.DeviceService"} {
		resp, err := healthpb.NewHealthClient(conn).Check(context.Background(), &healthpb.HealthCheckRequest{Service: svc})
		require.NoError(t, err, svc)
		assert.Equal(t, healthpb.HealthCheckResponse_SERVING, resp.Status, svc)
	}
}
