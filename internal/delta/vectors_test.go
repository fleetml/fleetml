package delta

import (
	"bytes"
	"math/rand"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// testdata/old.bin, new.bin and patch.fmldelta are applied by the Android and
// iOS SDK tests too, so every client agrees with this package on the format.
func TestVectors(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	old := make([]byte, 30_000)
	r.Read(old)
	next := append([]byte(nil), old[:12_000]...)
	next = append(next, []byte("an inserted layer")...)
	next = append(next, old[12_000:25_000]...)
	tail := make([]byte, 4_000)
	r.Read(tail)
	next = append(next, tail...)
	for i := 0; i < 50; i++ {
		next[r.Intn(len(next))] ^= 0x5a
	}
	if os.Getenv("UPDATE_VECTORS") == "1" {
		d, err := Diff(old, next)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile("testdata/old.bin", old, 0o644))
		require.NoError(t, os.WriteFile("testdata/new.bin", next, 0o644))
		require.NoError(t, os.WriteFile("testdata/patch.fmldelta", d, 0o644))
	}
	storedOld, err := os.ReadFile("testdata/old.bin")
	require.NoError(t, err)
	storedNew, err := os.ReadFile("testdata/new.bin")
	require.NoError(t, err)
	patch, err := os.ReadFile("testdata/patch.fmldelta")
	require.NoError(t, err)
	require.True(t, bytes.Equal(old, storedOld) && bytes.Equal(next, storedNew), "testdata is up to date (UPDATE_VECTORS=1 regenerates it)")
	got, err := Patch(storedOld, patch)
	require.NoError(t, err)
	require.True(t, bytes.Equal(storedNew, got))
}
