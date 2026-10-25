package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/cli/clitest"
)

// stdout runs f and returns what it printed.
func stdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	saved := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = saved }()
	f()
	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

// push --promote reports a refused status change instead of printing
// "Pushed" as if the version were active.
func TestPush_PromoteFailureIsReported(t *testing.T) {
	fake := clitest.NewServer(t, "k")
	target, err := url.Parse(fake.URL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/status") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"code":"INVALID_TRANSITION","message":"invalid status transition: cannot transition from DRAFT to STAGED"}}`))
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(refusing.Close)

	file := filepath.Join(t.TempDir(), "m.tflite")
	require.NoError(t, os.WriteFile(file, []byte("TFL3 model"), 0o644))
	require.NoError(t, runCLI(t, refusing.URL, "k", "project", "create", "--name", "promote"))
	projectID := fake.Projects()[0]["id"].(string)

	var runErr error
	out := stdout(t, func() {
		runErr = runCLI(t, refusing.URL, "k", "push", file, "--project", projectID, "--model", "m", "--version", "1.0.0", "--promote")
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "but promoting it to STAGED failed")
	assert.Contains(t, runErr.Error(), "cannot transition from DRAFT to STAGED")
	assert.NotContains(t, out, "Pushed")
	assert.Equal(t, "READY", fake.Variants()[0]["status"], "the file itself was uploaded")
}

// simulate takes the enrollment key from FLEETML_ENROLLMENT_KEY too, which
// keeps it out of the process list.
func TestSimulate_KeyFromEnvironment(t *testing.T) {
	t.Setenv("FLEETML_ENROLLMENT_KEY", "")
	err := runCLI(t, "http://127.0.0.1:1", "k", "simulate", "--grpc", "127.0.0.1:1", "--devices", "1", "--duration", "1s")
	assert.ErrorContains(t, err, "--enrollment-key or FLEETML_ENROLLMENT_KEY is required")

	t.Setenv("FLEETML_ENROLLMENT_KEY", "ek-from-env")
	err = runCLI(t, "http://127.0.0.1:1", "k", "simulate", "--grpc", "127.0.0.1:1", "--devices", "1", "--duration", "1s")
	if err != nil {
		assert.NotContains(t, err.Error(), "is required", "the key was taken from the environment")
	}
}

// telemetry events shows when each event was recorded and for which version,
// looking each version up once.
func TestTelemetryEvents_TimeAndVersion(t *testing.T) {
	lookups := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/telemetry/events":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"recorded_at": "2026-10-25T10:00:01Z", "model_version_id": "11111111-aaaa-4aaa-8aaa-000000000001", "event_type": "INFERENCE", "latency_ms": 3, "confidence_score": 0.97, "error_message": ""},
				{"recorded_at": "2026-10-25T10:00:02Z", "model_version_id": "11111111-aaaa-4aaa-8aaa-000000000001", "event_type": "INFERENCE", "latency_ms": 4, "confidence_score": 0.9, "error_message": ""},
				{"recorded_at": "2026-10-25T10:00:03Z", "model_version_id": "22222222-bbbb-4bbb-8bbb-000000000002", "event_type": "MODEL_LOAD", "latency_ms": 0, "confidence_score": 0, "error_message": ""},
			}})
		case strings.HasPrefix(r.URL.Path, "/api/v1/versions/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/versions/")
			lookups[id]++
			if strings.HasPrefix(id, "2222") {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"model version not found"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": id, "version": "1.0.0"}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("TZ", "UTC")

	var err error
	out := stdout(t, func() { err = runCLI(t, srv.URL, "k", "telemetry", "events", "--device", "d1") })
	require.NoError(t, err)
	assert.Contains(t, out, "Time")
	assert.Contains(t, out, "Version")
	assert.Regexp(t, `2026-10-25 \d\d:00:01\s+1\.0\.0\s+INFERENCE`, out)
	assert.Regexp(t, `22222222\s+MODEL_LOAD`, out, "an unknown version shows its ID's start")
	assert.Regexp(t, `INFERENCE\s+3\s+0\.970`, out)
	assert.NotContains(t, out, "0.000", "no confidence is a blank, not a zero")
	assert.Equal(t, 1, lookups["11111111-aaaa-4aaa-8aaa-000000000001"], "one lookup per version")
}

// push --promote starts from the version's status: a second variant of an
// ACTIVE version changes nothing and succeeds, and a STAGED version only
// takes the last step.
func TestPush_PromoteFromCurrentStatus(t *testing.T) {
	fake := clitest.NewServer(t, "k")
	target, err := url.Parse(fake.URL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	var patches []string
	counting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/status") {
			var body map[string]string
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			patches = append(patches, body["status"])
			r.Body = io.NopCloser(strings.NewReader(string(raw)))
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(counting.Close)

	dir := t.TempDir()
	fileA, fileB := filepath.Join(dir, "a.tflite"), filepath.Join(dir, "b.tflite")
	require.NoError(t, os.WriteFile(fileA, []byte("TFL3 int8"), 0o644))
	require.NoError(t, os.WriteFile(fileB, []byte("TFL3 fp16"), 0o644))
	require.NoError(t, runCLI(t, counting.URL, "k", "project", "create", "--name", "promote"))
	projectID := fake.Projects()[0]["id"].(string)
	push := func(file, quant string) (string, error) {
		var runErr error
		out := stdout(t, func() {
			runErr = runCLI(t, counting.URL, "k", "push", file, "--project", projectID, "--model", "m", "--version", "1.0.0", "--quantization", quant, "--promote")
		})
		return out, runErr
	}

	out, err := push(fileA, "INT8")
	require.NoError(t, err)
	assert.Equal(t, []string{"STAGED", "ACTIVE"}, patches)
	assert.Contains(t, out, "v1.0.0 promoted to STAGED → ACTIVE", "a promotion says what it did")

	out, err = push(fileB, "FP16")
	require.NoError(t, err, "a second variant of an ACTIVE version")
	assert.Contains(t, out, "Pushed")
	assert.Contains(t, out, "v1.0.0 was already ACTIVE")
	assert.Equal(t, []string{"STAGED", "ACTIVE"}, patches, "no status change for an ACTIVE version")

	fake.Versions()[0]["status"] = "STAGED"
	patches = nil
	out, err = push(fileA, "INT4")
	require.NoError(t, err)
	assert.Equal(t, []string{"ACTIVE"}, patches, "STAGED takes the last step only")
	assert.Contains(t, out, "v1.0.0 promoted to ACTIVE")
}
