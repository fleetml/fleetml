//go:build unix

package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
	"github.com/fleetml/fleetml/internal/delta"
)

// An active model is readable by any user, whether it was downloaded in
// full or built from a delta, and whatever the agent's umask: the inference
// process usually runs as another user than fleetml-agent.
func TestActiveModelIsReadableByOtherUsers(t *testing.T) {
	old := syscall.Umask(0o077)
	defer syscall.Umask(old)

	v1 := []byte("weights of one")
	v2 := []byte("weights of two")
	d, err := delta.Diff(v1, v2)
	require.NoError(t, err)
	files := map[string][]byte{"/v1": v1, "/d": d}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(files[r.URL.Path]) }))
	defer srv.Close()
	// The agent creates its state_dir (no systemd StateDirectory= here).
	a, err := New(&Config{Server: "127.0.0.1:1", StateDir: filepath.Join(t.TempDir(), "state")}, quiet())
	require.NoError(t, err)
	defer func() { _ = a.Close() }()
	ctx := context.Background()

	assertReadable := func(what string) {
		t.Helper()
		m := a.ActiveModels()[0]
		fi, err := os.Stat(filepath.Join(m.Active.Dir, modelFile))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm(), "%s: model file mode", what)
		for _, dir := range []string{m.Active.Dir, filepath.Dir(m.Active.Dir), a.cfg.ModelsDir, a.cfg.StateDir} {
			fi, err := os.Stat(dir)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o755), fi.Mode().Perm(), "%s: mode of %s", what, dir)
		}
	}

	a.apply(ctx, []*fleetmlv1.ModelAssignment{{ModelId: "m1", ModelName: "iris", Version: "1.0.0", VersionId: "ver-1",
		VariantId: "var-1", DownloadUrl: srv.URL + "/v1", BlobSha256: shaHex(v1), Action: fleetmlv1.ModelAssignment_LOAD}})
	require.Len(t, a.ActiveModels(), 1)
	assertReadable("full download")

	a.apply(ctx, []*fleetmlv1.ModelAssignment{{ModelId: "m1", ModelName: "iris", Version: "2.0.0", VersionId: "ver-2",
		VariantId: "var-2", DownloadUrl: srv.URL + "/missing", BlobSha256: shaHex(v2), Action: fleetmlv1.ModelAssignment_UPDATE,
		DeltaUrl: srv.URL + "/d", DeltaSha256: shaHex(d), DeltaBaseSha256: shaHex(v1)}})
	require.Equal(t, "2.0.0", a.ActiveModels()[0].Active.Version)
	assertReadable("delta")

	fi, err := os.Stat(filepath.Join(a.cfg.StateDir, "downloads"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), fi.Mode().Perm(), "partial downloads stay private")
}
