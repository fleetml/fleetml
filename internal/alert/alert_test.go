package alert

import (
	"context"
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

func TestSend_FiveAttemptsThenDeadLetter(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	n := NewNotifier(nil, "", "", quietLogger())
	n.backoff = fastBackoff()
	store := &fakeStore{sinks: []SinkConfig{{ID: "s1", ProjectID: "p1", Name: "ops", Kind: KindWebhook, Enabled: true, Config: map[string]string{"url": srv.URL}}}}
	n.SetStore(store)
	n.Send(Alert{Type: TypeRollback, Rollback: &Rollback{ProjectID: "p1"}})
	n.Wait()
	assert.Equal(t, int32(5), calls.Load(), "one attempt plus four retries")
	require.Len(t, store.deliveries, 5)
	for i, d := range store.deliveries[:4] {
		assert.Equal(t, i+1, d.Attempt)
		assert.Equal(t, StatusError, d.Status)
		assert.Equal(t, http.StatusServiceUnavailable, d.HTTPStatus)
	}
	assert.Equal(t, StatusDead, store.deliveries[4].Status, "the last failure is the dead letter")
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
	n.backoff = fastBackoff()
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

func fastBackoff() []time.Duration {
	return []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond}
}

type fakeStore struct {
	mu         sync.Mutex
	sinks      []SinkConfig
	deliveries []Delivery
}

func (f *fakeStore) EnabledSinks(_ context.Context, projectID string) ([]SinkConfig, error) {
	var out []SinkConfig
	for _, s := range f.sinks {
		if s.ProjectID == projectID && s.Enabled {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeStore) RecordDelivery(_ context.Context, d Delivery) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deliveries = append(f.deliveries, d)
	return nil
}

type fakeLookup struct{}

func (fakeLookup) DescribeVersion(_ context.Context, _ string) (projectID, modelName, version string, err error) {
	return "p1", "detector", "2.0.0", nil
}

// A drift alert carries no project: the lookup finds it, and only that
// project's enabled sinks receive the alert, Slack as a Block Kit message.
func TestSend_ProjectSinksAndSlack(t *testing.T) {
	var mu sync.Mutex
	got := map[string][][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got[r.URL.Path] = append(got[r.URL.Path], b)
		mu.Unlock()
	}))
	defer srv.Close()

	store := &fakeStore{sinks: []SinkConfig{
		{ID: "hook", ProjectID: "p1", Name: "hook", Kind: KindWebhook, Enabled: true, Config: map[string]string{"url": srv.URL + "/hook"}},
		{ID: "slack", ProjectID: "p1", Name: "slack", Kind: KindSlack, Enabled: true, Config: map[string]string{"webhook_url": srv.URL + "/slack"}},
		{ID: "off", ProjectID: "p1", Name: "off", Kind: KindWebhook, Enabled: false, Config: map[string]string{"url": srv.URL + "/off"}},
		{ID: "other", ProjectID: "p2", Name: "other", Kind: KindWebhook, Enabled: true, Config: map[string]string{"url": srv.URL + "/other"}},
	}}
	n := NewNotifier(nil, "", "", quietLogger())
	n.SetStore(store)
	n.SetLookup(fakeLookup{})
	n.SetEnv(SinkEnv{DashboardURL: "https://fleet.example.com/"})
	n.Send(Alert{Type: TypeDrift, Drift: &Drift{ModelVersionID: "v2", Metric: "confidence", PSI: 0.31, PreviousSeverity: "WARNING"}})
	n.Wait()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got["/hook"], 1)
	require.Len(t, got["/slack"], 1)
	assert.Empty(t, got["/off"])
	assert.Empty(t, got["/other"])

	var a Alert
	require.NoError(t, json.Unmarshal(got["/hook"][0], &a))
	assert.Equal(t, "p1", a.ProjectID)
	assert.Equal(t, &ModelRef{Name: "detector", Version: "2.0.0"}, a.Model)

	slack := string(got["/slack"][0])
	assert.Contains(t, slack, `"type":"header"`)
	assert.Contains(t, slack, "Drift CRITICAL: detector 2.0.0 confidence")
	assert.Contains(t, slack, "<https://fleet.example.com/drift?version=v2|Open in the FleetML dashboard>")
	assert.Len(t, store.deliveries, 2)
}

func TestSendTest_OneAttemptRecorded(t *testing.T) {
	n := NewNotifier(nil, "", "", quietLogger())
	store := &fakeStore{}
	n.SetStore(store)
	d := n.SendTest(context.Background(), SinkConfig{ID: "s1", Kind: KindWebhook, Config: map[string]string{"url": "http://127.0.0.1:1/"}})
	assert.Equal(t, StatusError, d.Status)
	assert.NotEmpty(t, d.Error)
	require.Len(t, store.deliveries, 1)
	assert.Equal(t, TypeTest, store.deliveries[0].AlertType)
}

func TestValidateAndRedact(t *testing.T) {
	assert.NoError(t, ValidateSink(KindWebhook, map[string]string{"url": "https://x.example/h"}, false))
	assert.Error(t, ValidateSink(KindWebhook, map[string]string{"url": "ftp://x"}, false))
	assert.Error(t, ValidateSink(KindSlack, map[string]string{}, false))
	assert.Error(t, ValidateSink(KindEmail, map[string]string{"to": "a@b.c"}, false), "email needs SMTP")
	assert.NoError(t, ValidateSink(KindEmail, map[string]string{"to": "a@b.c, d@e.f"}, true))
	assert.Error(t, ValidateSink(KindEmail, map[string]string{"to": "nobody"}, true))
	assert.Error(t, ValidateSink("pager", nil, true))

	r := RedactedConfig(KindSlack, map[string]string{"webhook_url": "https://hooks.slack.com/services/T0/B0/xyz"})
	assert.Equal(t, "https://hooks.slack.com/…", r["webhook_url"])
	r = RedactedConfig(KindWebhook, map[string]string{"url": "https://x.example/h", "secret": "k"})
	assert.Equal(t, "https://x.example/h", r["url"])
	assert.Equal(t, "********", r["secret"])
}
