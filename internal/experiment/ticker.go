package experiment

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// Ticker periodically evaluates running experiments.
type Ticker struct {
	service  *Service
	interval time.Duration
	logger   *slog.Logger
	stopCh   chan struct{}
}

// NewTicker creates a new experiment ticker.
func NewTicker(service *Service, interval time.Duration, logger *slog.Logger) *Ticker {
	return &Ticker{service: service, interval: interval, logger: logger, stopCh: make(chan struct{})}
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
	exps, err := t.service.ListRunning(ctx)
	if err != nil {
		if t.logger != nil {
			t.logger.Error("experiment ticker: failed to list", "error", err)
		}
		return
	}

	for _, exp := range exps {
		result, err := t.service.EvaluateExperiment(ctx, exp.ID)
		if err != nil {
			if t.logger != nil {
				t.logger.Warn("experiment ticker: evaluation failed", "experiment_id", exp.ID, "error", err)
			}
			continue
		}

		// Auto-conclude if configured and significant
		if exp.AutoConclude && result.IsSignificant &&
			result.ControlStats.SampleSize >= int64(exp.MinSampleSize) &&
			result.TreatmentStats.SampleSize >= int64(exp.MinSampleSize) {
			var winner uuid.UUID
			if result.Recommendation == "ADOPT_TREATMENT" {
				winner = exp.TreatmentVersionID
			} else {
				winner = exp.ControlVersionID
			}
			if _, err := t.service.Conclude(ctx, exp.ID, winner, "auto-concluded: "+result.Recommendation); err != nil {
				if t.logger != nil {
					t.logger.Warn("experiment ticker: auto-conclude failed", "experiment_id", exp.ID, "error", err)
				}
			} else if t.logger != nil {
				t.logger.Info("experiment auto-concluded", "experiment_id", exp.ID, "winner", winner, "recommendation", result.Recommendation)
			}
		}
	}
}
