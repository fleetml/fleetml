package rest

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/registry"
	"github.com/fleetml/fleetml/internal/telemetry"
)

// TelemetryHandler handles REST endpoints for telemetry queries.
type TelemetryHandler struct {
	service  *telemetry.Service
	versions versionLookup
	logger   *slog.Logger
}

// versionLookup finds a model version; *registry.Service implements it.
type versionLookup interface {
	GetVersion(ctx context.Context, id uuid.UUID) (*registry.RegistryVersion, error)
}

// requireVersion answers 404 NOT_FOUND when :model_version_id names no
// version, so a mistyped ID does not read as "no telemetry yet". With no
// lookup it lets every request through.
func requireVersion(versions versionLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		if versions == nil {
			return
		}
		id, ok := parseUUID(c, "model_version_id")
		if !ok {
			c.Abort()
			return
		}
		if _, err := versions.GetVersion(c.Request.Context(), id); err != nil {
			if errors.Is(err, registry.ErrNotFound) {
				respondError(c, http.StatusNotFound, "NOT_FOUND", "model version not found")
			} else {
				internalError(c, err, "An internal error occurred")
			}
			c.Abort()
		}
	}
}

// NewTelemetryHandler creates a new TelemetryHandler.
func NewTelemetryHandler(svc *telemetry.Service, logger *slog.Logger) *TelemetryHandler {
	return &TelemetryHandler{service: svc, logger: logger}
}

// WithVersions makes the per-version routes answer 404 for an unknown version.
func (h *TelemetryHandler) WithVersions(v versionLookup) *TelemetryHandler {
	h.versions = v
	return h
}

// RegisterRoutes registers telemetry routes.
func (h *TelemetryHandler) RegisterRoutes(rg *gin.RouterGroup) {
	known := requireVersion(h.versions)
	rg.GET("/telemetry/events", h.queryEvents)
	rg.GET("/telemetry/summary/:model_version_id", known, h.getSummary)
	rg.GET("/telemetry/summary/:model_version_id/by-class", known, h.getSummaryByClass)
	rg.GET("/telemetry/timeseries/:model_version_id", known, h.getTimeSeries)
}

func (h *TelemetryHandler) queryEvents(c *gin.Context) {
	filter := telemetry.EventFilter{}
	limit, offset := parsePagination(c)
	filter.Limit = limit
	filter.Offset = offset

	if v := c.Query("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = t
		}
	}
	if v := c.Query("device_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.DeviceID = &id
		}
	}
	if v := c.Query("model_version_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.ModelVersionID = &id
		}
	}
	if v := c.Query("event_type"); v != "" {
		filter.EventType = v
	}
	if c.Query("error_only") == "true" {
		filter.ErrorOnly = true
	}

	events, total, err := h.service.QueryEvents(c.Request.Context(), filter)
	if err != nil {
		internalError(c, err, "Failed to query events")
		return
	}
	if events == nil {
		events = []telemetry.Event{}
	}
	respondList(c, events, total, limit, offset)
}

func (h *TelemetryHandler) getSummary(c *gin.Context) {
	id, ok := parseUUID(c, "model_version_id")
	if !ok {
		return
	}

	from, to := parseTimeRange(c)
	summary, err := h.service.GetSummary(c.Request.Context(), id, from, to)
	if err != nil {
		internalError(c, err, "Failed to get summary")
		return
	}
	respondData(c, http.StatusOK, summary)
}

func (h *TelemetryHandler) getSummaryByClass(c *gin.Context) {
	id, ok := parseUUID(c, "model_version_id")
	if !ok {
		return
	}

	from, to := parseTimeRange(c)
	summaries, err := h.service.GetSummaryByDeviceClass(c.Request.Context(), id, from, to)
	if err != nil {
		internalError(c, err, "Failed to get summary by class")
		return
	}
	if summaries == nil {
		summaries = []telemetry.Summary{}
	}
	respondData(c, http.StatusOK, summaries)
}

func (h *TelemetryHandler) getTimeSeries(c *gin.Context) {
	id, ok := parseUUID(c, "model_version_id")
	if !ok {
		return
	}

	from, to := parseTimeRange(c)
	interval := c.DefaultQuery("interval", "1h")
	if !telemetry.ValidIntervals[interval] {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "interval must be one of: 1m, 5m, 15m, 1h, 1d")
		return
	}

	points, err := h.service.GetTimeSeries(c.Request.Context(), id, from, to, interval)
	if err != nil {
		internalError(c, err, "Failed to get time series")
		return
	}
	if points == nil {
		points = []telemetry.TimeSeriesPoint{}
	}
	respondData(c, http.StatusOK, points)
}

func parseTimeRange(c *gin.Context) (from, to time.Time) {
	from = time.Now().Add(-24 * time.Hour)
	to = time.Now()
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	return from, to
}
