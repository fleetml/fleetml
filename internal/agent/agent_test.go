package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/artifact"
	"github.com/fleetml/fleetml/internal/delta"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func shaHex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestQueuePersistsAndBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q")
	q, err := OpenQueue(path)
	require.NoError(t, err)
	for _, v := range []string{"a", "b", "c"} {
		require.NoError(t, q.Add(&fleetmlv1.TelemetryEvent{ModelVersionId: v}))
	}
	require.NoError(t, q.Remove(1))
	q2, err := OpenQueue(path)
	require.NoError(t, err)
	got := q2.Peek(10)
	require.Len(t, got, 2)
	assert.Equal(t, "b", got[0].ModelVersionId)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data[:len(data)-2], 0o600)) // torn last record
	q3, err := OpenQueue(path)
	require.NoError(t, err)
	assert.Equal(t, 1, q3.Len())
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := OpenStore(path)
	require.NoError(t, err)
	require.NoError(t, st.Update(func(s *State) {
		s.Salt, s.DeviceID = "salt", "dev-1"
		s.Models["m"] = &ModelState{Name: "iris", Active: &Version{Version: "1.0.0", VariantID: "v1"}}
	}))
	st2, err := OpenStore(path)
	require.NoError(t, err)
	st2.Read(func(s *State) {
		assert.Equal(t, "dev-1", s.DeviceID)
		assert.Equal(t, "1.0.0", s.Models["m"].Active.Version)
	})
}

func TestFingerprint(t *testing.T) {
	a, b := Fingerprint("machine", "salt-1"), Fingerprint("machine", "salt-2")
	assert.Len(t, a, 64)
	assert.Equal(t, a, Fingerprint("machine", "salt-1"), "stable")
	assert.NotEqual(t, a, b, "two installs on one machine differ")
	assert.NotContains(t, a, "machine")
}

func TestPolicy(t *testing.T) {
	def := fleetmlv1.DownloadPolicy_DOWNLOAD_POLICY_UNSPECIFIED
	assert.True(t, MayDownload(def, false, false, true), "unmetered")
	assert.True(t, MayDownload(def, false, true, false), "metered but on mains power")
	assert.False(t, MayDownload(def, false, true, true), "metered on battery waits")
	assert.True(t, MayDownload(def, true, true, true), "urgent")
	assert.False(t, MayDownload(fleetmlv1.DownloadPolicy_UNMETERED_ONLY, false, true, false))
	now := time.Now()
	assert.True(t, refreshDue(now, now.Add(-13*time.Hour), now.Add(11*time.Hour)), "half of a 24h token")
	assert.False(t, refreshDue(now, now, now.Add(30*24*time.Hour)))
}

// rangeServer serves body, honouring "Range: bytes=N-".
func rangeServer(body []byte, failFirst bool) *httptest.Server {
	failed := false
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := 0
		if rg := r.Header.Get("Range"); strings.HasPrefix(rg, "bytes=") {
			_, _ = sscan(strings.TrimSuffix(strings.TrimPrefix(rg, "bytes="), "-"), &start)
			w.WriteHeader(http.StatusPartialContent)
		}
		if failFirst && !failed {
			failed = true
			w.Header().Set("Content-Length", "1000000")
			_, _ = w.Write(body[:len(body)/2])
			return // the client sees a short body
		}
		_, _ = w.Write(body[start:])
	}))
}

func sscan(s string, n *int) (int, error) {
	v := 0
	for _, c := range s {
		v = v*10 + int(c-'0')
	}
	*n = v
	return 1, nil
}

func TestDownloadResumes(t *testing.T) {
	body := bytes.Repeat([]byte("fleetml-model-"), 5000)
	srv := rangeServer(body, true)
	defer srv.Close()
	part := filepath.Join(t.TempDir(), "x.part")
	err := download(context.Background(), srv.Client(), srv.URL, part, shaHex(body), "")
	require.Error(t, err, "the first attempt is cut short")
	fi, _ := os.Stat(part)
	require.Equal(t, int64(len(body)/2), fi.Size(), "the partial file is kept")
	require.NoError(t, download(context.Background(), srv.Client(), srv.URL, part, shaHex(body), ""))
	got, _ := os.ReadFile(part)
	assert.Equal(t, body, got)
	assert.Error(t, download(context.Background(), srv.Client(), srv.URL, filepath.Join(t.TempDir(), "y.part"), strings.Repeat("0", 64), ""))
}

