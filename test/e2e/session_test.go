//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// V2.3: the dashboard's cookie session against a real server.
func TestE2E_DashboardSession(t *testing.T) {
	e := newEnv(t)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	browser := &http.Client{Jar: jar}
	send := func(method, path, csrf, body string) (int, map[string]any) {
		req, err := http.NewRequest(method, e.api+path, strings.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		resp, err := browser.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	login := func() string {
		code, out := send("POST", "/api/v1/auth/login", "", `{"api_key":"`+e.key+`"}`)
		require.Equal(t, 200, code, out)
		return out["data"].(map[string]any)["csrf_token"].(string)
	}

	code, _ := send("GET", "/api/v1/projects", "", "")
	require.Equal(t, 401, code)
	csrf1 := login()
	code, _ = send("GET", "/api/v1/projects", "", "")
	assert.Equal(t, 200, code, "the cookie authenticates")
	code, out := send("POST", "/api/v1/projects", "", `{"name":"session-`+randHex(4)+`"}`)
	assert.Equal(t, 403, code, out)
	code, _ = send("POST", "/api/v1/projects", csrf1, `{"name":"session-`+randHex(4)+`"}`)
	assert.Equal(t, 201, code)

	code, _ = send("POST", "/api/v1/auth/logout", csrf1, "")
	require.Equal(t, 204, code)
	code, _ = send("GET", "/api/v1/projects", "", "")
	assert.Equal(t, 401, code, "logged out")

	csrf2 := login()
	code, out = send("POST", "/api/v1/projects", csrf1, `{"name":"session-`+randHex(4)+`"}`)
	assert.Equal(t, 403, code, "the previous session's CSRF token is stale")
	assert.Equal(t, "CSRF_INVALID", out["error"].(map[string]any)["code"])

	// An operator with the API key revokes the session; the browser is logged out.
	var sessions []struct {
		ID string `json:"id"`
	}
	require.Equal(t, 200, e.call(t, "GET", "/api/v1/auth/sessions", nil, &sessions))
	require.NotEmpty(t, sessions)
	require.Equal(t, 204, e.call(t, "POST", "/api/v1/auth/sessions/"+sessions[0].ID+"/revoke", nil, nil))
	code, out = send("POST", "/api/v1/projects", csrf2, `{"name":"x"}`)
	assert.Equal(t, 401, code)
	assert.Equal(t, "SESSION_EXPIRED", out["error"].(map[string]any)["code"])
}
