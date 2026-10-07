package rest

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

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
	rg.GET("/device-classes", h.listDeviceClasses)
	rg.GET("/device-classes/:class_id", h.getDeviceClass)
}

func (h *DeviceHandler) listDevices(c *gin.Context) {
	limit, offset := parsePagination(c)

	// Parse label.* query params
	labels := make(map[string]string)
	for key, values := range c.Request.URL.Query() {
		if strings.HasPrefix(key, "label.") {
			labelKey := strings.TrimPrefix(key, "label.")
			if len(values) > 0 && labelKey != "" {
				labels[labelKey] = values[0]
			}
		}
	}

	var (
		devices []device.Device
		total   int
		err     error
	)
	if len(labels) > 0 {
		devices, total, err = h.service.ListDevicesByLabels(c.Request.Context(), labels, limit, offset)
	} else {
		devices, total, err = h.service.ListDevices(c.Request.Context(), limit, offset)
	}
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
		respondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred")
	}
}
