package rest

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/fleetml/fleetml/internal/audit"
	"github.com/fleetml/fleetml/internal/auth"
)

// Access groups users, roles and the audit log. Without it the API has a
// single role: every API key may do everything (as before v0.3.0).
type Access struct {
	Users    *auth.UserService
	Resolver ProjectResolver
	Audit    *audit.Logger
}

// AuditRequests records every request that changes something and every
// refused request, after it ran (it runs before Authorize).
func AuditRequests(log *audit.Logger, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		status := c.Writer.Status()
		if c.Request.Method == http.MethodGet && status != http.StatusForbidden {
			return
		}
		e := audit.Entry{
			Actor:        "unknown",
			Action:       c.Request.Method + " " + strings.TrimPrefix(c.FullPath(), "/api/v1"),
			ResourceType: "request",
			Status:       &status,
			IP:           c.ClientIP(),
			Details:      map[string]any{"path": c.Request.URL.Path, "request_id": c.GetString(requestIDKey)},
		}
		if p := principal(c); p != nil {
			e.Actor = p.User.Name
			e.Details["api_key"] = p.Key
		}
		outcome := c.GetString(outcomeKey)
		if status == http.StatusForbidden {
			outcome = "denied"
		}
		if outcome != "" {
			e.Details["outcome"] = outcome
		}
		if r, ok := permissions[e.Action]; ok && r.kind != "" {
			e.ResourceType = r.kind
		}
		if id, err := uuid.Parse(c.GetString(projectKey)); err == nil {
			e.ProjectID = &id
			e.ResourceID = id
		}
		for _, p := range c.Params { // the most specific resource in the path
			if id, err := uuid.Parse(p.Value); err == nil {
				e.ResourceID = id
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := log.Record(ctx, e); err != nil {
			logger.Warn("writing the audit log failed", "error", err)
		}
	}
}

// AccessHandler serves users, project members and the audit log.
type AccessHandler struct {
	users  *auth.UserService
	keys   *auth.APIKeyService
	audit  *audit.Logger
	logger *slog.Logger
}

// RegisterRoutes registers the access routes.
func (h *AccessHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/users", h.listUsers)
	rg.POST("/users", h.createUser)
	rg.POST("/users/:user_id/api-keys", h.createUserKey)
	rg.GET("/projects/:project_id/members", h.members)
	rg.PUT("/projects/:project_id/members", h.setMember)
	rg.DELETE("/projects/:project_id/members/:user_id", h.removeMember)
	rg.GET("/audit", h.auditLog)
	rg.GET("/audit/export", h.auditExport)
}

func (h *AccessHandler) listUsers(c *gin.Context) {
	users, err := h.users.List(c.Request.Context())
	if err != nil {
		handleAuthError(c, err)
		return
	}
	respondData(c, http.StatusOK, users)
}

type createUserRequest struct {
	Name  string `json:"name" binding:"required"`
	Admin bool   `json:"admin"`
}

func (h *AccessHandler) createUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	u, err := h.users.Create(c.Request.Context(), req.Name, req.Admin)
	if err != nil {
		handleAuthError(c, err)
		return
	}
	respondData(c, http.StatusCreated, u)
}

// createUserKey issues an API key acting as the user; the key is shown once.
func (h *AccessHandler) createUserKey(c *gin.Context) {
	userID, ok := parseUUID(c, "user_id")
	if !ok {
		return
	}
	var req createKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	key, k, err := h.keys.Create(c.Request.Context(), userID, req.Name)
	if err != nil {
		handleAuthError(c, err)
		return
	}
	respondData(c, http.StatusCreated, createdKeyResponse{APIKey: *k, Key: key})
}

func (h *AccessHandler) members(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	m, err := h.users.Members(c.Request.Context(), projectID)
	if err != nil {
		handleAuthError(c, err)
		return
	}
	respondData(c, http.StatusOK, m)
}

type setMemberRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	Role   string    `json:"role" binding:"required"`
}

func (h *AccessHandler) setMember(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	var req setMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "user_id and role are required")
		return
	}
	if err := h.users.SetRole(c.Request.Context(), req.UserID, projectID, req.Role); err != nil {
		handleAuthError(c, err)
		return
	}
	h.members(c)
}

func (h *AccessHandler) removeMember(c *gin.Context) {
	projectID, ok := parseUUID(c, "project_id")
	if !ok {
		return
	}
	userID, ok := parseUUID(c, "user_id")
	if !ok {
		return
	}
	if err := h.users.RemoveRole(c.Request.Context(), userID, projectID); err != nil {
		handleAuthError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// auditFilter reads the filters: project_id, actor, action, outcome
// (allowed|denied), from and to (RFC 3339), limit.
func auditFilter(c *gin.Context, maxLimit int) (audit.Filter, error) {
	f := audit.Filter{Actor: c.Query("actor"), Action: c.Query("action"), Outcome: c.Query("outcome"), Limit: 200}
	if v := c.Query("project_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return f, errors.New("project_id must be a UUID")
		}
		f.ProjectID = &id
	}
	if f.Outcome != "" && f.Outcome != "allowed" && f.Outcome != "denied" {
		return f, errors.New("outcome must be allowed or denied")
	}
	for name, dst := range map[string]*time.Time{"from": &f.From, "to": &f.To} {
		if v := c.Query(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return f, fmt.Errorf("%s must be an RFC 3339 time", name)
			}
			*dst = t
		}
	}
	if v := c.Query("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxLimit {
			return f, fmt.Errorf("limit must be between 1 and %d", maxLimit)
		}
		f.Limit = n
	}
	return f, nil
}

func (h *AccessHandler) auditLog(c *gin.Context) {
	f, err := auditFilter(c, 1000)
	if err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	entries, err := h.audit.Query(c.Request.Context(), f)
	if err != nil {
		internalError(c, err, "Failed to read the audit log")
		return
	}
	respondData(c, http.StatusOK, entries)
}

// auditExport is the audit log as CSV, with the same filters (up to 10,000 rows).
func (h *AccessHandler) auditExport(c *gin.Context) {
	f, err := auditFilter(c, 10000)
	if err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	if c.Query("limit") == "" {
		f.Limit = 10000
	}
	entries, err := h.audit.Query(c.Request.Context(), f)
	if err != nil {
		internalError(c, err, "Failed to read the audit log")
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="fleetml-audit.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"recorded_at", "actor", "action", "status", "outcome", "project_id", "resource_type", "resource_id", "ip", "details"})
	for _, e := range entries {
		status, project, outcome := "", "", ""
		if e.Status != nil {
			status = strconv.Itoa(*e.Status)
		}
		if e.ProjectID != nil {
			project = e.ProjectID.String()
		}
		if o, ok := e.Details["outcome"].(string); ok {
			outcome = o
		}
		details, _ := json.Marshal(e.Details)
		_ = w.Write([]string{e.RecordedAt.UTC().Format(time.RFC3339), e.Actor, e.Action, status, outcome, project,
			e.ResourceType, e.ResourceID.String(), e.IP, string(details)})
	}
	w.Flush()
}