func TestCommandValidator(t *testing.T) {
	f := filepath.Join(t.TempDir(), "it's a model.bin")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	assert.NoError(t, runCommand(context.Background(), "test -s {file}", f, nil), "{file} is quoted")
	err := runCommand(context.Background(), `echo "bad weights" >&2; exit 3`, f, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bad weights")
	assert.NoError(t, runCommand(context.Background(), `test "$FLEETML_MODEL_NAME" = iris`, f, map[string]string{"FLEETML_MODEL_NAME": "iris"}))
}

// The install path: download, validate, symlink, last-known-good, hook.
func TestInstallActivatesAndKeepsLastKnownGood(t *testing.T) {
	v1, v2, bad := []byte("model one"), []byte("model two"), []byte("broken")
	files := map[string][]byte{"/v1": v1, "/v2": v2, "/bad": bad}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(files[r.URL.Path]) }))
	defer srv.Close()
	dir := t.TempDir()
	marker := filepath.Join(dir, "hook.log")
	cfg := &Config{Server: "127.0.0.1:1", StateDir: dir,
		Validator:    `! grep -q broken {file}`,
		PostActivate: `echo "$FLEETML_MODEL_VERSION $FLEETML_MODEL_PATH" >> ` + marker}
	a, err := New(cfg, quiet())
	require.NoError(t, err)
	defer func() { _ = a.Close() }()
	assign := func(action fleetmlv1.ModelAssignment_Action, version, path string, body []byte) *fleetmlv1.ModelAssignment {
		return &fleetmlv1.ModelAssignment{ModelId: "m1", ModelName: "iris", Version: version, VersionId: "ver-" + version,
			VariantId: "var-" + version, DownloadUrl: srv.URL + path, BlobSha256: shaHex(body), Action: action}
	}
	ctx := context.Background()
	current := filepath.Join(dir, "models", "iris", "current")

	a.apply(ctx, []*fleetmlv1.ModelAssignment{assign(fleetmlv1.ModelAssignment_LOAD, "1.0.0", "/v1", v1)})
	got, err := os.ReadFile(filepath.Join(current, "model.bin"))
	require.NoError(t, err)
	assert.Equal(t, v1, got)

	// A broken 2.0.0 fails validation: 1.0.0 stays, MODEL_LOAD_FAILED is queued, no retry for an hour.
	a.apply(ctx, []*fleetmlv1.ModelAssignment{assign(fleetmlv1.ModelAssignment_UPDATE, "2.0.0", "/bad", bad),
		{ModelId: "m1", VariantId: "var-1.0.0", Action: fleetmlv1.ModelAssignment_UNLOAD}})
	got, _ = os.ReadFile(filepath.Join(current, "model.bin"))
	assert.Equal(t, v1, got, "last-known-good stays active")
	events := a.queue.Peek(10)
	assert.Equal(t, fleetmlv1.TelemetryEvent_MODEL_LOAD_FAILED, events[len(events)-1].Type)
	_, statErr := os.Stat(filepath.Join(dir, "models", "iris", "2.0.0-var-2.0."))
	assert.True(t, os.IsNotExist(statErr), "the rejected version is deleted")

	// A good 3.0.0 replaces it; 1.0.0 becomes last-known-good and stays on disk.
	a.apply(ctx, []*fleetmlv1.ModelAssignment{assign(fleetmlv1.ModelAssignment_UPDATE, "3.0.0", "/v2", v2)})
	got, _ = os.ReadFile(filepath.Join(current, "model.bin"))
	assert.Equal(t, v2, got)
	m := a.ActiveModels()[0]
	assert.Equal(t, "3.0.0", m.Active.Version)
	assert.Equal(t, "1.0.0", m.LastKnownGood.Version)
	_, err = os.Stat(filepath.Join(m.LastKnownGood.Dir, "model.bin"))
	assert.NoError(t, err)
	hook, _ := os.ReadFile(marker)
	assert.Equal(t, []string{"1.0.0 " + current, "3.0.0 " + current}, strings.Split(strings.TrimSpace(string(hook)), "\n"), "the hook ran per activation")

	// The local endpoint attributes inference events to the active version.
	rec := httptest.NewRecorder()
	a.handleEvents(rec, httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(`[{"model":"iris","latency_ms":7},{"model":"other"}]`)))
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.JSONEq(t, `{"accepted":1,"rejected":1}`, rec.Body.String())
	last := a.queue.Peek(100)
	assert.Equal(t, "ver-3.0.0", last[len(last)-1].ModelVersionId)
	rec = httptest.NewRecorder()
	a.handleModels(rec, httptest.NewRequest(http.MethodGet, "/v1/models", http.NoBody))
	assert.JSONEq(t, `[{"model":"iris","version":"3.0.0","version_id":"ver-3.0.0","path":"`+current+`"}]`, rec.Body.String(),
		"snake_case keys, like /v1/events")

	// UNLOAD without a replacement removes the model.
	a.apply(ctx, []*fleetmlv1.ModelAssignment{{ModelId: "m1", VariantId: "var-3.0.0", Action: fleetmlv1.ModelAssignment_UNLOAD}})
	assert.Empty(t, a.ActiveModels())
	_, err = os.Stat(current)
	assert.True(t, os.IsNotExist(err))
}

