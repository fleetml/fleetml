package rest

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/auth"
	"github.com/fleetml/fleetml/internal/drift"
)

// apiRoutes registers every handler the server has (with nil dependencies:
// registering does not use them) and returns its routes under /api/v1.
func apiRoutes(t *testing.T) []gin.RouteInfo {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api/v1")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	NewAPIKeyHandler(nil, logger).RegisterRoutes(api)
	NewEnrollmentKeyHandler(nil, logger).RegisterRoutes(api)
	NewSessionHandler(nil, logger).RegisterRoutes(r.Group("/api/v1"), api)
	NewProjectHandler(nil, logger).RegisterRoutes(api)
	NewModelHandler(nil, logger).RegisterRoutes(api)
	NewVersionHandler(nil, logger).RegisterRoutes(api)
	NewVariantHandler(nil, logger).RegisterRoutes(api)
	NewDeviceHandler(nil, logger).RegisterRoutes(api)
	NewDeploymentHandler(nil, logger).RegisterRoutes(api)
	NewIntelligenceHandler(nil, nil, nil, logger).RegisterRoutes(api)
	NewTelemetryHandler(nil, logger).RegisterRoutes(api)
	NewExperimentHandler(nil, logger).RegisterRoutes(api)
	NewDriftHandler(nil, drift.NewAdapter(nil), logger).RegisterRoutes(api)
	NewSigningHandler(nil, logger).RegisterRoutes(api)
	NewAlertSinkHandler(nil, nil, false, logger).RegisterRoutes(api)
	(&AccessHandler{}).RegisterRoutes(api)
	var out []gin.RouteInfo
	for _, ri := range r.Routes() {
		if strings.HasPrefix(ri.Path, "/api/v1/") && ri.Path != "/api/v1/auth/login" {
			out = append(out, ri)
		}
	}
	return out
}

func routeKey(ri gin.RouteInfo) string {
	return ri.Method + " " + strings.TrimPrefix(ri.Path, "/api/v1")
}

// The matrix and the router agree: every route has a rule, every rule a route.
func TestPermissions_EveryRoute(t *testing.T) {
	routes := apiRoutes(t)
	seen := map[string]bool{}
	for _, ri := range routes {
		seen[routeKey(ri)] = true
		_, ok := permissions[routeKey(ri)]
		assert.True(t, ok, "%s has no rule in the permission matrix", routeKey(ri))
	}
	for k := range permissions {
		assert.True(t, seen[k], "the rule %s matches no route", k)
	}
	assert.GreaterOrEqual(t, len(routes), 90)
}

type fakePrincipals map[uuid.UUID]*auth.Principal

func (f fakePrincipals) PrincipalForKey(_ context.Context, id uuid.UUID) (*auth.Principal, error) {
	return f[id], nil
}

// Every resource belongs to projectA, except the one called "missing".
type fakeResolver struct{ projectA string }

func (f fakeResolver) ProjectOf(_ context.Context, _, id string) (string, error) {
	if id == "missing" || id == "" {
		return "", nil
	}
	return f.projectA, nil
}

// Every route, for each kind of caller: allowed exactly when the matrix says so.
func TestPermissions_Matrix(t *testing.T) {
	projectA, projectB := uuid.NewString(), uuid.NewString()
	callers := map[string]*auth.Principal{
		"global admin": {User: auth.User{Name: "root", Admin: true}},
		"admin":        {User: auth.User{Name: "ada"}, Roles: map[string]string{projectA: auth.RoleAdmin}},
		"operator":     {User: auth.User{Name: "otto"}, Roles: map[string]string{projectA: auth.RoleOperator}},
		"viewer":       {User: auth.User{Name: "vera"}, Roles: map[string]string{projectA: auth.RoleViewer}},
		"outsider":     {User: auth.User{Name: "oscar"}, Roles: map[string]string{projectB: auth.RoleAdmin}},
	}
	keys := fakePrincipals{}
	keyOf := map[string]uuid.UUID{}
	for name, p := range callers {
		id := uuid.New()
		keys[id], keyOf[name] = p, id
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	r := gin.New()
	api := r.Group("/api/v1", func(c *gin.Context) { c.Set(apiKeyIDKey, c.GetHeader("X-Test-Key")); c.Next() },
		Authorize(keys, fakeResolver{projectA}, logger))
	routes := apiRoutes(t)
	for _, ri := range routes {
		api.Handle(ri.Method, strings.TrimPrefix(ri.Path, "/api/v1"), func(c *gin.Context) { c.Status(http.StatusOK) })
	}

	expected := func(name string, rl rule) bool {
		switch {
		case rl.scope == scopeSelf || rl.scope == scopeList:
			return true
		case name == "global admin":
			return true
		case rl.scope == scopeGlobal:
			return false
		}
		rank := map[string]role{"admin": admin, "operator": operator, "viewer": viewer, "outsider": -1}[name]
		return rank >= rl.role
	}
	checked := 0
	for _, ri := range routes {
		rl := permissions[routeKey(ri)]
		path := ri.Path
		for _, seg := range strings.Split(path, "/") {
			if strings.HasPrefix(seg, ":") {
				v := uuid.NewString()
				if seg == ":project_id" {
					v = projectA
				}
				path = strings.Replace(path, seg, v, 1)
			}
		}
		if rl.scope == scopeQuery {
			path += "?" + rl.param + "=" + projectA
		}
		for name := range callers {
			req := httptest.NewRequest(ri.Method, path, strings.NewReader(`{"project_id":"`+projectA+`","deployment_id":"`+uuid.NewString()+`"}`))
			req.Header.Set("X-Test-Key", keyOf[name].String())
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			want := http.StatusForbidden
			if expected(name, rl) {
				want = http.StatusOK
			}
			require.Equal(t, want, w.Code, "%s as %s", routeKey(ri), name)
			checked++
		}
	}
	assert.Equal(t, len(routes)*len(callers), checked)

	// A resource that does not exist is refused to everyone but global admins, so
	// its existence in another project does not leak.
	for name, want := range map[string]int{"viewer": http.StatusForbidden, "global admin": http.StatusOK} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/by-slug/missing", http.NoBody)
		req.Header.Set("X-Test-Key", keyOf[name].String())
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, want, w.Code, name)
	}
}
