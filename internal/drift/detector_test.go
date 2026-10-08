package drift

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/alert"
)

// fakeTelemetry returns a baseline window and a current window of values.
type fakeTelemetry struct{ baseline, current []float64 }

func (f *fakeTelemetry) values(to time.Time) []float64 {
	if time.Since(to) > time.Hour {
		return f.baseline
	}
	return f.current
}

func (f *fakeTelemetry) GetLatencyValues(_ context.Context, _ uuid.UUID, _, to time.Time, _ int) ([]float64, error) {
	return f.values(to), nil
}

func (f *fakeTelemetry) GetConfidenceValues(context.Context, uuid.UUID, time.Time, time.Time, int) ([]float64, error) {
	return nil, nil
}

type memStore struct{ rows []DriftResult }

func (m *memStore) SaveDriftResult(_ context.Context, r *DriftResult) error {
	r.ID = uuid.New()
	m.rows = append(m.rows, *r)
	return nil
}
func (m *memStore) GetLatestResults(context.Context, uuid.UUID) ([]DriftResult, error) {
	return nil, nil
}
func (m *memStore) GetHistory(context.Context, uuid.UUID, int) ([]DriftResult, error) {
	return m.rows, nil
}
func (m *memStore) LatestResult(_ context.Context, v uuid.UUID, metric string) (*DriftResult, error) {
	for i := len(m.rows) - 1; i >= 0; i-- {
		if m.rows[i].ModelVersionID == v && m.rows[i].Metric == metric {
			r := m.rows[i]
			return &r, nil
		}
	}
	return nil, nil
}
func (m *memStore) RefreshDriftResult(_ context.Context, id uuid.UUID, r *DriftResult) error {
	for i := range m.rows {
		if m.rows[i].ID == id {
			m.rows[i].PSIValue, m.rows[i].CheckedAt = r.PSIValue, r.CheckedAt
		}
	}
	return nil
}

type fakeAlerts struct{ sent []alert.Alert }

func (f *fakeAlerts) Send(a alert.Alert) { f.sent = append(f.sent, a) }

func normal(mean float64, rng *rand.Rand) []float64 {
	out := make([]float64, 500)
	for i := range out {
		out[i] = mean + rng.NormFloat64()*5
	}
	return out
}

// An unchanged drift state is stored once and alerted once, however often it
// is checked; a change of severity writes a new row.
func TestDetector_DedupAndAlertOnTransition(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	tel := &fakeTelemetry{baseline: normal(50, rng), current: normal(50, rng)}
	store := &memStore{}
	alerts := &fakeAlerts{}
	d := NewDetector(tel, store, nil)
	d.SetAlertSender(alerts)
	v := uuid.New()

	_, err := d.CheckDrift(context.Background(), v, 7*24*time.Hour, 24*time.Hour)
	require.NoError(t, err)
	require.Len(t, store.rows, 1)
	assert.Equal(t, "NONE", store.rows[0].Severity)
	assert.Empty(t, alerts.sent)

	tel.current = normal(120, rng) // latency shifted: CRITICAL
	for i := 0; i < 5; i++ {
		res, err := d.CheckDrift(context.Background(), v, 7*24*time.Hour, 24*time.Hour)
		require.NoError(t, err)
		require.Equal(t, "CRITICAL", res[0].Severity)
	}
	assert.Len(t, store.rows, 2, "five CRITICAL checks store one CRITICAL row")
	require.Len(t, alerts.sent, 1, "and send one alert")
	a := alerts.sent[0]
	assert.Equal(t, alert.TypeDrift, a.Type)
	assert.Equal(t, "CRITICAL", a.Drift.Severity)
	assert.Equal(t, "NONE", a.Drift.PreviousSeverity)
	assert.Equal(t, "latency", a.Drift.Metric)

	tel.current = normal(50, rng) // recovered
	_, err = d.CheckDrift(context.Background(), v, 7*24*time.Hour, 24*time.Hour)
	require.NoError(t, err)
	assert.Len(t, store.rows, 3)
	assert.Len(t, alerts.sent, 1, "recovery is recorded but not alerted")
}
