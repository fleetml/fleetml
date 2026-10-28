package rest

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/registry"
)

type VersionHandler struct {
	service *registry.Service
	logger  *slog.Logger
}

func NewVersionHandler(svc *registry.Service, logger *slog.Logger) *VersionHandler {
	return &VersionHandler{service: svc, logger: logger}
}

func (h *VersionHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/models/:model_id/versions", h.create)
	rg.GET("/models/:model_id/versions", h.list)
	rg.GET("/versions/:version_id", h.get)
	rg.PATCH("/versions/:version_id/status", h.updateStatus)
	rg.DELETE("/versions/:version_id", h.delete)
}

type createVersionRequest struct {
	Version      string                 `json:"version" binding:"required"`
	ReleaseNotes string                 `json:"release_notes"`
	Metadata     map[string]interface{} `json:"metadata"`
}

func (h *VersionHandler) create(c *gin.Context) {
	modelID, ok := parseUUID(c, "model_id")
	if !ok {
		return
	}
	var req createVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "version is required")
		return
	}

	v, err := h.service.CreateVersion(c.Request.Context(), modelID, req.Version, req.ReleaseNotes, req.Metadata)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusCreated, v)
}

func (h *VersionHandler) list(c *gin.Context) {
	modelID, ok := parseUUID(c, "model_id")
	if !ok {
		return
	}
	limit, offset, ok := parsePagination(c, maxPageSize)
	if !ok {
		return
	}
	versions, total, err := h.service.ListVersions(c.Request.Context(), modelID, c.Query("status"), limit, offset)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	if versions == nil {
		versions = []registry.RegistryVersion{}
	}
	respondList(c, versions, total, limit, offset)
}

func (h *VersionHandler) get(c *gin.Context) {
	id, ok := parseUUID(c, "version_id")
	if !ok {
		return
	}
	v, err := h.service.GetVersion(c.Request.Context(), id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, v)
}

type updateStatusRequest struct {
	Status     string  `json:"status" binding:"required"`
	PromotedBy *string `json:"promoted_by"`
}

func (h *VersionHandler) updateStatus(c *gin.Context) {
	id, ok := parseUUID(c, "version_id")
	if !ok {
		return
	}
	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "status is required")
		return
	}

	v, err := h.service.UpdateVersionStatus(c.Request.Context(), id, req.Status, req.PromotedBy)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	respondData(c, http.StatusOK, v)
}

func (h *VersionHandler) delete(c *gin.Context) {
	id, ok := parseUUID(c, "version_id")
	if !ok {
		return
	}
	if err := h.service.DeleteVersion(c.Request.Context(), id); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
