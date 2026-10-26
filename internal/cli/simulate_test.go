package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

func TestParseProfiles(t *testing.T) {
	m, err := parseFloats([]string{"2.0.0=0.3", "2.1.0=14"})
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"2.0.0": 0.3, "2.1.0": 14}, m)
	_, err = parseFloats([]string{"2.0.0=lots"})
	assert.Error(t, err)
	_, err = parsePairs([]string{"novalue"})
	assert.Error(t, err)
}

func TestSimulatedEvents(t *testing.T) {
	d := &simDevice{rng: rand.New(rand.NewSource(1)), arms: map[string]string{"v2": "arm-1"}}
	a := &fleetmlv1.ModelAssignment{Version: "2.0.0", VersionId: "v2"}
	c := simConfig{latency: map[string]float64{"2.0.0": 100}, errorRate: map[string]float64{"2.0.0": 1}}
	e := d.event(c, a, time.Now())
	assert.Equal(t, fleetmlv1.TelemetryEvent_ERROR, e.Type, "an error rate of 1 fails every inference")
	assert.Equal(t, "arm-1", e.ExperimentArmId)
	assert.InDelta(t, 100, e.LatencyMs, 60)

	e = d.event(simConfig{}, &fleetmlv1.ModelAssignment{Version: "1.0.0", VersionId: "v1"}, time.Now())
	assert.Equal(t, fleetmlv1.TelemetryEvent_INFERENCE, e.Type)
	assert.Empty(t, e.ExperimentArmId)
}

func TestFleetStatus(t *testing.T) {
	a := func(v string) map[string]*fleetmlv1.ModelAssignment {
		return map[string]*fleetmlv1.ModelAssignment{"m": {ModelName: "iris", Version: v}}
	}
	devs := []*simDevice{{loaded: a("1.0.0")}, {loaded: a("1.0.0")}, {loaded: a("2.0.0")}}
	assert.Equal(t, "iris 1.0.0 on 2, iris 2.0.0 on 1", fleetStatus(devs))
	assert.Equal(t, "no models assigned yet", fleetStatus([]*simDevice{{loaded: map[string]*fleetmlv1.ModelAssignment{}}}))
}

func TestDownloadVerifiesSHA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("model")) }))
	defer srv.Close()
	sum := sha256.Sum256([]byte("model"))
	assert.NoError(t, download(context.Background(), srv.URL, hex.EncodeToString(sum[:])))
	assert.ErrorContains(t, download(context.Background(), srv.URL, "00"), "sha256 mismatch")
}

// fakeDevices refuses the first heartbeat of each token it is told to and
// counts registrations; the embedded interface leaves the other RPCs out.
type fakeDevices struct {
	fleetmlv1.DeviceServiceClient
	registered int
	refuse     string
}

func (f *fakeDevices) Register(_ context.Context, _ *fleetmlv1.RegisterRequest, _ ...grpc.CallOption) (*fleetmlv1.RegisterResponse, error) {
	f.registered++
	return &fleetmlv1.RegisterResponse{DeviceId: "dev-1", AuthToken: fmt.Sprintf("token-%d", f.registered)}, nil
}

func (f *fakeDevices) Heartbeat(ctx context.Context, _ *fleetmlv1.HeartbeatRequest, _ ...grpc.CallOption) (*fleetmlv1.HeartbeatResponse, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	if got := md.Get("authorization"); len(got) == 1 && got[0] == "Bearer "+f.refuse {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	return &fleetmlv1.HeartbeatResponse{}, nil
}

// After a JWT secret rotation the server refuses every device token: each
// simulated device registers again with the enrollment key, says so once,
// and checks in with its new token on the next tick.
func TestRefusedTokenRegistersAgain(t *testing.T) {
	devs := &fakeDevices{}
	d := &simDevice{n: 3, fp: "fp", loaded: map[string]*fleetmlv1.ModelAssignment{}, arms: map[string]string{}, seen: map[string]bool{}, rng: rand.New(rand.NewSource(1))}
	require.NoError(t, d.register(context.Background(), simConfig{}, devs))
	assert.Equal(t, "token-1", d.token)

	devs.refuse = "token-1"
	var out bytes.Buffer
	d.tick(context.Background(), simConfig{}, devs, nil, &out)
	assert.Equal(t, 2, devs.registered)
	assert.Equal(t, "token-2", d.token)
	assert.Equal(t, "simulator: device 3: token refused, registered again\n", out.String())

	out.Reset()
	d.tick(context.Background(), simConfig{}, devs, nil, &out)
	assert.Equal(t, 2, devs.registered, "the new token is accepted: no further registration")
	assert.Empty(t, out.String())
}
