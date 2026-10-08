package rest

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fleetml/fleetml/internal/auth"
)

// Dashboard session cookie and CSRF header.
const (
	SessionCookie   = "fleetml_session"
	CSRFHeader      = "X-CSRF-Token"
	sessionKey      = "session"
	sessionTokenKey = "session_token"
)

// currentSession is the session that authenticated the request, or nil for an API key.
func currentSession(c *gin.Context) *auth.Session {
	if v, ok := c.Get(sessionKey); ok {
		return v.(*auth.Session)
	}
	return nil
}

// SessionHandler logs the dashboard in and out and manages sessions.
type SessionHandler struct {
	sessions *auth.SessionService
	logger   *slog.Logger
}

// NewSessionHandler creates a SessionHandler.
func NewSessionHandler(sessions *auth.SessionService, logger *slog.Logger) *SessionHandler {
	return &SessionHandler{sessions: sessions, logger: logger}
}

// RegisterRoutes registers the login route on public and the others on api.
func (h *SessionHandler) RegisterRoutes(public, api *gin.RouterGroup) {
	public.POST("/auth/login", h.login)
	api.GET("/auth/session", h.current)
	api.POST("/auth/logout", h.logout)
	api.GET("/auth/sessions", h.list)
	api.POST("/auth/sessions/:session_id/revoke", h.revoke)
}

type loginRequest struct {
	APIKey string `json:"api_key" binding:"required"`
}

// sessionResponse is a session with the CSRF token for its holder.
type sessionResponse struct {
	Session   *auth.Session `json:"session"`
	CSRFToken string        `json:"csrf_token"`
}

func setSessionCookie(c *gin.Context, token string, expires time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: SessionCookie, Value: token, Path: "/", Expires: expires, MaxAge: int(time.Until(expires).Seconds()),
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: secureRequest(c),
	})
}

func clearSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: secureRequest(c),
	})
}

// secureRequest reports whether the browser reached us over HTTPS, directly
// or through a TLS-terminating proxy; the cookie is then marked Secure.
func secureRequest(c *gin.Context) bool {
	return c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

func (h *SessionHandler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "VALIDATION_ERROR", "api_key is required")
		return
	}
	token, sess, err := h.sessions.Login(c.Request.Context(), strings.TrimSpace(req.APIKey), c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		if errors.Is(err, auth.ErrInvalidKey) {
			h.logger.Warn("dashboard login with an invalid or revoked API key", "ip", c.ClientIP())
			respondError(c, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or revoked API key")
			return
		}
		internalError(c, err, "Login failed")
		return
	}
	setSessionCookie(c, token, sess.ExpiresAt)
	respondData(c, http.StatusOK, sessionResponse{Session: sess, CSRFToken: h.sessions.CSRFToken(token)})
}

// current returns the caller's session and its CSRF token (after a reload).
func (h *SessionHandler) current(c *gin.Context) {
	sess := currentSession(c)
	if sess == nil {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "this request is authenticated with an API key, not a session")
		return
	}
	respondData(c, http.StatusOK, sessionResponse{Session: sess, CSRFToken: h.sessions.CSRFToken(c.GetString(sessionTokenKey))})
}

func (h *SessionHandler) logout(c *gin.Context) {
	if sess := currentSession(c); sess != nil {
		if err := h.sessions.Revoke(c.Request.Context(), sess.ID); err != nil {
			handleAuthError(c, err)
			return
		}
	}
	clearSessionCookie(c)
	c.Status(http.StatusNoContent)
}

type sessionView struct {
	auth.Session
	Current bool `json:"current"`
}

func (h *SessionHandler) list(c *gin.Context) {
	sessions, err := h.sessions.List(c.Request.Context())
	if err != nil {
		handleAuthError(c, err)
		return
	}
	me := currentSession(c)
	out := make([]sessionView, len(sessions))
	for i, s := range sessions {
		out[i] = sessionView{Session: s, Current: me != nil && s.ID == me.ID}
	}
	respondData(c, http.StatusOK, out)
}

func (h *SessionHandler) revoke(c *gin.Context) {
	id, ok := parseUUID(c, "session_id")
	if !ok {
		return
	}
	if err := h.sessions.Revoke(c.Request.Context(), id); err != nil {
		handleAuthError(c, err)
		return
	}
	if me := currentSession(c); me != nil && me.ID == id {
		clearSessionCookie(c)
	}
	c.Status(http.StatusNoContent)
}