// A model whose signature does not verify is not downloaded or activated,
// and SIGNATURE_INVALID is reported.
func TestInstallRejectsBadSignature(t *testing.T) {
	body := []byte("signed model")
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { downloads++; _, _ = w.Write(body) }))
	defer srv.Close()
	dir := t.TempDir()
	a, err := New(&Config{Server: "127.0.0.1:1", StateDir: dir}, quiet())
	require.NoError(t, err)
	defer func() { _ = a.Close() }()

	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub := priv.Public().(ed25519.PublicKey)
	require.NoError(t, a.store.Update(func(s *State) {
		s.SigningKeys = []artifact.Key{{KeyID: artifact.KeyID(pub), PublicKey: pub}}
	}))
	as := &fleetmlv1.ModelAssignment{ModelId: "m1", ModelName: "iris", Version: "1.0.0", VersionId: "ver-1", VariantId: "var-1",
		DownloadUrl: srv.URL, BlobSha256: shaHex(body), Action: fleetmlv1.ModelAssignment_LOAD, SigningKeyId: artifact.KeyID(pub)}
	as.Signature = ed25519.Sign(priv, artifact.Message("m1", "1.0.0", shaHex([]byte("another file"))))

	a.apply(context.Background(), []*fleetmlv1.ModelAssignment{as})
	assert.Zero(t, downloads, "nothing is downloaded")
	assert.Empty(t, a.ActiveModels())
	events := a.queue.Peek(10)
	require.NotEmpty(t, events)
	assert.Equal(t, fleetmlv1.TelemetryEvent_SIGNATURE_INVALID, events[len(events)-1].Type)

	// Correctly signed, it installs (after the failure cooldown).
	as.Signature = ed25519.Sign(priv, artifact.Message("m1", "1.0.0", shaHex(body)))
	require.NoError(t, a.store.Update(func(s *State) { delete(s.Failed, "var-1") }))
	a.apply(context.Background(), []*fleetmlv1.ModelAssignment{as})
	require.Len(t, a.ActiveModels(), 1)
	assert.Equal(t, 1, downloads)
}

// Feature values posted by the inference process are binned with the edges
// the server configured for the model; unconfigured features are dropped.
func TestLocalEventsBinFeatures(t *testing.T) {
	body := []byte("model")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	a, err := New(&Config{Server: "127.0.0.1:1", StateDir: t.TempDir()}, quiet())
	require.NoError(t, err)
	defer func() { _ = a.Close() }()
	a.apply(context.Background(), []*fleetmlv1.ModelAssignment{{ModelId: "m1", ModelName: "iris", Version: "1.0.0", VersionId: "ver-1",
		VariantId: "var-1", DownloadUrl: srv.URL, BlobSha256: shaHex(body), Action: fleetmlv1.ModelAssignment_LOAD}})
	a.applyConfig(&fleetmlv1.DeviceConfig{FeatureBins: []*fleetmlv1.FeatureBins{{ModelId: "m1", Feature: "petal_length", Edges: []float64{1, 2}}}})

	rec := httptest.NewRecorder()
	a.handleEvents(rec, httptest.NewRequest(http.MethodPost, "/v1/events",
		strings.NewReader(`{"model":"iris","latency_ms":3,"features":{"petal_length":[0.5,1.5,1.7,9],"unknown":[1]}}`)))
	require.Equal(t, http.StatusAccepted, rec.Code)
	events := a.queue.Peek(100)
	last := events[len(events)-1]
	require.Len(t, last.Features, 1)
	assert.Equal(t, "petal_length", last.Features[0].Name)
	assert.Equal(t, []uint32{1, 2, 1}, last.Features[0].Counts)
}

