package rest

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/deploy"
)

// DeploymentHandler handles REST endpoints for deployments.
type DeploymentHandler struct {
	service *deploy.Service
	logger  *slog.Logger
}

// NewDeploymentHandler creates a new DeploymentHandler.
func NewDeploymentHandler(svc *deploy.Service, logger *slog.Logger) *DeploymentHandler {
	return &DeploymentHandler{service: svc, logger: logger}
}

// RegisterRoutes registers deployment routes.
func (h *DeploymentHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/deployments", h.create)
	rg.GET("/deployments", h.list)
	rg.GET("/deployments/:deployment_id", h.get)
	rg.DELETE("/deployments/:deployment_id", h.delete)
	rg.POST("/deployments/:deployment_id/start", h.start)
	rg.POST("/deployments/:deployment_id/pause", h.pause)
	rg.POST("/deployments/:deployment_id/resume", h.resume)
	rg.POST("/deployments/:deployment_id/rollback", h.rollback)
	rg.POST("/deployments/:deployment_id/progress", h.progress)
	rg.GET("/deployments/:deployment_id/health", h.health)
	rg.PATCH("/deployments/:deployment_id/status", h.updateStatus)
}

type createDeploymentRequest struct {
	ProjectID      uuid.UUID                 `json:"project_id" binding:"required"`
	ModelVersionID uuid.UUID                 `json:"model_version_id" binding:"required"`
	Name           string                    `json:"name"`
	TargetSelector map[string]string         `json:"target_selector"`
	Strategy       string                    `json:"strategy"`
	CanaryStages   []deploy.CanaryStageInput `json:"canary_stages"`
	RollingConfig  *deploy.RollingConfig     `json:"rolling_config"`
	HealthGate     *deploy.HealthGate        `json:"health_gate"`
	CreatedBy      string                    `json:"created_by"`
}

func (h *DeploymentHandler) create(c *gin.Context) {
	var req createDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "project_id and model_version_id are required")
		return
	}
	d, err := h.service.Create(c.Request.Context(), deploy.CreateInput{
		ProjectID:      req.ProjectID,
		ModelVersionID: req.ModelVersionID,
		Name:           req.Name,
		TargetSelector: req.TargetSelector,
		Strategy:       req.Strategy,
		CanaryStages:   req.CanaryStages,
		RollingConfig:  req.RollingConfig,
		HealthGate:     req.HealthGate,
		CreatedBy:      req.CreatedBy,
	})
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusCreated, d)
}

func (h *DeploymentHandler) list(c *gin.Context) {
	limit, offset := parsePagination(c)
	deployments, total, err := h.service.List(c.Request.Context(), limit, offset)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	if deployments == nil {
		deployments = []deploy.Deployment{}
	}
	respondList(c, deployments, total, limit, offset)
}

func (h *DeploymentHandler) get(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	d, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusOK, d)
}

func (h *DeploymentHandler) delete(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		handleDeployError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *DeploymentHandler) start(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	d, err := h.service.StartDeployment(c.Request.Context(), id)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusOK, d)
}

func (h *DeploymentHandler) pause(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	d, err := h.service.PauseDeployment(c.Request.Context(), id)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusOK, d)
}

func (h *DeploymentHandler) resume(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	d, err := h.service.ResumeDeployment(c.Request.Context(), id)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusOK, d)
}

type rollbackRequest struct {
	Reason string `json:"reason"`
}

func (h *DeploymentHandler) rollback(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	var req rollbackRequest
	_ = c.ShouldBindJSON(&req)
	if req.Reason == "" {
		req.Reason = "manual rollback"
	}
	d, err := h.service.RollbackDeployment(c.Request.Context(), id, req.Reason)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusOK, d)
}

func (h *DeploymentHandler) progress(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	d, err := h.service.ProgressDeployment(c.Request.Context(), id)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusOK, d)
}

func (h *DeploymentHandler) health(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	status, err := h.service.GetHealthStatus(c.Request.Context(), id)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusOK, status)
}

type updateDeploymentStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

func (h *DeploymentHandler) updateStatus(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	var req updateDeploymentStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "status is required")
		return
	}
	if err := h.service.UpdateStatus(c.Request.Context(), id, req.Status); err != nil {
		handleDeployError(c, err)
		return
	}
	d, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusOK, d)
}

func handleDeployError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, deploy.ErrNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, deploy.ErrConflict):
		respondError(c, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, deploy.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, deploy.ErrInvalidTransition):
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred")
	}
}
