//go:build clitest

package main

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServe(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, "right-key") }()
	base := "http://" + ln.Addr().String()

	get := func(path, key string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, base+path, http.NoBody)
		require.NoError(t, err)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	assert.Equal(t, http.StatusOK, get("/healthz", ""))
	assert.Equal(t, http.StatusUnauthorized, get("/api/v1/projects", "wrong-key"))
	assert.Equal(t, http.StatusUnauthorized, get("/api/v1/projects", ""))
	assert.Equal(t, http.StatusOK, get("/api/v1/projects", "right-key"))

	cancel()
	require.NoError(t, <-done)
}
