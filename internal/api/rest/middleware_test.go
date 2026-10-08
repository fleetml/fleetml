package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInternalError_LogsCauseWithRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	r := gin.New()
	r.Use(RequestID(), RequestLogger(logger))
	r.GET("/boom", func(c *gin.Context) {
		internalError(c, errors.New("pq: relation \"secret_table\" does not exist"), "Failed to load")
	})
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest("GET", "/boom", http.NoBody)
	req.Header.Set("X-Request-ID", "req-123")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "secret_table", "the cause must not reach the client")
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INTERNAL_ERROR", body.Error.Code)
	assert.Equal(t, "req-123", body.Error.RequestID)

	var entry map[string]interface{}
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry))
	assert.Equal(t, "ERROR", entry["level"])
	assert.Equal(t, "req-123", entry["request_id"])
	assert.Contains(t, entry["error"], "secret_table", "the cause is logged server-side")

	logs.Reset()
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", http.NoBody))
	assert.Contains(t, logs.String(), `"level":"INFO"`)
}
