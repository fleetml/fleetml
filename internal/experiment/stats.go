package experiment

import (
	"math"

	"gonum.org/v1/gonum/stat/distuv"
)

// WelchTTest performs Welch's t-test for unequal variances.
// Returns t-statistic and two-tailed p-value.
func WelchTTest(mean1, stddev1 float64, n1 int64, mean2, stddev2 float64, n2 int64) (tStat, pValue float64) {
	if n1 <= 1 || n2 <= 1 {
		return 0, 1
	}
	if stddev1 == 0 && stddev2 == 0 {
		if mean1 == mean2 {
			return 0, 1
		}
		// Means differ but no variance — treat as highly significant
		return 100, 0
	}

	s1sq := stddev1 * stddev1
	s2sq := stddev2 * stddev2
	fn1 := float64(n1)
	fn2 := float64(n2)

	se := math.Sqrt(s1sq/fn1 + s2sq/fn2)
	if se == 0 {
		return 0, 1
	}

	tStat = (mean1 - mean2) / se

	// Welch-Satterthwaite degrees of freedom
	num := (s1sq/fn1 + s2sq/fn2) * (s1sq/fn1 + s2sq/fn2)
	denom := (s1sq*s1sq)/(fn1*fn1*(fn1-1)) + (s2sq*s2sq)/(fn2*fn2*(fn2-1))
	if denom == 0 {
		return tStat, 0
	}
	df := num / denom

	if df < 1 {
		df = 1
	}

	tdist := distuv.StudentsT{Mu: 0, Sigma: 1, Nu: df}
	pValue = 2 * (1 - tdist.CDF(math.Abs(tStat)))
	if pValue > 1 {
		pValue = 1
	}
	return tStat, pValue
}

// CohenD computes Cohen's d effect size.
func CohenD(mean1, stddev1 float64, n1 int64, mean2, stddev2 float64, n2 int64) float64 {
	if n1 <= 1 || n2 <= 1 {
		return 0
	}
	fn1 := float64(n1)
	fn2 := float64(n2)
	pooledVar := ((fn1-1)*stddev1*stddev1 + (fn2-1)*stddev2*stddev2) / (fn1 + fn2 - 2)
	if pooledVar <= 0 {
		return 0
	}
	return math.Abs(mean1-mean2) / math.Sqrt(pooledVar)
}

// Evaluate compares two cohorts and produces an ExperimentResult.
func Evaluate(control, treatment CohortStats, primaryMetric string, confidenceLevel float64) *ExperimentResult {
	var mean1, std1, mean2, std2 float64

	switch primaryMetric {
	case "latency":
		mean1, std1 = control.AvgLatency, control.StdDevLatency
		mean2, std2 = treatment.AvgLatency, treatment.StdDevLatency
	case "error_rate":
		// Each event is a Bernoulli trial: the per-event SD is sqrt(p(1-p)).
		// (WelchTTest divides by n itself; passing the standard error here
		// would shrink the variance n-fold and make noise look significant.)
		mean1, std1 = control.ErrorRate, bernoulliSD(control.ErrorRate)
		mean2, std2 = treatment.ErrorRate, bernoulliSD(treatment.ErrorRate)
	case "confidence":
		mean1, std1 = control.AvgConfidence, control.StdDevConfidence
		mean2, std2 = treatment.AvgConfidence, treatment.StdDevConfidence
	default:
		mean1, std1 = control.AvgLatency, control.StdDevLatency
		mean2, std2 = treatment.AvgLatency, treatment.StdDevLatency
	}

	_, pValue := WelchTTest(mean1, std1, control.SampleSize, mean2, std2, treatment.SampleSize)
	effectSize := CohenD(mean1, std1, control.SampleSize, mean2, std2, treatment.SampleSize)
	isSignificant := pValue < (1 - confidenceLevel)

	recommendation := "INCONCLUSIVE"
	if isSignificant {
		switch primaryMetric {
		case "latency", "error_rate":
			if mean2 < mean1 {
				recommendation = "ADOPT_TREATMENT"
			} else {
				recommendation = "KEEP_CONTROL"
			}
		case "confidence":
			if mean2 > mean1 {
				recommendation = "ADOPT_TREATMENT"
			} else {
				recommendation = "KEEP_CONTROL"
			}
		}
	}

	return &ExperimentResult{
		ControlStats:   control,
		TreatmentStats: treatment,
		PValue:         &pValue,
		IsSignificant:  isSignificant,
		EffectSize:     &effectSize,
		Recommendation: recommendation,
	}
}

func bernoulliSD(p float64) float64 {
	if p <= 0 || p >= 1 {
		return 0
	}
	return math.Sqrt(p * (1 - p))
}
