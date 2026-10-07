package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProgressWriter(t *testing.T) {
	pw := &ProgressWriter{Total: 100}
	n, err := pw.Write(make([]byte, 50))
	assert.NoError(t, err)
	assert.Equal(t, 50, n)
	assert.Equal(t, int64(50), pw.Written)
}

func TestProgressWriter_Complete(t *testing.T) {
	pw := &ProgressWriter{Total: 100}
	_, _ = pw.Write(make([]byte, 100))
	assert.Equal(t, int64(100), pw.Written)
}
