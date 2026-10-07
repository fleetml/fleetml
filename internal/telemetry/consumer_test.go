package telemetry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWriter mimics Postgres: a batch COPY fails entirely if any row references
// an unknown model version; single-row inserts fail only for the bad row.
type fakeWriter struct {
	mu        sync.Mutex
	known     map[uuid.UUID]bool
	stored    []Event
	transient bool // when true, single-row inserts fail with a non-Postgres error
	ctxErrs   []error
}

func (f *fakeWriter) InsertEvents(ctx context.Context, events []Event) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxErrs = append(f.ctxErrs, ctx.Err())
	for _, e := range events {
		if !f.known[e.ModelVersionID] {
			return 0, errors.New("telemetry.InsertEvents: ERROR: violates foreign key constraint (SQLSTATE 23503)")
		}
	}
	f.stored = append(f.stored, events...)
	return len(events), nil
}

func (f *fakeWriter) InsertEvent(ctx context.Context, e Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctxErrs = append(f.ctxErrs, ctx.Err())
	if f.transient {
		return errors.New("connection refused")
	}
	if !f.known[e.ModelVersionID] {
		return &pgconn.PgError{Code: "23503", Message: "violates foreign key constraint"}
	}
	f.stored = append(f.stored, e)
	return nil
}

func newTestConsumer(w eventWriter) *Consumer {
	return &Consumer{
		repo:          w,
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		batchSize:     defaultBatchSize,
		flushInterval: defaultFlushInterval,
	}
}

func goodEvents(versionID uuid.UUID, n int) []Event {
	out := make([]Event, n)
	for i := range out {
		out[i] = Event{DeviceID: uuid.New(), ModelVersionID: versionID, EventType: "INFERENCE", LatencyMs: 10 + i, RecordedAt: time.Now()}
	}
	return out
}

// Regression: one bad event used to fail the CopyFrom, get re-buffered, and
// block every later event forever.
func TestConsumer_PoisonEventDoesNotStallIngestion(t *testing.T) {
	version := uuid.New()
	w := &fakeWriter{known: map[uuid.UUID]bool{version: true}}
	c := newTestConsumer(w)
	ctx := context.Background()

	unknownVersion := Event{DeviceID: uuid.New(), ModelVersionID: uuid.New(), EventType: "INFERENCE", RecordedAt: time.Now()}
	nilVersion := Event{DeviceID: uuid.New(), EventType: "INFERENCE", RecordedAt: time.Now()}

	c.addToBuffer(ctx, []Event{unknownVersion, nilVersion})
	c.addToBuffer(ctx, goodEvents(version, 3))
	c.flush(ctx)

	assert.Len(t, w.stored, 3, "good events must land despite the bad ones")
	assert.Equal(t, int64(2), c.Dropped())
	assert.Empty(t, c.buffer, "nothing should be re-buffered for permanent failures")

	// Later events keep flowing.
	c.addToBuffer(ctx, goodEvents(version, 2))
	c.flush(ctx)
	assert.Len(t, w.stored, 5)
	assert.Equal(t, int64(2), c.Dropped())
}

func TestConsumer_TransientFailureIsRequeued(t *testing.T) {
	version := uuid.New()
	w := &fakeWriter{known: map[uuid.UUID]bool{}, transient: true}
	c := newTestConsumer(w)
	ctx := context.Background()

	c.addToBuffer(ctx, goodEvents(version, 3))
	c.flush(ctx)

	assert.Empty(t, w.stored)
	assert.Len(t, c.buffer, 3, "non-Postgres errors are retried on the next flush")
	assert.Equal(t, int64(0), c.Dropped())
}

func TestConsumer_FinalFlushUsesLiveContext(t *testing.T) {
	version := uuid.New()
	w := &fakeWriter{known: map[uuid.UUID]bool{version: true}}
	c := newTestConsumer(w)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	c.addToBuffer(cancelled, goodEvents(version, 2))
	c.finalFlush()

	require.Len(t, w.stored, 2)
	for _, err := range w.ctxErrs {
		assert.NoError(t, err, "final flush must not run on the cancelled context")
	}
}

func TestIngestEvents_DropsNilModelVersion(t *testing.T) {
	svc := &Service{}
	accepted, rejected, err := svc.IngestEvents(context.Background(), uuid.New(), []Event{
		{EventType: "INFERENCE", LatencyMs: 5, RecordedAt: time.Now()},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, accepted)
	assert.Equal(t, 1, rejected)
	assert.Equal(t, int64(1), svc.Dropped())
}
