package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
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
	rg.POST("/deployments/:deployment_id/cancel", h.cancel)
	rg.POST("/deployments/:deployment_id/progress", h.progress)
	rg.GET("/deployments/:deployment_id/health", h.health)
	rg.PATCH("/deployments/:deployment_id/status", h.updateStatus)
}

// The IDs are pointers so that "required" means "sent": the all-zeros UUID
// is an ID that matches nothing (404), like any unknown one.
type createDeploymentRequest struct {
	ProjectID      *uuid.UUID                `json:"project_id" binding:"required"`
	ModelVersionID *uuid.UUID                `json:"model_version_id" binding:"required"`
	Name           string                    `json:"name"`
	TargetSelector map[string]string         `json:"target_selector"`
	Strategy       string                    `json:"strategy"`
	CanaryStages   []deploy.CanaryStageInput `json:"canary_stages"`
	RollingConfig  *deploy.RollingConfig     `json:"rolling_config"`
	HealthGate     *deploy.HealthGate        `json:"health_gate"`
	Urgent         bool                      `json:"urgent"`
	CreatedBy      string                    `json:"created_by"`
}

// UnmarshalJSON rejects a top-level key the request does not have, which
// would otherwise be dropped (a misspelled "strategy" made ALL_AT_ONCE).
func (r *createDeploymentRequest) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	known := []string{"project_id", "model_version_id", "name", "target_selector", "strategy", "canary_stages", "rolling_config", "health_gate", "urgent", "created_by"}
	var unknown []string
	for k := range raw {
		if !slices.Contains(known, k) {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return fmt.Errorf("unknown key %q (known: %s)", unknown[0], strings.Join(known, ", "))
	}
	type plain createDeploymentRequest
	return json.Unmarshal(b, (*plain)(r))
}

func (h *DeploymentHandler) create(c *gin.Context) {
	var req createDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var missing validator.ValidationErrors
		if errors.As(err, &missing) {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "project_id and model_version_id are required")
		} else {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body: "+err.Error())
		}
		return
	}
	d, err := h.service.Create(c.Request.Context(), deploy.CreateInput{
		ProjectID:      *req.ProjectID,
		ModelVersionID: *req.ModelVersionID,
		Name:           req.Name,
		TargetSelector: req.TargetSelector,
		Strategy:       req.Strategy,
		CanaryStages:   req.CanaryStages,
		RollingConfig:  req.RollingConfig,
		HealthGate:     req.HealthGate,
		Urgent:         req.Urgent,
		CreatedBy:      req.CreatedBy,
	})
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusCreated, d)
}

// list filters on the project_id query parameter.
func (h *DeploymentHandler) list(c *gin.Context) {
	limit, offset, ok := parsePagination(c, maxPageSize)
	if !ok {
		return
	}
	var projectID *uuid.UUID
	if v := c.Query("project_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "project_id must be a UUID")
			return
		}
		projectID = &id
	}
	deployments, total, err := h.service.List(c.Request.Context(), projectID, limit, offset)
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
	Reason string `json:"reason"`
}

func (h *DeploymentHandler) cancel(c *gin.Context) {
	id, ok := parseUUID(c, "deployment_id")
	if !ok {
		return
	}
	d, err := h.service.CancelDeployment(c.Request.Context(), id)
	if err != nil {
		handleDeployError(c, err)
		return
	}
	respondData(c, http.StatusOK, d)
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
	d, err := h.service.UpdateStatus(c.Request.Context(), id, req.Status, req.Reason)
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
		respondError(c, http.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, deploy.ErrNotDeployable):
		respondError(c, http.StatusUnprocessableEntity, "NOT_DEPLOYABLE", err.Error())
	default:
		internalError(c, err, "An internal error occurred")
	}
}
