// Package features bins input feature values for feature-level drift. It has
// no dependencies beyond the standard library, so the device agent uses it.
package features

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
)

// MaxEdges bounds the bins of one feature.
const MaxEdges = 50

var validName = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,100}$`)

// Validate checks a feature name and its bin edges (strictly increasing, finite).
func Validate(name string, edges []float64) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("feature name %q: use 1 to 100 letters, digits, '_', '.', ':' or '-'", name)
	}
	if len(edges) == 0 || len(edges) > MaxEdges {
		return fmt.Errorf("feature %s: between 1 and %d edges are required", name, MaxEdges)
	}
	for i, e := range edges {
		if math.IsNaN(e) || math.IsInf(e, 0) {
			return fmt.Errorf("feature %s: edges must be finite numbers", name)
		}
		if i > 0 && e <= edges[i-1] {
			return errors.New("feature " + name + ": edges must be strictly increasing")
		}
	}
	return nil
}

// Bin counts values into len(edges)+1 bins: below edges[0], [edges[i-1],
// edges[i]), and at or above the last edge. NaN values are skipped.
func Bin(edges, values []float64) []uint32 {
	counts := make([]uint32, len(edges)+1)
	for _, v := range values {
		if math.IsNaN(v) {
			continue
		}
		counts[sort.Search(len(edges), func(i int) bool { return edges[i] > v })]++
	}
	return counts
}
