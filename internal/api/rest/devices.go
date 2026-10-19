package rest

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/device"
)

// DeviceHandler handles REST endpoints for device management.
type DeviceHandler struct {
	service *device.Service
	logger  *slog.Logger
}

// NewDeviceHandler creates a new DeviceHandler.
func NewDeviceHandler(svc *device.Service, logger *slog.Logger) *DeviceHandler {
	return &DeviceHandler{service: svc, logger: logger}
}

// RegisterRoutes registers device routes.
func (h *DeviceHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/devices", h.listDevices)
	rg.GET("/devices/:device_id", h.getDevice)
	rg.DELETE("/devices/:device_id", h.deleteDevice)
	rg.POST("/devices/:device_id/revoke", h.revokeDevice)
	rg.GET("/device-classes", h.listDeviceClasses)
	rg.GET("/device-classes/:class_id", h.getDeviceClass)
}

// listDevices filters on project_id and on label.<key>=<value> query parameters.
func (h *DeviceHandler) listDevices(c *gin.Context) {
	limit, offset := parsePagination(c)

	var f device.ListFilter
	if v := c.Query("project_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "project_id must be a UUID")
			return
		}
		f.ProjectID = &id
	}
	for key, values := range c.Request.URL.Query() {
		if labelKey, ok := strings.CutPrefix(key, "label."); ok && labelKey != "" && len(values) > 0 {
			if f.Labels == nil {
				f.Labels = map[string]string{}
			}
			f.Labels[labelKey] = values[0]
		}
	}

	devices, total, err := h.service.ListDevices(c.Request.Context(), f, limit, offset)
	if err != nil {
		handleDeviceError(c, err)
		return
	}
	if devices == nil {
		devices = []device.Device{}
	}
	respondList(c, devices, total, limit, offset)
}

func (h *DeviceHandler) getDevice(c *gin.Context) {
	id, ok := parseUUID(c, "device_id")
	if !ok {
		return
	}
	d, err := h.service.GetDevice(c.Request.Context(), id)
	if err != nil {
		handleDeviceError(c, err)
		return
	}
	respondData(c, http.StatusOK, d)
}

func (h *DeviceHandler) deleteDevice(c *gin.Context) {
	id, ok := parseUUID(c, "device_id")
	if !ok {
		return
	}
	if err := h.service.DeleteDevice(c.Request.Context(), id); err != nil {
		handleDeviceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// revokeDevice invalidates the device's token; it must re-enroll to get a new one.
func (h *DeviceHandler) revokeDevice(c *gin.Context) {
	id, ok := parseUUID(c, "device_id")
	if !ok {
		return
	}
	if err := h.service.RevokeDeviceToken(c.Request.Context(), id); err != nil {
		handleDeviceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *DeviceHandler) listDeviceClasses(c *gin.Context) {
	classes, err := h.service.ListDeviceClasses(c.Request.Context())
	if err != nil {
		handleDeviceError(c, err)
		return
	}
	if classes == nil {
		classes = []device.DeviceClass{}
	}
	respondData(c, http.StatusOK, classes)
}

func (h *DeviceHandler) getDeviceClass(c *gin.Context) {
	id, ok := parseUUID(c, "class_id")
	if !ok {
		return
	}
	dc, err := h.service.GetDeviceClass(c.Request.Context(), id)
	if err != nil {
		handleDeviceError(c, err)
		return
	}
	respondData(c, http.StatusOK, dc)
}

func handleDeviceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, device.ErrNotFound):
		respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, device.ErrConflict):
		respondError(c, http.StatusConflict, "CONFLICT", err.Error())
	case errors.Is(err, device.ErrInvalidInput):
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		internalError(c, err, "An internal error occurred")
	}
}
