// The race detector makes this timing test take minutes and says nothing
// about it; the small tests in this package cover the same code under -race.
//go:build !race

package delta

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A 20 MB model whose last 2% changed: how long Diff takes and how big the delta is.
func TestLargeModel(t *testing.T) {
	if testing.Short() {
		t.Skip("large")
	}
	r := rand.New(rand.NewSource(2))
	old := make([]byte, 20<<20)
	r.Read(old)
	next := append([]byte(nil), old...)
	r.Read(next[len(next)-len(next)/50:])
	start := time.Now()
	d := roundTrip(t, old, next)
	t.Logf("20 MB, last 2%% changed: delta %d bytes (%.1f%%) in %s", len(d), 100*float64(len(d))/float64(len(next)), time.Since(start))
	require.Less(t, len(d), len(next)/20)
}