// An UPDATE with a delta from the active version is built from the delta,
// without downloading the full model; a delta that does not match falls back.
func TestInstallFromDelta(t *testing.T) {
	v1 := bytes.Repeat([]byte("weights-"), 20000)
	v2 := append(append([]byte(nil), v1[:150000]...), []byte("a retrained head")...)
	d, err := delta.Diff(v1, v2)
	require.NoError(t, err)
	served := map[string]int{}
	files := map[string][]byte{"/v1": v1, "/v2": v2, "/d": d, "/bad": []byte("FMLDLT01 not really")}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served[r.URL.Path]++
		_, _ = w.Write(files[r.URL.Path])
	}))
	defer srv.Close()
	a, err := New(&Config{Server: "127.0.0.1:1", StateDir: t.TempDir()}, quiet())
	require.NoError(t, err)
	defer func() { _ = a.Close() }()
	ctx := context.Background()
	a.apply(ctx, []*fleetmlv1.ModelAssignment{{ModelId: "m1", ModelName: "iris", Version: "1.0.0", VersionId: "ver-1",
		VariantId: "var-1", DownloadUrl: srv.URL + "/v1", BlobSha256: shaHex(v1), Action: fleetmlv1.ModelAssignment_LOAD}})

	update := &fleetmlv1.ModelAssignment{ModelId: "m1", ModelName: "iris", Version: "2.0.0", VersionId: "ver-2", VariantId: "var-2",
		DownloadUrl: srv.URL + "/v2", BlobSha256: shaHex(v2), Action: fleetmlv1.ModelAssignment_UPDATE,
		DeltaUrl: srv.URL + "/d", DeltaSha256: shaHex(d), DeltaBaseSha256: shaHex(v1)}
	a.apply(ctx, []*fleetmlv1.ModelAssignment{update})
	got, err := os.ReadFile(filepath.Join(a.currentLink("iris"), "model.bin"))
	require.NoError(t, err)
	assert.Equal(t, v2, got)
	assert.Equal(t, 1, served["/d"])
	assert.Zero(t, served["/v2"], "the full model is not downloaded")

	// A third version whose "delta" is garbage (but has the right sha256 for its bytes) falls back to the full file.
	v3 := append(append([]byte(nil), v2...), '!')
	files["/v3"] = v3
	a.apply(ctx, []*fleetmlv1.ModelAssignment{{ModelId: "m1", ModelName: "iris", Version: "3.0.0", VersionId: "ver-3", VariantId: "var-3",
		DownloadUrl: srv.URL + "/v3", BlobSha256: shaHex(v3), Action: fleetmlv1.ModelAssignment_UPDATE,
		DeltaUrl: srv.URL + "/bad", DeltaSha256: shaHex(files["/bad"]), DeltaBaseSha256: shaHex(v2)}})
	got, _ = os.ReadFile(filepath.Join(a.currentLink("iris"), "model.bin"))
	assert.Equal(t, v3, got)
	assert.Equal(t, 1, served["/v3"])
}

