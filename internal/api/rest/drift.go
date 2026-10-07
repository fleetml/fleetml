package rest

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/drift"
)

// DriftHandler handles REST endpoints for drift detection.
type DriftHandler struct {
	detector *drift.Detector
	store    drift.ResultStore
	logger   *slog.Logger
}

// NewDriftHandler creates a new DriftHandler.
func NewDriftHandler(detector *drift.Detector, store drift.ResultStore, logger *slog.Logger) *DriftHandler {
	return &DriftHandler{detector: detector, store: store, logger: logger}
}

// RegisterRoutes registers drift routes.
func (h *DriftHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/drift/:model_version_id", h.getLatest)
	rg.POST("/drift/:model_version_id/check", h.check)
	rg.GET("/drift/:model_version_id/history", h.getHistory)
}

func (h *DriftHandler) getLatest(c *gin.Context) {
	id, ok := parseUUID(c, "model_version_id")
	if !ok {
		return
	}
	results, err := h.store.GetLatestResults(c.Request.Context(), id)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get drift results")
		return
	}
	if results == nil {
		results = []drift.DriftResult{}
	}
	respondData(c, http.StatusOK, results)
}

func (h *DriftHandler) check(c *gin.Context) {
	id, ok := parseUUID(c, "model_version_id")
	if !ok {
		return
	}
	results, err := h.detector.CheckDrift(c.Request.Context(), id, 7*24*time.Hour, 24*time.Hour)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if results == nil {
		results = []drift.DriftResult{}
	}
	respondData(c, http.StatusOK, results)
}

func (h *DriftHandler) getHistory(c *gin.Context) {
	id, ok := parseUUID(c, "model_version_id")
	if !ok {
		return
	}
	results, err := h.store.GetHistory(c.Request.Context(), id, 50)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get drift history")
		return
	}
	if results == nil {
		results = []drift.DriftResult{}
	}
	respondData(c, http.StatusOK, results)
}
