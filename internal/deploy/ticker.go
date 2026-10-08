package deploy

import (
	"context"
	"log/slog"
	"time"

	"github.com/fleetml/fleetml/internal/metrics"
)

// Ticker periodically checks active deployments and progresses them.
type Ticker struct {
	engine   *Engine
	repo     Repository
	interval time.Duration
	logger   *slog.Logger
	stopCh   chan struct{}
}

// NewTicker creates a new deployment ticker.
func NewTicker(engine *Engine, repo Repository, interval time.Duration, logger *slog.Logger) *Ticker {
	return &Ticker{
		engine:   engine,
		repo:     repo,
		interval: interval,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
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
	defer metrics.ObserveTick("deploy", time.Now())
	deployments, err := t.repo.ListByStatus(ctx, []string{"ROLLING_OUT"})
	if err != nil {
		if t.logger != nil {
			t.logger.Error("ticker: failed to list active deployments", "error", err)
		}
		return
	}

	for _, dep := range deployments {
		if _, err := t.engine.ProgressDeployment(ctx, dep.ID); err != nil {
			if t.logger != nil {
				t.logger.Warn("ticker: failed to progress deployment", "deployment_id", dep.ID, "error", err)
			}
		}
	}
}
