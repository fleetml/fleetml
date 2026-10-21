package agent

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fleetmlv1 "github.com/fleetml/fleetml/gen/fleetml/v1"
)

// POST /v1/events says why an event was rejected, in the body and (the
// first reason) in the agent's log.
func TestLocalEventsRejectedReasons(t *testing.T) {
	var logs bytes.Buffer
	a, err := New(&Config{Server: "127.0.0.1:1", StateDir: t.TempDir()}, slog.New(slog.NewTextHandler(&logs, nil)))
	require.NoError(t, err)
	defer func() { _ = a.Close() }()
	post := func(body string) string {
		rec := httptest.NewRecorder()
		a.handleEvents(rec, httptest.NewRequest(http.MethodPost, "/v1/events", strings.NewReader(body)))
		require.Equal(t, http.StatusAccepted, rec.Code)
		return rec.Body.String()
	}

	assert.JSONEq(t, `{"accepted":0,"rejected":1,"rejected_reasons":["event 0: unknown model \"iris\" (no model is active yet)"]}`,
		post(`{"model":"iris","latency_ms":7}`))

	model := []byte("model")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(model) }))
	defer srv.Close()
	a.apply(context.Background(), []*fleetmlv1.ModelAssignment{{ModelId: "m1", ModelName: "iris", Version: "1.0.0", VersionId: "ver-1",
		VariantId: "var-1", DownloadUrl: srv.URL, BlobSha256: shaHex(model), Action: fleetmlv1.ModelAssignment_LOAD}})
	assert.JSONEq(t, `{"accepted":1,"rejected":1,"rejected_reasons":["event 1: unknown model \"foo\" (active models: iris)"]}`,
		post(`[{"model":"iris","latency_ms":7},{"model":"foo"}]`))
	assert.JSONEq(t, `{"accepted":1,"rejected":0}`, post(`{"model":"iris","latency_ms":7}`), "no reasons when nothing is rejected")
	assert.Contains(t, logs.String(), `level=INFO msg="local telemetry events rejected" rejected=1 first="event 1: unknown model \"foo\" (active models: iris)"`)
}
