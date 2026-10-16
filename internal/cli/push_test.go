package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/cli/clitest"
)

// runCLI runs the CLI with args against server, with an empty home directory.
func runCLI(t *testing.T, server, apiKey string, args ...string) error {
	t.Helper()
	tempHome(t)
	t.Setenv("FLEETML_SERVER", server)
	t.Setenv("FLEETML_API_KEY", apiKey)
	root := NewRootCmd()
	root.SetArgs(args)
	return root.Execute()
}

// A model file opened by a path with spaces (and, on Windows, backslashes and
// a drive letter) is hashed and uploaded byte for byte.
func TestPush_PathWithSpaces(t *testing.T) {
	fake := clitest.NewServer(t, "k-push")
	dir := filepath.Join(t.TempDir(), "my models", "iris v1")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	file := filepath.Join(dir, "iris model.tflite")
	blob := []byte("TFL3\x00\r\n\x1a binary model bytes, CRLF and NUL kept as they are")
	require.NoError(t, os.WriteFile(file, blob, 0o644))

	require.NoError(t, runCLI(t, fake.URL, "k-push", "project", "create", "--name", "push"))
	projectID := fake.Projects()[0]["id"].(string)
	require.NoError(t, runCLI(t, fake.URL, "k-push", "push", file, "--project", projectID, "--model", "iris", "--version", "1.0.0"))

	variants := fake.Variants()
	require.Len(t, variants, 1)
	sum := sha256.Sum256(blob)
	assert.Equal(t, hex.EncodeToString(sum[:]), variants[0]["blob_sha256"])
	assert.Equal(t, "READY", variants[0]["status"], "finalize accepted the upload")
	assert.Equal(t, blob, fake.Upload(variants[0]["id"].(string)))

	// The same version again: push finds the existing model and version.
	require.NoError(t, runCLI(t, fake.URL, "k-push", "push", file, "--project", projectID, "--model", "iris", "--version", "1.0.0", "--quantization", "FP16"))
	assert.Len(t, fake.Variants(), 2)
}

func TestPush_MissingFile(t *testing.T) {
	fake := clitest.NewServer(t, "k")
	err := runCLI(t, fake.URL, "k", "push", filepath.Join(t.TempDir(), "no such dir", "m.tflite"), "--project", "p", "--model", "m", "--version", "1.0.0")
	assert.ErrorContains(t, err, "file not found")
}
