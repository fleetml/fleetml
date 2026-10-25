package main

import (
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	return addr
}

func getJSON(t *testing.T, url string) (code int, body map[string]any) {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	return resp.StatusCode, body
}

// While start-up waits for its dependencies, /livez is 200 (a startup probe
// does not restart a server that only waits) and /readyz is 503 naming the
// dependency.
func TestStartupListener(t *testing.T) {
	addr := freeAddr(t)
	l, err := listenDuringStartup(addr)
	require.NoError(t, err)
	l.wait("database")

	code, body := getJSON(t, "http://"+addr+"/livez")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "ok", body["status"])
	assert.NotEmpty(t, body["version"])

	code, body = getJSON(t, "http://"+addr+"/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "starting", body["status"])
	assert.Equal(t, "database", body["waiting_for"])

	code, body = getJSON(t, "http://"+addr+"/api/v1/projects")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, "UNAVAILABLE", body["error"].(map[string]any)["code"])

	l.stop()
	l.stop() // a second stop is a no-op
	ln, err := net.Listen("tcp", addr)
	require.NoError(t, err, "stop frees the port for the REST server")
	require.NoError(t, ln.Close())
}

// The dependencies share one deadline: time spent on the first is gone for
// the next.
func TestWaitFor_SharedDeadline(t *testing.T) {
	deadline := time.Now().Add(300 * time.Millisecond)
	start := time.Now()
	err := waitFor(t.Context(), "first", deadline, func() error {
		if time.Since(start) < 200*time.Millisecond {
			return assert.AnError
		}
		return nil
	})
	require.NoError(t, err)

	err = waitFor(t.Context(), "second", deadline, func() error { return assert.AnError })
	require.Error(t, err)
	assert.Less(t, time.Since(start), 1500*time.Millisecond, "the second wait ends at the shared deadline, not a fresh one")
}
