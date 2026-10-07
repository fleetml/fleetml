package drift

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPSI_IdenticalDistributions(t *testing.T) {
	hist := []float64{0.1, 0.2, 0.3, 0.2, 0.1, 0.1}
	psi := PSI(hist, hist)
	assert.InDelta(t, 0, psi, 0.001, "identical distributions should have PSI ≈ 0")
}

func TestPSI_ShiftedDistribution(t *testing.T) {
	expected := []float64{0.3, 0.3, 0.2, 0.1, 0.1}
	actual := []float64{0.1, 0.1, 0.2, 0.3, 0.3}
	psi := PSI(expected, actual)
	assert.Greater(t, psi, 0.1, "shifted distribution should have PSI > 0.1")
}

func TestPSI_EmptyInput(t *testing.T) {
	assert.Equal(t, 0.0, PSI(nil, nil))
	assert.Equal(t, 0.0, PSI([]float64{0.5, 0.5}, []float64{0.5}))
}

func TestBuildHistogram_Basic(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	hist := BuildHistogram(values, 5)
	assert.Len(t, hist, 5)
	var sum float64
	for _, v := range hist {
		sum += v
	}
	assert.InDelta(t, 1.0, sum, 0.01)
}

func TestBuildHistogram_ConstantValues(t *testing.T) {
	values := []float64{5, 5, 5, 5, 5}
	hist := BuildHistogram(values, 3)
	assert.Equal(t, 1.0, hist[0])
}

func TestBuildHistogram_Empty(t *testing.T) {
	assert.Nil(t, BuildHistogram(nil, 5))
}

func TestPSIFromRawValues_SameDistribution(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	var data []float64
	for i := 0; i < 1000; i++ {
		data = append(data, rng.NormFloat64()*10+50)
	}
	psi := PSIFromRawValues(data, data, 10)
	assert.InDelta(t, 0, psi, 0.01)
}

func TestPSIFromRawValues_DifferentMeans(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	var baseline, current []float64
	for i := 0; i < 1000; i++ {
		baseline = append(baseline, rng.NormFloat64()*10+50)
		current = append(current, rng.NormFloat64()*10+70) // shifted mean
	}
	psi := PSIFromRawValues(baseline, current, 10)
	assert.Greater(t, psi, 0.25, "different means should produce high PSI")
}

func TestPSI_Severity(t *testing.T) {
	assert.Equal(t, "NONE", classifySeverity(0.05))
	assert.Equal(t, "WARNING", classifySeverity(0.15))
	assert.Equal(t, "CRITICAL", classifySeverity(0.30))
}
