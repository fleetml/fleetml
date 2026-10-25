package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/health"
	"github.com/fleetml/fleetml/internal/version"
)

func probeRouter(dbErr error) *gin.Engine {
	checker := health.NewChecker(nil)
	checker.Add("db", func(context.Context) error { return dbErr })
	s := &Server{readiness: checker}
	r := gin.New()
	r.GET("/livez", s.live)
	r.GET("/readyz", s.ready)
	return r
}

func probe(t *testing.T, r *gin.Engine, path string) (code int, body map[string]string) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, http.NoBody))
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w.Code, body
}

// Both probes report the server's version: a release build says the
// release, any other build "dev".
func TestProbes_ReportVersion(t *testing.T) {
	defer func(v string) { version.Version = v }(version.Version)
	for _, v := range []string{"0.9.0", "dev"} {
		version.Version = v
		r := probeRouter(nil)
		code, body := probe(t, r, "/livez")
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, map[string]string{"status": "ok", "version": v}, body)

		code, body = probe(t, r, "/readyz")
		assert.Equal(t, http.StatusOK, code)
		assert.Equal(t, map[string]string{"status": "ok", "version": v, "db": "connected"}, body)
	}
}

// A degraded server still says which version it is.
func TestProbes_ReadyzDegradedKeepsVersion(t *testing.T) {
	defer func(v string) { version.Version = v }(version.Version)
	version.Version = "v0.9.0"
	code, body := probe(t, probeRouter(errors.New("down")), "/readyz")
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, map[string]string{"status": "degraded", "version": "0.9.0", "db": "disconnected"}, body)
}
