package cli

import (
	"os"
	"path/filepath"
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

// A file or pipe is not a terminal, so push draws no progress bar into it.
func TestIsTerminal_File(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	assert.False(t, isTerminal(f))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	assert.False(t, isTerminal(w))
}
