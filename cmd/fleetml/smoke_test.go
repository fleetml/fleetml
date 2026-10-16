package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/cli/clitest"
)

// TestQuickstartCalls builds the fleetml binary and runs the quickstart's CLI
// calls with it: project create, push, deploy create and start, device list.
// By default it runs against clitest's fake of the REST API, which works on
// every OS (the Windows CI job). With FLEETML_SMOKE_SERVER and
// FLEETML_SMOKE_API_KEY set it runs the same calls against a real server
// (use a throwaway one: it creates a project and a deployment).
func TestQuickstartCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI")
	}
	server, apiKey := os.Getenv("FLEETML_SMOKE_SERVER"), os.Getenv("FLEETML_SMOKE_API_KEY")
	if server == "" {
		apiKey = "smoke-admin-key"
		server = clitest.NewServer(t, apiKey).URL
	}

	bin := filepath.Join(t.TempDir(), "fleetml")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	// No VCS stamping: the test needs no git (and no safe.directory) to build.
	build := exec.Command("go", "build", "-buildvcs=false", "-o", bin, ".")
	buildOut, err := build.CombinedOutput()
	require.NoError(t, err, string(buildOut))

	home := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), "FLEETML_SERVER="+server, "FLEETML_API_KEY="+apiKey,
			"HOME="+home, "USERPROFILE="+home, "FLEETML_OUTPUT=")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "fleetml %v:\n%s", args, out)
		return string(out)
	}
	runJSON := func(v any, args ...string) {
		t.Helper()
		require.NoError(t, json.Unmarshal([]byte(run(append([]string{"-o", "json"}, args...)...)), v))
	}

	assert.Contains(t, run("version"), "fleetml ")

	out := run("project", "create", "--name", fmt.Sprintf("smoke-%d", time.Now().UnixNano()))
	m := regexp.MustCompile(`ID: (\S+)`).FindStringSubmatch(out)
	require.NotNil(t, m, out)
	projectID := m[1]

	// The sample model, copied to a path with spaces.
	dir := filepath.Join(t.TempDir(), "my models")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	model := filepath.Join(dir, "iris v1.tflite")
	copyFile(t, filepath.Join("..", "..", "models", "iris", "iris_v1.tflite"), model)
	assert.Contains(t, run("push", model, "--project", projectID, "--model", "iris", "--version", "1.0.0"), "Pushed")

	var models []map[string]any
	runJSON(&models, "model", "list", "--project", projectID)
	require.Len(t, models, 1)
	var versions []map[string]any
	runJSON(&versions, "ver", "list", "--model", fmt.Sprint(models[0]["id"]))
	require.Len(t, versions, 1)

	var dep map[string]any
	runJSON(&dep, "deploy", "create", "--project", projectID, "--version", fmt.Sprint(versions[0]["id"]), "--target", `{"app":"smoke"}`)
	assert.Equal(t, "PENDING", dep["status"])
	assert.Contains(t, run("deploy", "start", fmt.Sprint(dep["id"])), "Deployment start → COMPLETE")

	out = run("device", "list")
	assert.Regexp(t, `ID\s+OS\s+RAM\(MB\)\s+Chipset|No results\.`, out)
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from)
	require.NoError(t, err)
	defer func() { _ = src.Close() }()
	dst, err := os.Create(to)
	require.NoError(t, err)
	_, err = io.Copy(dst, src)
	require.NoError(t, err)
	require.NoError(t, dst.Close())
}
