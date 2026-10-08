package rest

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// DefaultCORSOrigins are the dashboard origins used in development:
// the Vite dev server and the API's own origin.
const DefaultCORSOrigins = "http://localhost:5173,http://localhost:18080"

// ParseCORSOrigins splits a comma-separated origin list and normalises each
// entry. "*" allows any origin, but is never combined with credentials.
func ParseCORSOrigins(list string) (origins map[string]bool, wildcard bool) {
	origins = map[string]bool{}
	for _, o := range strings.Split(list, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			wildcard = true
			continue
		}
		if n, ok := normalizeOrigin(o); ok {
			origins[n] = true
		}
	}
	return origins, wildcard
}

// normalizeOrigin lowercases scheme and host and drops default ports, so that
// "http://localhost:80" in config matches the "http://localhost" browsers send.
func normalizeOrigin(o string) (string, bool) {
	u, err := url.Parse(strings.TrimSuffix(o, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Path != "" && u.Path != "/") {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host, true
}

// CORS allows cross-origin requests only from the configured origins. It never
// reflects an arbitrary Origin and never sends Access-Control-Allow-Credentials:
// the API authenticates with a bearer header, not cookies.
func CORS(list string) gin.HandlerFunc {
	origins, wildcard := ParseCORSOrigins(list)
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		c.Header("Vary", "Origin")
		if origin == "" {
			c.Next()
			return
		}

		allowed := ""
		if n, ok := normalizeOrigin(origin); ok && origins[n] {
			allowed = origin
		} else if wildcard {
			allowed = "*"
		}

		preflight := c.Request.Method == http.MethodOptions && c.GetHeader("Access-Control-Request-Method") != ""
		if allowed == "" {
			if preflight {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			// No CORS headers: the browser blocks the response.
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", allowed)
		if preflight {
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
			c.Header("Access-Control-Max-Age", "3600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Header("Access-Control-Expose-Headers", "X-Request-ID")
		c.Next()
	}
}
