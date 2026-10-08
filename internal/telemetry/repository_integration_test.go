//go:build integration

package telemetry

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/dbtest"
)

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRepository_Integration(t *testing.T) {
	pool := dbtest.Pool(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	fx := dbtest.NewVersion(t, pool)
	dev := dbtest.NewDevice(t, pool, "")
	arm := uuid.New()
	now := time.Now()

	events := []Event{
		{DeviceID: dev, ModelVersionID: fx.VersionID, ExperimentArmID: &arm, EventType: "INFERENCE", LatencyMs: 10, Confidence: 0.9, RecordedAt: now.Add(-2 * time.Hour)},
		{DeviceID: dev, ModelVersionID: fx.VersionID, EventType: "INFERENCE", LatencyMs: 30, Confidence: 0.7, RecordedAt: now.Add(-time.Hour)},
		{DeviceID: dev, ModelVersionID: fx.VersionID, EventType: "ERROR", ErrorMessage: "boom", RecordedAt: now.Add(-time.Minute)},
	}
	n, err := repo.InsertEvents(ctx, events)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	require.NoError(t, repo.InsertEvent(ctx, Event{DeviceID: dev, ModelVersionID: fx.VersionID, EventType: "OOM", RecordedAt: now}))
	err = repo.InsertEvent(ctx, Event{DeviceID: dev, ModelVersionID: uuid.New(), EventType: "INFERENCE", RecordedAt: now})
	assert.Error(t, err, "unknown model version violates the foreign key")

	var tagged int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM telemetry_events WHERE experiment_arm_id = $1`, arm).Scan(&tagged))
	assert.Equal(t, 1, tagged, "the arm ID is stored")

	known, err := repo.KnownModelVersions(ctx, []uuid.UUID{fx.VersionID, uuid.New()})
	require.NoError(t, err)
	assert.Equal(t, map[uuid.UUID]bool{fx.VersionID: true}, known)

	got, total, err := repo.QueryEvents(ctx, EventFilter{ModelVersionID: &fx.VersionID, DeviceID: &dev, From: now.Add(-3 * time.Hour), To: now.Add(time.Minute)})
	require.NoError(t, err)
	assert.Equal(t, 4, total)
	assert.Len(t, got, 4)
	errs, _, err := repo.QueryEvents(ctx, EventFilter{ErrorOnly: true, EventType: "ERROR", From: now.Add(-3 * time.Hour), To: now.Add(time.Minute), Limit: 5000})
	require.NoError(t, err)
	assert.Len(t, errs, 1)

	sum, err := repo.GetSummary(ctx, fx.VersionID, now.Add(-3*time.Hour), now.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(4), sum.TotalEvents)
	assert.Equal(t, int64(1), sum.ErrorCount)
	assert.InDelta(t, 30, sum.MaxLatency, 0.01)

	byClass, err := repo.GetSummaryByDeviceClass(ctx, fx.VersionID, now.Add(-3*time.Hour), now.Add(time.Minute))
	require.NoError(t, err)
	require.Len(t, byClass, 1)
	assert.Equal(t, "unknown", byClass[0].DeviceClass)

	points, err := repo.GetTimeSeries(ctx, fx.VersionID, now.Add(-3*time.Hour), now.Add(time.Minute), "1h")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(points), 2)
	_, err = repo.GetTimeSeries(ctx, fx.VersionID, now.Add(-3*time.Hour), now, "bogus")
	require.NoError(t, err, "an unknown interval falls back to 1h")

	svc := NewService(repo, nil, discard())
	accepted, rejected, err := svc.IngestEvents(ctx, dev, []Event{
		{ModelVersionID: fx.VersionID, EventType: "INFERENCE", LatencyMs: 5},
		{ModelVersionID: uuid.New(), EventType: "INFERENCE", LatencyMs: 5},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, accepted)
	assert.Equal(t, 1, rejected, "unknown version dropped at ingest")
	_, _, err = svc.QueryEvents(ctx, EventFilter{})
	require.NoError(t, err)
	_, err = svc.GetSummary(ctx, fx.VersionID, time.Time{}, time.Time{})
	require.NoError(t, err)
	_, err = svc.GetSummaryByDeviceClass(ctx, fx.VersionID, time.Time{}, time.Time{})
	require.NoError(t, err)
	_, err = svc.GetTimeSeries(ctx, fx.VersionID, time.Time{}, time.Time{}, "5m")
	require.NoError(t, err)
}

// The NATS pipeline end to end: publish -> consumer -> hypertable, with one
// poison event that the row-by-row fallback discards.
func TestPipeline_Integration(t *testing.T) {
	natsURL := os.Getenv("FLEETML_TEST_NATS_URL")
	if natsURL == "" {
		t.Skip("FLEETML_TEST_NATS_URL not set")
	}
	pool := dbtest.Pool(t)
	repo := NewRepository(pool)
	fx := dbtest.NewVersion(t, pool)
	dev := dbtest.NewDevice(t, pool, "")

	nc, err := nats.Connect(natsURL)
	require.NoError(t, err)
	defer nc.Close()

	consumer := NewConsumer(nc, repo, discard())
	consumer.flushInterval = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = consumer.Start(ctx); close(done) }()
	time.Sleep(200 * time.Millisecond)

	pub := NewPublisher(nc, discard())
	require.True(t, pub.IsConnected())
	require.NoError(t, pub.Publish([]Event{
		{DeviceID: dev, ModelVersionID: fx.VersionID, EventType: "INFERENCE", LatencyMs: 7, RecordedAt: time.Now()},
		{DeviceID: dev, ModelVersionID: uuid.New(), EventType: "INFERENCE", LatencyMs: 7, RecordedAt: time.Now()},
	}))
	require.Eventually(t, func() bool {
		var n int
		_ = pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM telemetry_events WHERE device_id = $1`, dev).Scan(&n)
		return n == 1
	}, 5*time.Second, 100*time.Millisecond)
	assert.Equal(t, int64(1), consumer.Dropped())

	cancel()
	<-done
	require.NoError(t, consumer.Stop())
}
