package rest

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/drift"
	"github.com/fleetml/fleetml/internal/features"
)

// DriftHandler handles REST endpoints for drift detection.
type DriftHandler struct {
	detector *drift.Detector
	store    drift.ResultStore
	features *drift.Adapter
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
	if a, ok := h.store.(*drift.Adapter); ok {
		h.features = a
		rg.GET("/drift/:model_version_id/points", h.getPoints)
		rg.GET("/models/:model_id/features", h.getFeatures)
		rg.PUT("/models/:model_id/features", h.putFeatures)
	}
}

// getPoints returns every check's PSI since ?days= (default 7, at most 90).
func (h *DriftHandler) getPoints(c *gin.Context) {
	id, ok := parseUUID(c, "model_version_id")
	if !ok {
		return
	}
	days := 7
	if v := c.Query("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 90 {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "days must be between 1 and 90")
			return
		}
		days = n
	}
	points, err := h.features.Points(c.Request.Context(), id, time.Now().AddDate(0, 0, -days))
	if err != nil {
		internalError(c, err, "Failed to get drift points")
		return
	}
	respondData(c, http.StatusOK, points)
}

func (h *DriftHandler) getFeatures(c *gin.Context) {
	id, ok := parseUUID(c, "model_id")
	if !ok {
		return
	}
	specs, err := h.features.ModelFeatures(c.Request.Context(), id)
	if err != nil {
		internalError(c, err, "Failed to get features")
		return
	}
	respondData(c, http.StatusOK, specs)
}

type putFeaturesRequest struct {
	Features []drift.FeatureSpec `json:"features"`
}

// putFeatures replaces the model's features. Devices bin each feature's
// values with its edges and report the histograms with their inferences.
func (h *DriftHandler) putFeatures(c *gin.Context) {
	id, ok := parseUUID(c, "model_id")
	if !ok {
		return
	}
	var req putFeaturesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "body must be {\"features\": [{\"name\": ..., \"edges\": [...]}]}")
		return
	}
	seen := map[string]bool{}
	for _, f := range req.Features {
		if err := features.Validate(f.Name, f.Edges); err != nil {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		if seen[f.Name] {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "feature "+f.Name+" is listed twice")
			return
		}
		seen[f.Name] = true
	}
	if len(req.Features) > 100 {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "at most 100 features per model")
		return
	}
	if err := h.features.SetModelFeatures(c.Request.Context(), id, req.Features); err != nil {
		if errors.Is(err, drift.ErrModelNotFound) {
			respondError(c, http.StatusNotFound, "NOT_FOUND", "model not found")
			return
		}
		internalError(c, err, "Failed to save features")
		return
	}
	if req.Features == nil {
		req.Features = []drift.FeatureSpec{}
	}
	respondData(c, http.StatusOK, req.Features)
}

func (h *DriftHandler) getLatest(c *gin.Context) {
	id, ok := parseUUID(c, "model_version_id")
	if !ok {
		return
	}
	results, err := h.store.GetLatestResults(c.Request.Context(), id)
	if err != nil {
		internalError(c, err, "Failed to get drift results")
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
		internalError(c, err, "Drift check failed")
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
		internalError(c, err, "Failed to get drift history")
		return
	}
	if results == nil {
		results = []drift.DriftResult{}
	}
	respondData(c, http.StatusOK, results)
}
