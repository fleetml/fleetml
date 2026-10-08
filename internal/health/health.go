// Package health reports whether the server's dependencies are reachable. The
// REST /readyz endpoint and the gRPC health service share one Checker.
package health

import (
	"context"
	"log/slog"
	"time"
)

// Checker runs named dependency checks.
type Checker struct {
	names  []string
	checks []func(context.Context) error
	logger *slog.Logger
}

// NewChecker returns an empty Checker.
func NewChecker(logger *slog.Logger) *Checker { return &Checker{logger: logger} }

// Add registers a dependency check.
func (c *Checker) Add(name string, check func(context.Context) error) {
	c.names = append(c.names, name)
	c.checks = append(c.checks, check)
}

// Check runs every check (2 s timeout each) and returns each dependency's
// status, "connected" or "disconnected", and whether all of them passed.
func (c *Checker) Check(ctx context.Context) (map[string]string, bool) {
	statuses := make(map[string]string, len(c.names))
	ok := true
	for i, name := range c.names {
		cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := c.checks[i](cctx)
		cancel()
		if err != nil {
			ok = false
			statuses[name] = "disconnected"
			if c.logger != nil {
				c.logger.Warn("readiness check failed", "dependency", name, "error", err)
			}
			continue
		}
		statuses[name] = "connected"
	}
	return statuses, ok
}
