package drift

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// ModelVersionQuerier lists active model versions.
type ModelVersionQuerier interface {
	ListActiveVersionIDs(ctx context.Context) ([]uuid.UUID, error)
}

// Ticker periodically checks drift for active model versions.
type Ticker struct {
	detector *Detector
	versions ModelVersionQuerier
	interval time.Duration
	logger   *slog.Logger
	stopCh   chan struct{}
}

// NewTicker creates a new drift ticker.
func NewTicker(detector *Detector, versions ModelVersionQuerier, interval time.Duration, logger *slog.Logger) *Ticker {
	return &Ticker{detector: detector, versions: versions, interval: interval, logger: logger, stopCh: make(chan struct{})}
}

// Start begins the ticker loop.
func (t *Ticker) Start(ctx context.Context) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopCh:
			return
		case <-ticker.C:
			t.tick(ctx)
		}
	}
}

// Stop stops the ticker.
func (t *Ticker) Stop() {
	select {
	case <-t.stopCh:
	default:
		close(t.stopCh)
	}
}

func (t *Ticker) tick(ctx context.Context) {
	ids, err := t.versions.ListActiveVersionIDs(ctx)
	if err != nil {
		if t.logger != nil {
			t.logger.Error("drift ticker: failed to list versions", "error", err)
		}
		return
	}

	for _, id := range ids {
		results, err := t.detector.CheckDrift(ctx, id, 7*24*time.Hour, 24*time.Hour)
		if err != nil {
			if t.logger != nil {
				t.logger.Warn("drift check failed", "version_id", id, "error", err)
			}
			continue
		}
		for _, r := range results {
			if r.Severity != "NONE" && t.logger != nil {
				t.logger.Warn("drift detected", "version_id", id, "metric", r.Metric, "psi", r.PSIValue, "severity", r.Severity)
			}
		}
	}
}
