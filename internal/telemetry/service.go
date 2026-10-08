package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Service provides business logic for telemetry operations.
type Service struct {
	repo      *Repository
	publisher *Publisher
	logger    *slog.Logger
	dropped   atomic.Int64
}

// Dropped returns the number of events rejected at ingest because their
// model_version_id was missing or unknown.
func (s *Service) Dropped() int64 {
	return s.dropped.Load()
}

// NewService creates a new telemetry service.
func NewService(repo *Repository, publisher *Publisher, logger *slog.Logger) *Service {
	return &Service{repo: repo, publisher: publisher, logger: logger}
}

// IngestEvents validates and publishes telemetry events.
// Uses NATS async pipeline if available, falls back to direct DB insert.
func (s *Service) IngestEvents(ctx context.Context, deviceID uuid.UUID, events []Event) (accepted, rejected int, err error) {
	now := time.Now()
	maxFutureSkew := 5 * time.Minute

	var valid []Event
	for _, e := range events {
		// Validate
		if e.RecordedAt.After(now.Add(maxFutureSkew)) {
			rejected++
			continue
		}
		if e.LatencyMs < 0 {
			rejected++
			continue
		}
		if e.ModelVersionID == uuid.Nil {
			rejected++
			s.dropped.Add(1)
			s.warn("dropping telemetry event without model_version_id", "device_id", deviceID, "dropped_total", s.dropped.Load())
			continue
		}
		if e.Confidence < 0 {
			e.Confidence = 0
		}
		if e.Confidence > 1 {
			e.Confidence = 1
		}
		if e.RecordedAt.IsZero() {
			e.RecordedAt = now
		}
		if e.EventID == uuid.Nil {
			// From here on the ID is fixed, so a redelivery is still recognised.
			e.EventID = uuid.New()
		}

		e.DeviceID = deviceID
		valid = append(valid, e)
	}

	valid, unknown := s.dropUnknownVersions(ctx, deviceID, valid)
	rejected += unknown

	if len(valid) == 0 {
		return 0, rejected, nil
	}

	// The JetStream stream stores the batch before the device is answered.
	if s.publisher != nil {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := s.publisher.Publish(pctx, valid)
		cancel()
		if err == nil {
			s.logger.Debug("ingested telemetry events", "device_id", deviceID, "count", len(valid))
			return len(valid), rejected, nil
		}
		s.logger.Warn("JetStream publish failed, falling back to a direct insert", "error", err)
	}

	// Fallback: direct DB insert (idempotent, like the consumer's).
	if s.repo == nil {
		return 0, rejected, fmt.Errorf("no JetStream and no database repository available")
	}
	stored, err := s.repo.InsertEvents(ctx, valid)
	if err != nil {
		return 0, rejected, fmt.Errorf("direct insert failed: %w", err)
	}
	s.logger.Info("ingested telemetry events (direct)", "device_id", deviceID, "count", len(valid), "new", stored)
	return len(valid), rejected, nil
}

// dropUnknownVersions removes events whose model_version_id does not exist,
// so they never reach the batch insert (where one bad row aborts the batch).
// If the lookup itself fails, events pass through and the consumer's
// row-by-row fallback discards any offenders.
func (s *Service) dropUnknownVersions(ctx context.Context, deviceID uuid.UUID, events []Event) (kept []Event, dropped int) {
	if s.repo == nil || len(events) == 0 {
		return events, 0
	}
	seen := make(map[uuid.UUID]bool)
	var ids []uuid.UUID
	for _, e := range events {
		if !seen[e.ModelVersionID] {
			seen[e.ModelVersionID] = true
			ids = append(ids, e.ModelVersionID)
		}
	}
	known, err := s.repo.KnownModelVersions(ctx, deviceID, ids)
	if err != nil {
		s.warn("telemetry: model version lookup failed, deferring validation to consumer", "error", err)
		return events, 0
	}
	kept = events[:0]
	for _, e := range events {
		if !known[e.ModelVersionID] {
			dropped++
			continue
		}
		kept = append(kept, e)
	}
	if dropped > 0 {
		s.dropped.Add(int64(dropped))
		s.warn("dropping telemetry events for model versions outside the device's project",
			"device_id", deviceID, "count", dropped, "dropped_total", s.dropped.Load())
	}
	return kept, dropped
}

func (s *Service) warn(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Warn(msg, args...)
	}
}

// QueryEvents returns filtered telemetry events.
func (s *Service) QueryEvents(ctx context.Context, filter EventFilter) ([]Event, int, error) {
	if s.repo == nil {
		return nil, 0, fmt.Errorf("repository not available")
	}
	if filter.From.IsZero() {
		filter.From = time.Now().Add(-24 * time.Hour)
	}
	if filter.To.IsZero() {
		filter.To = time.Now()
	}
	return s.repo.QueryEvents(ctx, filter)
}

// GetSummary returns aggregated telemetry for a model version.
func (s *Service) GetSummary(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time) (*Summary, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("repository not available")
	}
	if from.IsZero() {
		from = time.Now().Add(-24 * time.Hour)
	}
	if to.IsZero() {
		to = time.Now()
	}
	return s.repo.GetSummary(ctx, modelVersionID, from, to)
}

// GetSummaryByDeviceClass returns per-device-class telemetry breakdown.
func (s *Service) GetSummaryByDeviceClass(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time) ([]Summary, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("repository not available")
	}
	if from.IsZero() {
		from = time.Now().Add(-24 * time.Hour)
	}
	if to.IsZero() {
		to = time.Now()
	}
	return s.repo.GetSummaryByDeviceClass(ctx, modelVersionID, from, to)
}

// ValidIntervals is the whitelist of allowed time bucket intervals.
var ValidIntervals = map[string]bool{"1m": true, "5m": true, "15m": true, "1h": true, "1d": true}

// GetTimeSeries returns time-bucketed latency data for charts.
func (s *Service) GetTimeSeries(ctx context.Context, modelVersionID uuid.UUID, from, to time.Time, interval string) ([]TimeSeriesPoint, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("repository not available")
	}
	if !ValidIntervals[interval] {
		return nil, fmt.Errorf("invalid interval %q: must be one of 1m, 5m, 15m, 1h, 1d", interval)
	}
	if from.IsZero() {
		from = time.Now().Add(-24 * time.Hour)
	}
	if to.IsZero() {
		to = time.Now()
	}
	return s.repo.GetTimeSeries(ctx, modelVersionID, from, to, interval)
}
