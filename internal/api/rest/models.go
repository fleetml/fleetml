package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/registry"
)

type ModelHandler struct {
	service *registry.Service
	logger  *slog.Logger
}

func NewModelHandler(svc *registry.Service, logger *slog.Logger) *ModelHandler {
	return &ModelHandler{service: svc, logger: logger}
}

func (h *ModelHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/projects/:project_id/models", h.create)
	rg.GET("/projects/:project_id/models", h.list)
	rg.GET("/models/:model_id", h.get)
	rg.PUT("/models/:model_id", h.update)
	rg.DELETE("/models/:model_id", h.delete)
}

type createModelRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Framework   string `json:"framework" binding:"required"`
	TaskType    string `json:"task_type" binding:"required"`
	CreatedBy   string `json:"created_by"`
}

func (h *ModelHandler) create(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	var req createModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name, framework, and task_type are required")
		return
	}

	m, err := h.service.CreateModel(c.Request.Context(), projectID, req.Name, req.Description, req.Framework, req.TaskType, req.CreatedBy)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusCreated, m)
}

func (h *ModelHandler) list(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	limit, offset, ok := parsePagination(c)
	if !ok {
		return
	}
	models, total, err := h.service.ListModels(c.Request.Context(), projectID, limit, offset)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	if models == nil {
		models = []registry.RegistryModel{}
	}
	respondList(c, models, total, limit, offset)
}

func (h *ModelHandler) get(c *gin.Context) {
	id, ok := parseUUID(c, "model_id")
	if !ok {
		return
	}
	m, err := h.service.GetModel(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, m)
}

type updateModelRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Framework   string `json:"framework"`
	TaskType    string `json:"task_type"`
}

func (h *ModelHandler) update(c *gin.Context) {
	id, ok := parseUUID(c, "model_id")
	if !ok {
		return
	}
	var req updateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	m, err := h.service.UpdateModel(c.Request.Context(), id, req.Name, req.Description, req.Framework, req.TaskType)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, m)
}

func (h *ModelHandler) delete(c *gin.Context) {
	id, ok := parseUUID(c, "model_id")
	if !ok {
		return
	}
	if err := h.service.DeleteModel(c.Request.Context(), id); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
