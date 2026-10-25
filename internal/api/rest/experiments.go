package rest

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/experiment"
)

// ExperimentHandler handles REST endpoints for experiments.
type ExperimentHandler struct {
	service *experiment.Service
	logger  *slog.Logger
}

// NewExperimentHandler creates a new ExperimentHandler.
func NewExperimentHandler(svc *experiment.Service, logger *slog.Logger) *ExperimentHandler {
	return &ExperimentHandler{service: svc, logger: logger}
}

// RegisterRoutes registers experiment routes.
func (h *ExperimentHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/experiments", h.create)
	rg.GET("/experiments", h.list)
	rg.GET("/experiments/:experiment_id", h.get)
	rg.DELETE("/experiments/:experiment_id", h.delete)
	rg.POST("/experiments/:experiment_id/start", h.start)
	rg.POST("/experiments/:experiment_id/pause", h.pause)
	rg.POST("/experiments/:experiment_id/resume", h.resume)
	rg.POST("/experiments/:experiment_id/conclude", h.conclude)
	rg.POST("/experiments/:experiment_id/cancel", h.cancel)
	rg.GET("/experiments/:experiment_id/results", h.results)
	rg.GET("/experiments/:experiment_id/cohorts", h.cohorts)
}

type createExperimentRequest struct {
	ProjectID          uuid.UUID         `json:"project_id" binding:"required"`
	Name               string            `json:"name" binding:"required"`
	Description        string            `json:"description"`
	ControlVersionID   uuid.UUID         `json:"control_version_id" binding:"required"`
	TreatmentVersionID uuid.UUID         `json:"treatment_version_id" binding:"required"`
	TrafficPercent     int               `json:"traffic_percent"`
	SplitRatio         int               `json:"split_ratio"`
	TargetSelector     map[string]string `json:"target_selector"`
	PrimaryMetric      string            `json:"primary_metric"`
	MinSampleSize      int               `json:"min_sample_size"`
	ConfidenceLevel    float64           `json:"confidence_level"`
	AutoConclude       bool              `json:"auto_conclude"`
	MaxDuration        string            `json:"max_duration"`
	CreatedBy          string            `json:"created_by"`
}

func (h *ExperimentHandler) create(c *gin.Context) {
	var req createExperimentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "project_id, name, control_version_id, and treatment_version_id are required")
		return
	}
	exp, err := h.service.Create(c.Request.Context(), experiment.CreateInput{
		ProjectID: req.ProjectID, Name: req.Name, Description: req.Description,
		ControlVersionID: req.ControlVersionID, TreatmentVersionID: req.TreatmentVersionID,
		TrafficPercent: req.TrafficPercent, SplitRatio: req.SplitRatio,
		TargetSelector: req.TargetSelector, PrimaryMetric: req.PrimaryMetric,
		MinSampleSize: req.MinSampleSize, ConfidenceLevel: req.ConfidenceLevel,
		AutoConclude: req.AutoConclude, MaxDuration: req.MaxDuration, CreatedBy: req.CreatedBy,
	})
	if err != nil {
		handleExpError(c, err)
		return
	}
	respondData(c, http.StatusCreated, exp)
}

func (h *ExperimentHandler) list(c *gin.Context) {
	limit, offset, ok := parsePagination(c, maxPageSize)
	if !ok {
		return
	}
	var projectID *uuid.UUID
	if v := c.Query("project_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			projectID = &id
		}
	}
	exps, total, err := h.service.List(c.Request.Context(), projectID, limit, offset)
	if err != nil {
		handleExpError(c, err)
		return
	}
	if exps == nil {
		exps = []experiment.Experiment{}
	}
	respondList(c, exps, total, limit, offset)
}

func (h *ExperimentHandler) get(c *gin.Context) {
	id, ok := parseUUID(c, "experiment_id")
	if !ok {
		return
	}
	exp, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		handleExpError(c, err)
		return
	}
	respondData(c, http.StatusOK, exp)
}

func (h *ExperimentHandler) delete(c *gin.Context) {
	id, ok := parseUUID(c, "experiment_id")
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		handleExpError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ExperimentHandler) start(c *gin.Context) {
	id, ok := parseUUID(c, "experiment_id")
	if !ok {
		return
	}
	exp, err := h.service.Start(c.Request.Context(), id)
	if err != nil {
		handleExpError(c, err)
		return
	}
	respondData(c, http.StatusOK, exp)
}

func (h *ExperimentHandler) pause(c *gin.Context) {
	id, ok := parseUUID(c, "experiment_id")
	if !ok {
		return
	}
	exp, err := h.service.Pause(c.Request.Context(), id)
	if err != nil {
		handleExpError(c, err)
		return
	}
	respondData(c, http.StatusOK, exp)
}

func (h *ExperimentHandler) resume(c *gin.Context) {
	id, ok := parseUUID(c, "experiment_id")
	if !ok {
		return
	}
	exp, err := h.service.Resume(c.Request.Context(), id)
	if err != nil {
		handleExpError(c, err)
		return
	}
	respondData(c, http.StatusOK, exp)
}

type concludeRequest struct {
	WinnerVersionID uuid.UUID `json:"winner_version_id" binding:"required"`
	Note            string    `json:"note"`
}

func (h *ExperimentHandler) conclude(c *gin.Context) {
	id, ok := parseUUID(c, "experiment_id")
	if !ok {
		return
	}
	var req concludeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "winner_version_id is required")
		return
	}
	exp, err := h.service.Conclude(c.Request.Context(), id, req.WinnerVersionID, req.Note)
	if err != nil {
		handleExpError(c, err)
		return
	}
	respondData(c, http.StatusOK, exp)
}

func (h *ExperimentHandler) cancel(c *gin.Context) {
	id, ok := parseUUID(c, "experiment_id")
	if !ok {
		return
	}
	exp, err := h.service.Cancel(c.Request.Context(), id)
	if err != nil {
		handleExpError(c, err)
		return
	}
	respondData(c, http.StatusOK, exp)
}

func (h *ExperimentHandler) results(c *gin.Context) {
	id, ok := parseUUID(c, "experiment_id")
	if !ok {
		return
	}
	result, err := h.service.EvaluateExperiment(c.Request.Context(), id)
	if err != nil {
		handleExpError(c, err)
		return
	}
	respondData(c, http.StatusOK, result)
}

func (h *ExperimentHandler) cohorts(c *gin.Context) {
	id, ok := parseUUID(c, "experiment_id")
	if !ok {
		return
	}
	assignments, err := h.service.GetCohortAssignments(c.Request.Context(), id)
	if err != nil {
		handleExpError(c, err)
		return
	}
	if assignments == nil {
		assignments = []experiment.CohortAssignment{}
	}
	respondData(c, http.StatusOK, assignments)
}

func handleExpError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, experiment.ErrNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, experiment.ErrConflict):
		respondError(c, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, experiment.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, experiment.ErrInvalidTransition):
		respondError(c, http.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, experiment.ErrNotDeployable):
		respondError(c, http.StatusUnprocessableEntity, "NOT_DEPLOYABLE", err.Error())
	default:
		internalError(c, err, "An internal error occurred")
	}
}
