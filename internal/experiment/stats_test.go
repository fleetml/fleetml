package experiment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWelchTTest_Significant(t *testing.T) {
	// Control: mean=50, stddev=10, n=100
	// Treatment: mean=45, stddev=10, n=100
	// Should be significant (means differ by 5 with same variance)
	_, pValue := WelchTTest(50, 10, 100, 45, 10, 100)
	assert.Less(t, pValue, 0.05, "should be significant")
}

func TestWelchTTest_NotSignificant(t *testing.T) {
	// Means very close
	_, pValue := WelchTTest(50, 10, 100, 49.5, 10, 100)
	assert.Greater(t, pValue, 0.05, "should not be significant")
}

func TestWelchTTest_ZeroSampleSize(t *testing.T) {
	_, pValue := WelchTTest(50, 10, 0, 45, 10, 0)
	assert.Equal(t, 1.0, pValue)
}

func TestWelchTTest_SampleSizeOne(t *testing.T) {
	_, pValue := WelchTTest(50, 10, 1, 45, 10, 100)
	assert.Equal(t, 1.0, pValue)
}

func TestWelchTTest_IdenticalMeans(t *testing.T) {
	_, pValue := WelchTTest(50, 10, 100, 50, 10, 100)
	assert.InDelta(t, 1.0, pValue, 0.01)
}

func TestWelchTTest_ZeroVariance(t *testing.T) {
	_, pValue := WelchTTest(50, 0, 100, 50, 0, 100)
	assert.Equal(t, 1.0, pValue, "same means with zero variance")

	_, pValue2 := WelchTTest(50, 0, 100, 45, 0, 100)
	assert.Less(t, pValue2, 0.01, "different means with zero variance should be significant")
}

func TestWelchTTest_LargeDifference(t *testing.T) {
	_, pValue := WelchTTest(100, 10, 500, 50, 10, 500)
	assert.Less(t, pValue, 0.001, "large difference should be very significant")
}

func TestCohenD_Small(t *testing.T) {
	d := CohenD(50, 10, 100, 48, 10, 100) // 0.2 difference
	assert.Less(t, d, 0.3, "should be small effect")
}

func TestCohenD_Medium(t *testing.T) {
	d := CohenD(50, 10, 100, 45, 10, 100) // 0.5 difference
	assert.InDelta(t, 0.5, d, 0.1, "should be medium effect")
}

func TestCohenD_Large(t *testing.T) {
	d := CohenD(50, 10, 100, 42, 10, 100) // 0.8 difference
	assert.Greater(t, d, 0.7, "should be large effect")
}

func TestCohenD_ZeroSample(t *testing.T) {
	d := CohenD(50, 10, 0, 45, 10, 0)
	assert.Equal(t, 0.0, d)
}

func TestEvaluate_LatencySignificant(t *testing.T) {
	control := CohortStats{SampleSize: 500, AvgLatency: 50, StdDevLatency: 10}
	treatment := CohortStats{SampleSize: 500, AvgLatency: 40, StdDevLatency: 10}
	result := Evaluate(control, treatment, "latency", 0.95)
	assert.True(t, result.IsSignificant)
	assert.Equal(t, "ADOPT_TREATMENT", result.Recommendation)
}

func TestEvaluate_LatencyNotSignificant(t *testing.T) {
	control := CohortStats{SampleSize: 50, AvgLatency: 50, StdDevLatency: 20}
	treatment := CohortStats{SampleSize: 50, AvgLatency: 49, StdDevLatency: 20}
	result := Evaluate(control, treatment, "latency", 0.95)
	assert.False(t, result.IsSignificant)
	assert.Equal(t, "INCONCLUSIVE", result.Recommendation)
}

func TestEvaluate_TreatmentWorse(t *testing.T) {
	control := CohortStats{SampleSize: 500, AvgLatency: 40, StdDevLatency: 10}
	treatment := CohortStats{SampleSize: 500, AvgLatency: 50, StdDevLatency: 10}
	result := Evaluate(control, treatment, "latency", 0.95)
	assert.True(t, result.IsSignificant)
	assert.Equal(t, "KEEP_CONTROL", result.Recommendation)
}

func TestEvaluate_Confidence(t *testing.T) {
	control := CohortStats{SampleSize: 500, AvgConfidence: 0.85}
	treatment := CohortStats{SampleSize: 500, AvgConfidence: 0.92}
	result := Evaluate(control, treatment, "confidence", 0.95)
	assert.True(t, result.IsSignificant)
	assert.Equal(t, "ADOPT_TREATMENT", result.Recommendation)
}

func TestEvaluate_ErrorRate(t *testing.T) {
	control := CohortStats{SampleSize: 1000, ErrorRate: 0.05}
	treatment := CohortStats{SampleSize: 1000, ErrorRate: 0.01}
	result := Evaluate(control, treatment, "error_rate", 0.95)
	// With binomial approximation, this should be significant
	assert.Equal(t, "ADOPT_TREATMENT", result.Recommendation)
}

func TestEvaluate_InsufficientData(t *testing.T) {
	control := CohortStats{SampleSize: 2, AvgLatency: 50, StdDevLatency: 10}
	treatment := CohortStats{SampleSize: 2, AvgLatency: 40, StdDevLatency: 10}
	result := Evaluate(control, treatment, "latency", 0.95)
	// With only 2 samples each, unlikely to be significant
	assert.Equal(t, "INCONCLUSIVE", result.Recommendation)
}

// 5.0% vs 6.0% errors over 1000 events per arm is not significant
// (two-proportion p ~ 0.33). Passing the standard error as the SD, as the
// old code did, shrank the variance 1000-fold and reported p ~ 0.
func TestEvaluate_ErrorRateUsesBernoulliSD(t *testing.T) {
	control := CohortStats{SampleSize: 1000, ErrorRate: 0.05}
	treatment := CohortStats{SampleSize: 1000, ErrorRate: 0.06}
	r := Evaluate(control, treatment, "error_rate", 0.95)
	require.NotNil(t, r.PValue)
	assert.InDelta(t, 0.33, *r.PValue, 0.03)
	assert.False(t, r.IsSignificant)

	// A real difference is still detected: 5% vs 9%.
	treatment.ErrorRate = 0.09
	r = Evaluate(control, treatment, "error_rate", 0.95)
	assert.True(t, r.IsSignificant)
	assert.Equal(t, "KEEP_CONTROL", r.Recommendation)
}

// Confidence uses the measured SD. With a tight spread (0.02) a 0.80 vs 0.82
// difference over 100 events per arm is clearly significant; the old fixed
// SD of 0.1 hid it.
func TestEvaluate_ConfidenceUsesMeasuredSD(t *testing.T) {
	control := CohortStats{SampleSize: 100, AvgConfidence: 0.80, StdDevConfidence: 0.02}
	treatment := CohortStats{SampleSize: 100, AvgConfidence: 0.82, StdDevConfidence: 0.02}
	r := Evaluate(control, treatment, "confidence", 0.95)
	assert.True(t, r.IsSignificant)
	assert.Equal(t, "ADOPT_TREATMENT", r.Recommendation)

	// And a wide measured spread makes the same means inconclusive.
	control.StdDevConfidence, treatment.StdDevConfidence = 0.3, 0.3
	r = Evaluate(control, treatment, "confidence", 0.95)
	assert.False(t, r.IsSignificant)
}
