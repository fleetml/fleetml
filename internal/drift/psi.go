package drift

import (
	"math"
	"sort"
)

const epsilon = 1e-10

// PSI computes the Population Stability Index between two distributions.
// Both inputs are normalized histogram proportions (must sum to ~1.0 each).
func PSI(expected, actual []float64) float64 {
	if len(expected) != len(actual) || len(expected) == 0 {
		return 0
	}
	var psi float64
	for i := range expected {
		e := expected[i] + epsilon
		a := actual[i] + epsilon
		psi += (a - e) * math.Log(a/e)
	}
	return psi
}

// BuildHistogram converts raw values into a normalized histogram with numBins bins.
func BuildHistogram(values []float64, numBins int) []float64 {
	if len(values) == 0 || numBins <= 0 {
		return nil
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	minVal, maxVal := sorted[0], sorted[len(sorted)-1]
	if minVal == maxVal {
		hist := make([]float64, numBins)
		hist[0] = 1.0
		return hist
	}

	binWidth := (maxVal - minVal) / float64(numBins)
	hist := make([]float64, numBins)
	total := float64(len(values))

	for _, v := range values {
		bin := int((v - minVal) / binWidth)
		if bin >= numBins {
			bin = numBins - 1
		}
		hist[bin]++
	}

	for i := range hist {
		hist[i] /= total
	}
	return hist
}

// PSIFromRawValues builds histograms with shared bin boundaries and computes PSI.
func PSIFromRawValues(baseline, current []float64, numBins int) float64 {
	if len(baseline) == 0 || len(current) == 0 || numBins <= 0 {
		return 0
	}

	// Find shared min/max
	allMin, allMax := baseline[0], baseline[0]
	for _, v := range baseline {
		if v < allMin {
			allMin = v
		}
		if v > allMax {
			allMax = v
		}
	}
	for _, v := range current {
		if v < allMin {
			allMin = v
		}
		if v > allMax {
			allMax = v
		}
	}
	if allMin == allMax {
		return 0
	}

	binWidth := (allMax - allMin) / float64(numBins)
	bHist := buildHistogramFixed(baseline, numBins, allMin, binWidth)
	cHist := buildHistogramFixed(current, numBins, allMin, binWidth)
	return PSI(bHist, cHist)
}

func buildHistogramFixed(values []float64, numBins int, minVal, binWidth float64) []float64 {
	hist := make([]float64, numBins)
	total := float64(len(values))
	for _, v := range values {
		bin := int((v - minVal) / binWidth)
		if bin >= numBins {
			bin = numBins - 1
		}
		if bin < 0 {
			bin = 0
		}
		hist[bin]++
	}
	for i := range hist {
		hist[i] /= total
	}
	return hist
}
