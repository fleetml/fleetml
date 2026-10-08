//go:build e2e

// Package e2e drives a running FleetML server over REST and gRPC the way the
// CLI, dashboard and SDKs do. Run it with `make e2e`, which starts a server
// with fast background loops against a throwaway database.
package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

type env struct {
	api, key string
	devices  fleetmlv1.DeviceServiceClient
	tel      fleetmlv1.TelemetryServiceClient
}

func newEnv(t *testing.T) *env {
	t.Helper()
	key := os.Getenv("FLEETML_E2E_ADMIN_KEY")
	if key == "" {
		t.Skip("FLEETML_E2E_ADMIN_KEY not set; run `make e2e`")
	}
	api := getenv("FLEETML_E2E_API", "http://localhost:18080")
	conn, err := grpc.NewClient(getenv("FLEETML_E2E_GRPC", "localhost:19090"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return &env{api: api, key: key, devices: fleetmlv1.NewDeviceServiceClient(conn), tel: fleetmlv1.NewTelemetryServiceClient(conn)}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// call sends a REST request and decodes {"data": ...} into out; it returns the status code.
func (e *env) call(t *testing.T, method, path string, body, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.api+path, rd)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(raw, &env), string(raw))
		require.NoError(t, json.Unmarshal(env.Data, out), string(raw))
	}
	return resp.StatusCode
}

type idOnly struct {
	ID string `json:"id"`
}

func randomBlob(n int) []byte {
	blob := make([]byte, n)
	_, _ = rand.Read(blob)
	return blob
}

type fixture struct {
	project, model, enrollKey string
}

func (e *env) newFixture(t *testing.T) fixture {
	t.Helper()
	var p, m idOnly
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/projects", map[string]string{"name": "e2e-" + randHex(4)}, &p))
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/projects/"+p.ID+"/models",
		map[string]string{"name": "clf", "framework": "TFLITE", "task_type": "CLASSIFICATION"}, &m))
	var k struct {
		Key string `json:"key"`
	}
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/projects/"+p.ID+"/enrollment-keys", map[string]string{"name": "e2e"}, &k))
	return fixture{project: p.ID, model: m.ID, enrollKey: k.Key}
}

// newVersion creates a version and uploads a variant; declaredSHA overrides the real hash.
// It returns the version ID, the blob's real sha256 and the finalize status code.
func (e *env) newVersion(t *testing.T, f fixture, version, declaredSHA string) (versionID, sha string, finalize int) {
	t.Helper()
	blob := randomBlob(4096)
	return e.newVersionOf(t, f, version, blob, declaredSHA)
}

// newVersionOf is newVersion with the given model file.
func (e *env) newVersionOf(t *testing.T, f fixture, version string, blob []byte, declaredSHA string) (versionID, sha string, finalize int) {
	t.Helper()
	var v idOnly
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/models/"+f.model+"/versions", map[string]string{"version": version}, &v))
	sum := sha256.Sum256(blob)
	sha = hex.EncodeToString(sum[:])
	if declaredSHA == "" {
		declaredSHA = sha
	}
	var created struct {
		Variant   idOnly `json:"variant"`
		UploadURL string `json:"upload_url"`
	}
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/versions/"+v.ID+"/variants", map[string]any{
		"quantization": "INT8", "target_runtime": "TFLITE", "blob_size_bytes": len(blob), "blob_sha256": declaredSHA,
	}, &created))
	req, err := http.NewRequest("PUT", created.UploadURL, bytes.NewReader(blob))
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Less(t, resp.StatusCode, 300)
	return v.ID, sha, e.call(t, "POST", "/api/v1/variants/"+created.Variant.ID+"/finalize", nil, nil)
}

func (e *env) deploy(t *testing.T, body map[string]any) string {
	t.Helper()
	var d idOnly
	require.Equal(t, 201, e.call(t, "POST", "/api/v1/deployments", body, &d))
	require.Equal(t, 200, e.call(t, "POST", "/api/v1/deployments/"+d.ID+"/start", nil, nil))
	return d.ID
}

func (e *env) deploymentStatus(t *testing.T, id string) string {
	t.Helper()
	var d struct {
		Status string `json:"status"`
	}
	require.Equal(t, 200, e.call(t, "GET", "/api/v1/deployments/"+id, nil, &d))
	return d.Status
}

// device is a simulated SDK client.
type device struct {
	id, token string
	loaded    map[string]*fleetmlv1.ModelAssignment // model ID -> loaded assignment
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (e *env) register(t *testing.T, enrollKey string, labels map[string]string) *device {
	t.Helper()
	resp, err := e.devices.Register(context.Background(), &fleetmlv1.RegisterRequest{
		Fingerprint: randHex(32), Os: "ANDROID", RamMb: 4096, EnrollmentKey: enrollKey, Labels: labels,
	})
	require.NoError(t, err)
	return &device{id: resp.DeviceId, token: resp.AuthToken, loaded: map[string]*fleetmlv1.ModelAssignment{}}
}

func (d *device) ctx() context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+d.token)
}

// heartbeat reports loaded models, applies LOAD/UPDATE/UNLOAD like the SDK and returns the response.
func (e *env) heartbeat(t *testing.T, d *device) *fleetmlv1.HeartbeatResponse {
	t.Helper()
	req := &fleetmlv1.HeartbeatRequest{DeviceId: d.id}
	for _, a := range d.loaded {
		req.LoadedModels = append(req.LoadedModels, &fleetmlv1.ModelStatus{ModelId: a.ModelId, Version: a.Version, VariantId: a.VariantId, Status: "LOADED"})
	}
	resp, err := e.devices.Heartbeat(d.ctx(), req)
	require.NoError(t, err)
	for _, a := range resp.Assignments {
		switch a.Action {
		case fleetmlv1.ModelAssignment_LOAD, fleetmlv1.ModelAssignment_UPDATE:
			d.loaded[a.ModelId] = a
		case fleetmlv1.ModelAssignment_UNLOAD:
			if cur, ok := d.loaded[a.ModelId]; ok && cur.VariantId == a.VariantId {
				delete(d.loaded, a.ModelId)
			}
		}
	}
	return resp
}

func (e *env) report(t *testing.T, d *device, versionID, armID string, n int, typ fleetmlv1.TelemetryEvent_EventType, latency uint32) {
	t.Helper()
	events := make([]*fleetmlv1.TelemetryEvent, n)
	for i := range events {
		events[i] = &fleetmlv1.TelemetryEvent{ModelVersionId: versionID, ExperimentArmId: armID, Type: typ, LatencyMs: latency, ConfidenceScore: 0.9}
	}
	ack, err := e.tel.ReportBatch(d.ctx(), &fleetmlv1.TelemetryBatch{DeviceId: d.id, Events: events})
	require.NoError(t, err)
	require.Equal(t, uint32(n), ack.AcceptedCount)
}

// waitFor polls cond until it is true or the timeout passes.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}

func fetchSHA(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec,noctx // presigned URL from the server under test
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, 200, resp.StatusCode)
	h := sha256.New()
	_, err = io.Copy(h, resp.Body)
	require.NoError(t, err)
	return fmt.Sprintf("%x", h.Sum(nil))
}