// A failing download is reported once per variant as MODEL_LOAD_FAILED with
// the error (without the presigned query string), however many heartbeats
// retry it; after the model arrives a later failure is reported again.
func TestDownloadFailureIsReportedOnce(t *testing.T) {
	body := []byte("model")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close() // connection refused
	a, err := New(&Config{Server: "127.0.0.1:1", StateDir: t.TempDir()}, quiet())
	require.NoError(t, err)
	defer func() { _ = a.Close() }()
	as := &fleetmlv1.ModelAssignment{ModelId: "m1", ModelName: "iris", Version: "1.0.0", VersionId: "ver-1", VariantId: "var-1",
		DownloadUrl: closed.URL + "/model.bin?X-Amz-Signature=secret", BlobSha256: shaHex(body), Action: fleetmlv1.ModelAssignment_LOAD}
	failures := func() []*fleetmlv1.TelemetryEvent {
		var out []*fleetmlv1.TelemetryEvent
		for _, e := range a.queue.Peek(100) {
			if e.Type == fleetmlv1.TelemetryEvent_MODEL_LOAD_FAILED {
				out = append(out, e)
			}
		}
		return out
	}

	for i := 0; i < 3; i++ {
		a.apply(context.Background(), []*fleetmlv1.ModelAssignment{as})
	}
	require.Len(t, failures(), 1, "three heartbeats, one event")
	ev := failures()[0]
	assert.Equal(t, "ver-1", ev.ModelVersionId)
	assert.Contains(t, ev.ErrorMessage, "download failed: downloading: Get \""+closed.URL+"/model.bin\"")
	assert.NotContains(t, ev.ErrorMessage, "Signature")
	assert.Empty(t, a.ActiveModels())

	as.DownloadUrl = srv.URL
	a.apply(context.Background(), []*fleetmlv1.ModelAssignment{as})
	require.Len(t, a.ActiveModels(), 1)
	assert.Len(t, failures(), 1)
}

// download_host_override sends full downloads and deltas to another address
// with the path and query unchanged and the URL's own host in the Host
// header, which the presigned signature covers.
func TestDownloadHostOverride(t *testing.T) {
	v1 := bytes.Repeat([]byte("weights-"), 20000)
	v2 := append(append([]byte(nil), v1[:150000]...), []byte("a retrained head")...)
	d, err := delta.Diff(v1, v2)
	require.NoError(t, err)
	files := map[string][]byte{"/fleetml-models/v1.bin": v1, "/fleetml-models/v2.bin": v2, "/fleetml-models/v2.delta": d}
	type hit struct{ host, uri string }
	var hits []hit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Host, r.URL.RequestURI()})
		_, _ = w.Write(files[r.URL.Path])
	}))
	defer srv.Close()
	override := strings.TrimPrefix(srv.URL, "http://")
	a, err := New(&Config{Server: "127.0.0.1:1", StateDir: t.TempDir(), DownloadHostOverride: override}, quiet())
	require.NoError(t, err)
	defer func() { _ = a.Close() }()
	const public = "http://localhost:19100"
	const query = "?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-SignedHeaders=host&X-Amz-Signature=abc"
	ctx := context.Background()

	a.apply(ctx, []*fleetmlv1.ModelAssignment{{ModelId: "m1", ModelName: "iris", Version: "1.0.0", VersionId: "ver-1", VariantId: "var-1",
		DownloadUrl: public + "/fleetml-models/v1.bin" + query, BlobSha256: shaHex(v1), Action: fleetmlv1.ModelAssignment_LOAD}})
	a.apply(ctx, []*fleetmlv1.ModelAssignment{{ModelId: "m1", ModelName: "iris", Version: "2.0.0", VersionId: "ver-2", VariantId: "var-2",
		DownloadUrl: public + "/fleetml-models/v2.bin" + query, BlobSha256: shaHex(v2), Action: fleetmlv1.ModelAssignment_UPDATE,
		DeltaUrl: public + "/fleetml-models/v2.delta" + query, DeltaSha256: shaHex(d), DeltaBaseSha256: shaHex(v1)}})

	require.Len(t, a.ActiveModels(), 1)
	assert.Equal(t, "2.0.0", a.ActiveModels()[0].Active.Version)
	assert.Equal(t, []hit{
		{"localhost:19100", "/fleetml-models/v1.bin" + query},
		{"localhost:19100", "/fleetml-models/v2.delta" + query},
	}, hits, "both requests reach the override address with the original Host header, path and query")

	for _, bad := range []string{"http://192.0.2.10:19100", "192.0.2.10", "192.0.2.10:19100/path"} {
		c := &Config{Server: "127.0.0.1:1", StateDir: t.TempDir(), DownloadHostOverride: bad}
		assert.ErrorContains(t, c.Validate(), "download_host_override must be host:port", bad)
	}
}
