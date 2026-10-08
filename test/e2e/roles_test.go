//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// V3.5: a viewer reads but cannot deploy; the refusal is in the audit log.
func TestE2E_RolesAndAudit(t *testing.T) {
	e := newEnv(t)
	f := e.newFixture(t)
	other := e.newFixture(t)
	v1, _, _ := e.newVersion(t, f, "1.0.0", "")

	newUser := func(name, role string) string {
		var u struct {
			ID string `json:"id"`
		}
		require.Equal(t, 201, e.call(t, "POST", "/api/v1/users", map[string]any{"name": name + "-" + randHex(3)}, &u))
		var k struct {
			Key string `json:"key"`
		}
		require.Equal(t, 201, e.call(t, "POST", "/api/v1/users/"+u.ID+"/api-keys", map[string]string{"name": "e2e"}, &k))
		require.Equal(t, 200, e.call(t, "PUT", "/api/v1/projects/"+f.project+"/members", map[string]string{"user_id": u.ID, "role": role}, nil))
		return k.Key
	}
	viewer, operator := newUser("vera", "viewer"), newUser("otto", "operator")
	deployment := map[string]any{"project_id": f.project, "model_version_id": v1, "target_selector": map[string]string{"suite": randHex(4)}}

	assert.Equal(t, 200, e.callAs(t, viewer, "GET", "/api/v1/projects/"+f.project, nil, nil), "a viewer reads its project")
	assert.Equal(t, 403, e.callAs(t, viewer, "POST", "/api/v1/deployments", deployment, nil), "a viewer cannot deploy")
	assert.Equal(t, 403, e.callAs(t, viewer, "GET", "/api/v1/projects/"+other.project, nil, nil), "nor read another project")
	assert.Equal(t, 403, e.callAs(t, viewer, "POST", "/api/v1/projects", map[string]string{"name": "x"}, nil), "nor create projects")
	var projects []struct {
		ID string `json:"id"`
	}
	require.Equal(t, 200, e.callAs(t, viewer, "GET", "/api/v1/projects", nil, &projects))
	require.Len(t, projects, 1, "lists show only the caller's projects")
	assert.Equal(t, f.project, projects[0].ID)
	assert.Equal(t, 201, e.callAs(t, operator, "POST", "/api/v1/deployments", deployment, nil), "an operator can deploy")
	assert.Equal(t, 403, e.callAs(t, operator, "POST", "/api/v1/projects/"+f.project+"/enrollment-keys", map[string]string{"name": "x"}, nil),
		"keys need the admin role")

	var entries []struct {
		Actor   string         `json:"actor"`
		Action  string         `json:"action"`
		Status  int            `json:"status"`
		Details map[string]any `json:"details"`
	}
	assert.Equal(t, 403, e.callAs(t, viewer, "GET", "/api/v1/audit?project_id="+f.project, nil, nil), "the audit log needs the admin role")
	require.Equal(t, 200, e.call(t, "GET", "/api/v1/audit?project_id="+f.project+"&outcome=denied&action=POST%20/deployments", nil, &entries))
	require.Len(t, entries, 1)
	assert.True(t, strings.HasPrefix(entries[0].Actor, "vera-"))
	assert.Equal(t, 403, entries[0].Status)
	assert.Equal(t, "denied", entries[0].Details["outcome"])

	req, err := http.NewRequest("GET", e.api+"/api/v1/audit/export?project_id="+f.project, http.NoBody)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+e.key)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	csv, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "text/csv; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Contains(t, string(csv), "recorded_at,actor,action,status,outcome")
	assert.Contains(t, string(csv), "POST /deployments,403,denied")
	assert.Contains(t, string(csv), "POST /deployments,201,allowed")
}
