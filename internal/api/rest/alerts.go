package rest

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/alert"
)

// AlertSinkHandler manages per-project alert sinks and their delivery log.
type AlertSinkHandler struct {
	repo     *alert.Repo
	notifier *alert.Notifier
	smtpOK   bool
	logger   *slog.Logger
}

// NewAlertSinkHandler creates an AlertSinkHandler; smtpOK enables email sinks.
func NewAlertSinkHandler(repo *alert.Repo, notifier *alert.Notifier, smtpOK bool, logger *slog.Logger) *AlertSinkHandler {
	return &AlertSinkHandler{repo: repo, notifier: notifier, smtpOK: smtpOK, logger: logger}
}

// RegisterRoutes registers alert sink routes.
func (h *AlertSinkHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/projects/:project_id/alert-sinks", h.create)
	rg.GET("/projects/:project_id/alert-sinks", h.list)
	rg.GET("/alert-sinks/:sink_id", h.get)
	rg.PATCH("/alert-sinks/:sink_id", h.update)
	rg.DELETE("/alert-sinks/:sink_id", h.delete)
	rg.POST("/alert-sinks/:sink_id/test", h.test)
	rg.GET("/alert-sinks/:sink_id/deliveries", h.deliveries)
}

type createSinkRequest struct {
	Name    string            `json:"name" binding:"required"`
	Kind    string            `json:"kind" binding:"required"`
	Config  map[string]string `json:"config"`
	Enabled *bool             `json:"enabled"`
}

type updateSinkRequest struct {
	Name    *string           `json:"name"`
	Config  map[string]string `json:"config"`
	Enabled *bool             `json:"enabled"`
}

// redacted hides the sink's secrets in responses.
func redacted(s alert.SinkConfig) alert.SinkConfig {
	s.Config = alert.RedactedConfig(s.Kind, s.Config)
	return s
}

func handleAlertError(c *gin.Context, err error) {
	if errors.Is(err, alert.ErrNotFound) {
		respondError(c, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	internalError(c, err, "An internal error occurred")
}

func (h *AlertSinkHandler) create(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	var req createSinkRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name and kind are required")
		return
	}
	if err := alert.ValidateSink(req.Kind, req.Config, h.smtpOK); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	s := alert.SinkConfig{ProjectID: projectID.String(), Name: strings.TrimSpace(req.Name), Kind: req.Kind, Config: req.Config, Enabled: true}
	if req.Enabled != nil {
		s.Enabled = *req.Enabled
	}
	out, err := h.repo.Create(c.Request.Context(), s)
	if err != nil {
		handleAlertError(c, err)
		return
	}
	respondData(c, http.StatusCreated, redacted(*out))
}

func (h *AlertSinkHandler) list(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	sinks, err := h.repo.List(c.Request.Context(), projectID.String())
	if err != nil {
		handleAlertError(c, err)
		return
	}
	for i := range sinks {
		sinks[i] = redacted(sinks[i])
	}
	respondData(c, http.StatusOK, sinks)
}

func (h *AlertSinkHandler) load(c *gin.Context) (*alert.SinkConfig, bool) {
	id, ok := parseUUID(c, "sink_id")
	if !ok {
		return nil, false
	}
	s, err := h.repo.Get(c.Request.Context(), id.String())
	if err != nil {
		handleAlertError(c, err)
		return nil, false
	}
	return s, true
}

func (h *AlertSinkHandler) get(c *gin.Context) {
	if s, ok := h.load(c); ok {
		respondData(c, http.StatusOK, redacted(*s))
	}
}

// update changes the name, enabled flag or config. Config values sent back
// redacted (as the API shows them) keep their stored value.
func (h *AlertSinkHandler) update(c *gin.Context) {
	s, ok := h.load(c)
	if !ok {
		return
	}
	var req updateSinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid body")
		return
	}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name must not be empty")
			return
		}
		s.Name = strings.TrimSpace(*req.Name)
	}
	if req.Enabled != nil {
		s.Enabled = *req.Enabled
	}
	if req.Config != nil {
		shown := alert.RedactedConfig(s.Kind, s.Config)
		for k, v := range req.Config {
			if v == shown[k] && shown[k] != s.Config[k] {
				req.Config[k] = s.Config[k]
			}
		}
		if err := alert.ValidateSink(s.Kind, req.Config, h.smtpOK); err != nil {
			respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		s.Config = req.Config
	}
	out, err := h.repo.Update(c.Request.Context(), *s)
	if err != nil {
		handleAlertError(c, err)
		return
	}
	respondData(c, http.StatusOK, redacted(*out))
}

func (h *AlertSinkHandler) delete(c *gin.Context) {
	id, ok := parseUUID(c, "sink_id")
	if !ok {
		return
	}
	if err := h.repo.Delete(c.Request.Context(), id.String()); err != nil {
		handleAlertError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// test sends one test alert, without retries, and returns the attempt.
func (h *AlertSinkHandler) test(c *gin.Context) {
	s, ok := h.load(c)
	if !ok {
		return
	}
	respondData(c, http.StatusOK, h.notifier.SendTest(c.Request.Context(), *s))
}

func (h *AlertSinkHandler) deliveries(c *gin.Context) {
	id, ok := parseUUID(c, "sink_id")
	if !ok {
		return
	}
	out, err := h.repo.Deliveries(c.Request.Context(), id.String(), alert.DeliveryLogSize)
	if err != nil {
		handleAlertError(c, err)
		return
	}
	respondData(c, http.StatusOK, out)
}
