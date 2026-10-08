package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
