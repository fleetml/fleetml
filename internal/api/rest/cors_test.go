package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newCORSRouter(origins string) *gin.Engine {
	r := gin.New()
	r.Use(CORS(origins))
	r.GET("/api/v1/projects", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func corsRequest(r *gin.Engine, method, origin string, preflight bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/projects", http.NoBody)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if preflight {
		req.Header.Set("Access-Control-Request-Method", "DELETE")
		req.Header.Set("Access-Control-Request-Headers", "authorization")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCORS_DefaultAllowsDashboardOrigins(t *testing.T) {
	r := newCORSRouter(DefaultCORSOrigins)
	for _, o := range []string{"http://localhost:5173", "http://localhost:18080"} {
		w := corsRequest(r, "GET", o, false)
		assert.Equal(t, o, w.Header().Get("Access-Control-Allow-Origin"))
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
		assert.Equal(t, "Origin", w.Header().Get("Vary"))
	}
}

func TestCORS_NeverReflectsUnknownOrigin(t *testing.T) {
	r := newCORSRouter(DefaultCORSOrigins)
	for _, o := range []string{"https://evil.example", "http://localhost:5173.evil.example", "null", "http://localhost:5174"} {
		w := corsRequest(r, "GET", o, false)
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"), o)
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"), o)

		pre := corsRequest(r, "OPTIONS", o, true)
		assert.Equal(t, http.StatusForbidden, pre.Code, o)
		assert.Empty(t, pre.Header().Get("Access-Control-Allow-Origin"), o)
	}
}

func TestCORS_PreflightForAllowedOrigin(t *testing.T) {
	r := newCORSRouter(DefaultCORSOrigins)
	w := corsRequest(r, "OPTIONS", "http://localhost:5173", true)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "DELETE")
}

func TestCORS_WildcardNeverWithCredentials(t *testing.T) {
	r := newCORSRouter("*")
	w := corsRequest(r, "GET", "https://evil.example", false)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"), "wildcard is literal, never the reflected origin")
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}

func TestCORS_NormalisesConfiguredOrigins(t *testing.T) {
	r := newCORSRouter(" http://LOCALHOST:80/ , https://dash.example.com:443")
	assert.Equal(t, "http://localhost", corsRequest(r, "GET", "http://localhost", false).Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "https://dash.example.com", corsRequest(r, "GET", "https://dash.example.com", false).Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, corsRequest(r, "GET", "http://dash.example.com", false).Header().Get("Access-Control-Allow-Origin"), "scheme must match")
}

func TestCORS_EmptyListAllowsNothing(t *testing.T) {
	r := newCORSRouter("")
	assert.Empty(t, corsRequest(r, "GET", "http://localhost:5173", false).Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, corsRequest(r, "GET", "", false).Code, "same-origin and non-browser requests are unaffected")
}
