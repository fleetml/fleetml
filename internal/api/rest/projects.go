package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/registry"
)

type ProjectHandler struct {
	service *registry.Service
	logger  *slog.Logger
}

func NewProjectHandler(svc *registry.Service, logger *slog.Logger) *ProjectHandler {
	return &ProjectHandler{service: svc, logger: logger}
}

func (h *ProjectHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/projects", h.create)
	rg.GET("/projects", h.list)
	rg.GET("/projects/:project_id", h.get)
	rg.GET("/projects/by-slug/:slug", h.getBySlug)
	rg.PUT("/projects/:project_id", h.update)
	rg.DELETE("/projects/:project_id", h.delete)
}

type createProjectRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

func (h *ProjectHandler) create(c *gin.Context) {
	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}

	p, err := h.service.CreateProject(c.Request.Context(), req.Name, req.Description)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusCreated, p)
}

func (h *ProjectHandler) list(c *gin.Context) {
	limit, offset, ok := parsePagination(c, maxPageSize)
	if !ok {
		return
	}
	projects, total, err := h.service.ListProjects(c.Request.Context(), limit, offset)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	if projects == nil {
		projects = []registry.Project{}
	}
	respondList(c, projects, total, limit, offset)
}

func (h *ProjectHandler) get(c *gin.Context) {
	id, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	p, err := h.service.GetProject(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, p)
}

func (h *ProjectHandler) getBySlug(c *gin.Context) {
	slug := c.Param("slug")
	p, err := h.service.GetProjectBySlug(c.Request.Context(), slug)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, p)
}

type updateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *ProjectHandler) update(c *gin.Context) {
	id, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	var req updateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	p, err := h.service.UpdateProject(c.Request.Context(), id, req.Name, req.Description)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, p)
}

func (h *ProjectHandler) delete(c *gin.Context) {
	id, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	if err := h.service.DeleteProject(c.Request.Context(), id); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
