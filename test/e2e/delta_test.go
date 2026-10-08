//go:build e2e

package e2e

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/delta"
)

func get(t *testing.T, url string) []byte {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return b
}

// V3.4: publishing a new version builds a delta from the previous one; a
// device that has the previous version is offered it, and applying it gives
// the new model's exact bytes. A device without the base gets no delta.
func TestE2E_DeltaUpdates(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	v1Blob := randomBlob(200_000)
	v2Blob := append(append([]byte(nil), v1Blob[:190_000]...), []byte("a retrained classification head")...)
	v1, sha1, _ := e.newVersionOf(t, f, "1.0.0", v1Blob, "")
	labels := map[string]string{"suite": randHex(4)}
	e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v1, "target_selector": labels})
	d := e.register(t, f.enrollKey, labels)
	resp := e.heartbeat(t, d)
	require.Len(t, resp.Assignments, 1)
	assert.Empty(t, resp.Assignments[0].DeltaUrl, "nothing to diff against yet")
	assert.Equal(t, sha1, resp.Assignments[0].BlobSha256)

	v2, sha2, _ := e.newVersionOf(t, f, "2.0.0", v2Blob, "")
	e.deploy(t, map[string]any{"project_id": f.project, "model_version_id": v2, "target_selector": labels})
	var update *fleetmlv1.ModelAssignment
	waitFor(t, 20*time.Second, "the delta to be built", func() bool {
		req := &fleetmlv1.HeartbeatRequest{DeviceId: d.id, LoadedModels: []*fleetmlv1.ModelStatus{
			{ModelId: f.model, Version: "1.0.0", VariantId: d.loaded[f.model].VariantId, Status: "LOADED"}}}
		r, err := e.devices.Heartbeat(d.ctx(), req)
		require.NoError(t, err)
		for _, a := range r.Assignments {
			if a.Action == fleetmlv1.ModelAssignment_UPDATE && a.DeltaUrl != "" {
				update = a
			}
		}
		return update != nil
	})
	assert.Equal(t, sha1, update.DeltaBaseSha256)
	assert.Less(t, update.DeltaSizeBytes, update.BlobSizeBytes/10, "the delta is a fraction of the model")

	patch := get(t, update.DeltaUrl)
	sum := sha256.Sum256(patch)
	require.Equal(t, update.DeltaSha256, hex.EncodeToString(sum[:]))
	out, err := delta.Patch(v1Blob, patch)
	require.NoError(t, err)
	sum = sha256.Sum256(out)
	assert.Equal(t, sha2, hex.EncodeToString(sum[:]), "the patched model is exactly 2.0.0")
	assert.Equal(t, sha2, update.BlobSha256)

	fresh := e.register(t, f.enrollKey, labels)
	resp = e.heartbeat(t, fresh)
	require.Len(t, resp.Assignments, 1)
	assert.Empty(t, resp.Assignments[0].DeltaUrl, "a device without the base downloads the full model")
}
