package features

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBin(t *testing.T) {
	edges := []float64{1, 2, 3}
	assert.Equal(t, []uint32{1, 2, 1, 2}, Bin(edges, []float64{0.5, 1, 1.9, 2, 3, 99}), "a value on an edge falls into the bin above it")
	assert.Equal(t, []uint32{0, 0, 0, 0}, Bin(edges, []float64{math.NaN()}))
	assert.Equal(t, []uint32{0, 0}, Bin([]float64{0}, nil))
}

func TestValidate(t *testing.T) {
	assert.NoError(t, Validate("petal_length", []float64{1, 2.5, 4}))
	assert.Error(t, Validate("", []float64{1}))
	assert.Error(t, Validate("bad name", []float64{1}))
	assert.Error(t, Validate("f", nil))
	assert.Error(t, Validate("f", []float64{1, 1}))
	assert.Error(t, Validate("f", []float64{2, 1}))
	assert.Error(t, Validate("f", []float64{math.Inf(1)}))
	assert.Error(t, Validate("f", make([]float64, MaxEdges+1)))
}
