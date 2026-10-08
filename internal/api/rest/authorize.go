package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fleetml/fleetml/internal/auth"

	visibility "github.com/fleetml/fleetml/internal/scope"
)

const (
	principalKey = "principal"
	projectKey   = "authz_project"
	outcomeKey   = "authz_outcome"
)

// Principals loads who an API key acts as (implemented by *auth.UserService).
type Principals interface {
	PrincipalForKey(ctx context.Context, apiKeyID uuid.UUID) (*auth.Principal, error)
}

// ProjectResolver finds the project a resource belongs to; "" if there is no such resource.
type ProjectResolver interface {
	ProjectOf(ctx context.Context, kind, id string) (string, error)
}

// PoolResolver resolves projects with resolveQueries.
type PoolResolver struct{ Pool *pgxpool.Pool }

// ProjectOf implements ProjectResolver.
func (r PoolResolver) ProjectOf(ctx context.Context, kind, id string) (string, error) {
	q, ok := resolveQueries[kind]
	if !ok {
		return "", fmt.Errorf("authz: unknown resource kind %q", kind)
	}
	if kind != "slug" {
		if _, err := uuid.Parse(id); err != nil {
			return "", nil // the handler answers 400 or 404
		}
	}
	var project string
	err := r.Pool.QueryRow(ctx, q, id).Scan(&project)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return project, err
}

// principal is the request's principal (nil before Authorize ran).
func principal(c *gin.Context) *auth.Principal {
	if v, ok := c.Get(principalKey); ok {
		return v.(*auth.Principal)
	}
	return nil
}

// roleIn is the caller's role in a project; global admins are admins everywhere.
func roleIn(p *auth.Principal, project string) role {
	if p == nil {
		return anyone
	}
	if p.User.Admin {
		return admin
	}
	r, ok := roleRank[p.Roles[project]]
	if !ok {
		return -1
	}
	return r
}

// Authorize enforces the permission matrix. It runs after RequireAuth.
func Authorize(principals Principals, resolver ProjectResolver, logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyID, err := uuid.Parse(c.GetString(apiKeyIDKey))
		if err != nil {
			unauthorized(c, "missing API key")
			return
		}
		p, err := principals.PrincipalForKey(c.Request.Context(), keyID)
		if err != nil {
			internalError(c, err, "An internal error occurred")
			c.Abort()
			return
		}
		c.Set(principalKey, p)

		route := c.Request.Method + " " + strings.TrimPrefix(c.FullPath(), "/api/v1")
		r, ok := permissions[route]
		if !ok {
			forbidden(c, "this route is not in the permission matrix")
			return
		}
		project := ""
		switch r.scope {
		case scopeSelf, scopeList:
			if r.scope == scopeList && !p.User.Admin {
				var visible []uuid.UUID
				for id := range p.Roles {
					if u, err := uuid.Parse(id); err == nil {
						visible = append(visible, u)
					}
				}
				c.Request = c.Request.WithContext(visibility.WithProjects(c.Request.Context(), visible))
			}
			c.Set(outcomeKey, "allowed")
			c.Next()
			return
		case scopeGlobal:
		case scopeProject, scopeResource:
			project, err = resolver.ProjectOf(c.Request.Context(), r.kind, c.Param(r.param))
		case scopeQuery:
			if v := c.Query(r.param); v != "" {
				project, err = resolver.ProjectOf(c.Request.Context(), r.kind, v)
			}
		case scopeBody:
			if id := bodyField(c, r.param); id != "" {
				project, err = resolver.ProjectOf(c.Request.Context(), r.kind, id)
			}
		}
		if err != nil {
			internalError(c, err, "An internal error occurred")
			c.Abort()
			return
		}
		c.Set(projectKey, project)

		allowed := false
		switch {
		case p.User.Admin:
			allowed = true
		case r.scope == scopeGlobal || project == "":
			// A route without a project is for global admins; so is a resource
			// that does not exist (no existence leaks to other projects).
		default:
			allowed = roleIn(p, project) >= r.role
		}
		if !allowed {
			need := "a global admin"
			if r.scope != scopeGlobal && project != "" {
				need = "the " + r.role.String() + " role in this project"
			}
			logger.Warn("request refused", "user", p.User.Name, "route", route, "project_id", project, "needs", need)
			forbidden(c, "this needs "+need)
			return
		}
		c.Set(outcomeKey, "allowed")
		c.Next()
	}
}

func forbidden(c *gin.Context, message string) {
	c.Set(outcomeKey, "denied")
	respondError(c, http.StatusForbidden, "FORBIDDEN", message)
	c.Abort()
}

// bodyField reads a string field of the JSON body and puts the body back
// for the handler ("" if there is none: the handler answers 400).
func bodyField(c *gin.Context, field string) string {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var m map[string]any
	if err != nil || json.Unmarshal(raw, &m) != nil {
		return ""
	}
	s, _ := m[field].(string)
	return s
}
