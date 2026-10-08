package rollout

import (
	"sort"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestInCanary_DeterministicAndBounded(t *testing.T) {
	dev, dep := uuid.New(), uuid.New()
	assert.Equal(t, InCanary(dev, dep, 50), InCanary(dev, dep, 50))
	assert.False(t, InCanary(dev, dep, 0))
	assert.True(t, InCanary(dev, dep, 100))
}

func TestCohortsOnlyGrow(t *testing.T) {
	dep := uuid.New()
	devices := make([]uuid.UUID, 2000)
	for i := range devices {
		devices[i] = uuid.New()
	}
	prev := map[uuid.UUID]bool{}
	for _, pct := range []int{1, 10, 25, 50, 100} {
		cur := map[uuid.UUID]bool{}
		for _, d := range devices {
			if Exposed(d, dep, PercentThreshold(pct)) {
				cur[d] = true
			}
		}
		for d := range prev {
			assert.True(t, cur[d], "a device exposed at a smaller percentage stays exposed")
		}
		assert.InDelta(t, float64(pct)/100, float64(len(cur))/float64(len(devices)), 0.05)
		prev = cur
	}
	assert.Len(t, prev, len(devices))
}

func TestThresholdFromSortedPositions(t *testing.T) {
	dep := uuid.New()
	devices := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	pos := make([]int64, len(devices))
	for i, d := range devices {
		pos[i] = int64(Position(d, dep))
	}
	sort.Slice(pos, func(i, j int) bool { return pos[i] < pos[j] })
	threshold := pos[1] + 1 // first two devices
	n := 0
	for _, d := range devices {
		if Exposed(d, dep, threshold) {
			n++
		}
	}
	assert.Equal(t, 2, n)
	assert.Equal(t, RingSize, PercentThreshold(100))
	assert.Equal(t, int64(0), PercentThreshold(0))
}
