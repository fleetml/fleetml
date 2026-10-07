package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Service provides business logic for telemetry operations.
type Service struct {
	repo      *Repository
	publisher *Publisher
	logger    *slog.Logger
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
		if e.Confidence < 0 {
			e.Confidence = 0
		}
		if e.Confidence > 1 {
			e.Confidence = 1
		}
		if e.RecordedAt.IsZero() {
			e.RecordedAt = now
		}

		e.DeviceID = deviceID
		valid = append(valid, e)
	}

	if len(valid) == 0 {
		return 0, rejected, nil
	}

	// Try NATS async pipeline
	if s.publisher != nil && s.publisher.IsConnected() {
		if err := s.publisher.Publish(valid); err != nil {
			s.logger.Warn("NATS publish failed, falling back to direct insert", "error", err)
		} else {
			s.logger.Info("ingested telemetry events", "device_id", deviceID, "count", len(valid))
			return len(valid), rejected, nil
		}
	}

	// Fallback: direct DB insert
	if s.repo == nil {
		return 0, rejected, fmt.Errorf("no NATS connection and no database repository available")
	}
	count, err := s.repo.InsertEvents(ctx, valid)
	if err != nil {
		return 0, rejected, fmt.Errorf("direct insert failed: %w", err)
	}
	s.logger.Info("ingested telemetry events (direct)", "device_id", deviceID, "count", count)
	return count, rejected, nil
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
