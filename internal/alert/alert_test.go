package alert

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeNATS struct {
	mu   sync.Mutex
	msgs map[string][][]byte
}

func (f *fakeNATS) Publish(subject string, data []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.msgs == nil {
		f.msgs = map[string][][]byte{}
	}
	f.msgs[subject] = append(f.msgs[subject], data)
	return nil
}

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSend_NATSAndSignedWebhook(t *testing.T) {
	var got []*http.Request
	var bodies [][]byte
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got, bodies = append(got, r), append(bodies, b)
		mu.Unlock()
	}))
	defer srv.Close()

	nc := &fakeNATS{}
	n := NewNotifier(nc, srv.URL, "s3cret", quietLogger())
	n.Send(Alert{Type: TypeRollback, Rollback: &Rollback{DeploymentID: "d1", Trigger: "health_gate", Reason: "error rate"}})
	n.Wait()
	mu.Lock()
	defer mu.Unlock()

	require.Len(t, nc.msgs[SubjectPrefix+TypeRollback], 1)
	require.Len(t, got, 1)
	assert.Equal(t, nc.msgs[SubjectPrefix+TypeRollback][0], bodies[0], "NATS and webhook carry the same JSON")
	assert.Equal(t, TypeRollback, got[0].Header.Get(HeaderEvent))
	assert.Equal(t, Sign([]byte("s3cret"), bodies[0]), got[0].Header.Get(HeaderSignature))

	var a Alert
	require.NoError(t, json.Unmarshal(bodies[0], &a))
	assert.NotEmpty(t, a.ID)
	assert.Equal(t, a.ID, got[0].Header.Get(HeaderDelivery))
	assert.Equal(t, "d1", a.Rollback.DeploymentID)
	assert.False(t, a.OccurredAt.IsZero())
}

func TestSend_RetriesThreeTimesThenGivesUp(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	n := NewNotifier(nil, srv.URL, "", quietLogger())
	n.backoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	n.Send(Alert{Type: TypeDrift, Drift: &Drift{Severity: "CRITICAL"}})
	n.Wait()
	assert.Equal(t, int32(4), calls.Load(), "one attempt plus three retries")
}

func TestSend_RetrySucceeds(t *testing.T) {
	var calls atomic.Int32
	var mu sync.Mutex
	var ids []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ids = append(ids, r.Header.Get(HeaderDelivery))
		mu.Unlock()
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()

	n := NewNotifier(nil, srv.URL, "k", quietLogger())
	n.backoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	n.Send(Alert{Type: TypeDrift})
	n.Wait()
	assert.Equal(t, int32(3), calls.Load())
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, ids[0], ids[2], "retries keep the delivery ID so receivers can dedupe")
}

func TestSend_NoWebhookConfigured(t *testing.T) {
	nc := &fakeNATS{}
	n := NewNotifier(nc, "", "", quietLogger())
	n.Send(Alert{Type: TypeDrift})
	n.Wait()
	assert.Len(t, nc.msgs[SubjectPrefix+TypeDrift], 1)
}
