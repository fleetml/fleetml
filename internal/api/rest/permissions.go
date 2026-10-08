package rest

import "github.com/fleetml/fleetml/internal/auth"

// role orders the project roles; a higher role may do what a lower one may.
type role int

const (
	anyone   role = iota // any authenticated user
	viewer               // auth.RoleViewer in the project
	operator             // auth.RoleOperator in the project
	admin                // auth.RoleAdmin in the project
)

var roleRank = map[string]role{auth.RoleViewer: viewer, auth.RoleOperator: operator, auth.RoleAdmin: admin}

func (r role) String() string {
	return [...]string{"any user", auth.RoleViewer, auth.RoleOperator, auth.RoleAdmin}[r]
}

// scope says where a request's project comes from.
type scope int

const (
	scopeGlobal   scope = iota // no project: global admins only
	scopeSelf                  // the caller's own session: any authenticated user
	scopeList                  // a list across projects: the handler keeps the caller's projects
	scopeProject               // the :project_id path parameter
	scopeResource              // a path parameter naming a resource of a project
	scopeBody                  // a field of the JSON body
	scopeQuery                 // a query parameter; without it, global admins only
)

// rule is one row of the permission matrix: what a route needs.
type rule struct {
	scope scope
	role  role
	param string // the parameter or body field for scopeResource, scopeBody, scopeQuery
	kind  string // the resource kind the parameter names (see resolveQueries)
}

func global() rule { return rule{scope: scopeGlobal, role: admin} }
func self() rule   { return rule{scope: scopeSelf} }
func list() rule   { return rule{scope: scopeList} }
func project(r role) rule {
	return rule{scope: scopeProject, role: r, param: "project_id", kind: "project"}
}
func resource(param, kind string, r role) rule {
	return rule{scope: scopeResource, role: r, param: param, kind: kind}
}
func body(field, kind string, r role) rule {
	return rule{scope: scopeBody, role: r, param: field, kind: kind}
}
func query(param, kind string, r role) rule {
	return rule{scope: scopeQuery, role: r, param: param, kind: kind}
}

// permissions is the permission matrix: every route under /api/v1 except the
// login. A route that is not listed is refused (TestPermissions_EveryRoute
// checks that the matrix and the router agree).
var permissions = map[string]rule{
	// Global: users, API keys, creating projects.
	"GET /users":                    global(),
	"POST /users":                   global(),
	"POST /users/:user_id/api-keys": global(),
	"GET /api-keys":                 global(),
	"POST /api-keys":                global(),
	"POST /api-keys/:key_id/revoke": global(),
	"POST /projects":                global(),
	"GET /audit":                    query("project_id", "project", admin),
	"GET /audit/export":             query("project_id", "project", admin),

	// The caller's own sessions; global admins see everyone's.
	"GET /auth/session":                      self(),
	"POST /auth/logout":                      self(),
	"GET /auth/sessions":                     self(),
	"POST /auth/sessions/:session_id/revoke": self(),

	// Lists the handlers narrow to the caller's projects; device classes are hardware types, not project data.
	"GET /projects":                 list(),
	"GET /devices":                  list(),
	"GET /deployments":              list(),
	"GET /experiments":              list(),
	"GET /device-classes":           list(),
	"GET /device-classes/:class_id": list(),

	// Projects.
	"GET /projects/:project_id":                     project(viewer),
	"PUT /projects/:project_id":                     project(admin),
	"DELETE /projects/:project_id":                  project(admin),
	"GET /projects/by-slug/:slug":                   resource("slug", "slug", viewer),
	"GET /projects/:project_id/models":              project(viewer),
	"POST /projects/:project_id/models":             project(operator),
	"GET /projects/:project_id/enrollment-keys":     project(admin),
	"POST /projects/:project_id/enrollment-keys":    project(admin),
	"POST /enrollment-keys/:key_id/revoke":          resource("key_id", "enrollment_key", admin),
	"POST /enrollment-keys/:key_id/rotate":          resource("key_id", "enrollment_key", admin),
	"GET /projects/:project_id/signing-key":         project(viewer),
	"POST /projects/:project_id/signing-key":        project(admin),
	"POST /projects/:project_id/signing-key/rotate": project(admin),
	"GET /projects/:project_id/members":             project(admin),
	"PUT /projects/:project_id/members":             project(admin),
	"DELETE /projects/:project_id/members/:user_id": project(admin),

	// Alert sinks.
	"GET /projects/:project_id/alert-sinks":  project(viewer),
	"POST /projects/:project_id/alert-sinks": project(admin),
	"GET /alert-sinks/:sink_id":              resource("sink_id", "alert_sink", viewer),
	"PATCH /alert-sinks/:sink_id":            resource("sink_id", "alert_sink", admin),
	"DELETE /alert-sinks/:sink_id":           resource("sink_id", "alert_sink", admin),
	"POST /alert-sinks/:sink_id/test":        resource("sink_id", "alert_sink", operator),
	"GET /alert-sinks/:sink_id/deliveries":   resource("sink_id", "alert_sink", viewer),

	// Models, versions, variants.
	"GET /models/:model_id":               resource("model_id", "model", viewer),
	"PUT /models/:model_id":               resource("model_id", "model", operator),
	"DELETE /models/:model_id":            resource("model_id", "model", admin),
	"GET /models/:model_id/versions":      resource("model_id", "model", viewer),
	"POST /models/:model_id/versions":     resource("model_id", "model", operator),
	"GET /models/:model_id/features":      resource("model_id", "model", viewer),
	"PUT /models/:model_id/features":      resource("model_id", "model", operator),
	"GET /versions/:version_id":           resource("version_id", "version", viewer),
	"DELETE /versions/:version_id":        resource("version_id", "version", admin),
	"PATCH /versions/:version_id/status":  resource("version_id", "version", operator),
	"GET /versions/:version_id/variants":  resource("version_id", "version", viewer),
	"POST /versions/:version_id/variants": resource("version_id", "version", operator),
	"GET /variants/:variant_id":           resource("variant_id", "variant", viewer),
	"DELETE /variants/:variant_id":        resource("variant_id", "variant", admin),
	"GET /variants/:variant_id/download":  resource("variant_id", "variant", viewer),
	"POST /variants/:variant_id/finalize": resource("variant_id", "variant", operator),

	// Deployments.
	"POST /deployments":                         body("project_id", "project", operator),
	"GET /deployments/:deployment_id":           resource("deployment_id", "deployment", viewer),
	"DELETE /deployments/:deployment_id":        resource("deployment_id", "deployment", admin),
	"GET /deployments/:deployment_id/health":    resource("deployment_id", "deployment", viewer),
	"POST /deployments/:deployment_id/start":    resource("deployment_id", "deployment", operator),
	"POST /deployments/:deployment_id/pause":    resource("deployment_id", "deployment", operator),
	"POST /deployments/:deployment_id/resume":   resource("deployment_id", "deployment", operator),
	"POST /deployments/:deployment_id/rollback": resource("deployment_id", "deployment", operator),
	"POST /deployments/:deployment_id/cancel":   resource("deployment_id", "deployment", operator),
	"POST /deployments/:deployment_id/progress": resource("deployment_id", "deployment", operator),
	"PATCH /deployments/:deployment_id/status":  resource("deployment_id", "deployment", operator),
	"POST /intelligence/preview":                body("deployment_id", "deployment", viewer),

	// Experiments.
	"POST /experiments":                         body("project_id", "project", operator),
	"GET /experiments/:experiment_id":           resource("experiment_id", "experiment", viewer),
	"DELETE /experiments/:experiment_id":        resource("experiment_id", "experiment", admin),
	"GET /experiments/:experiment_id/results":   resource("experiment_id", "experiment", viewer),
	"GET /experiments/:experiment_id/cohorts":   resource("experiment_id", "experiment", viewer),
	"POST /experiments/:experiment_id/start":    resource("experiment_id", "experiment", operator),
	"POST /experiments/:experiment_id/pause":    resource("experiment_id", "experiment", operator),
	"POST /experiments/:experiment_id/resume":   resource("experiment_id", "experiment", operator),
	"POST /experiments/:experiment_id/cancel":   resource("experiment_id", "experiment", operator),
	"POST /experiments/:experiment_id/conclude": resource("experiment_id", "experiment", operator),

	// Devices.
	"GET /devices/:device_id":         resource("device_id", "device", viewer),
	"DELETE /devices/:device_id":      resource("device_id", "device", admin),
	"GET /devices/:device_id/profile": resource("device_id", "device", viewer),
	"POST /devices/:device_id/revoke": resource("device_id", "device", admin),

	// Telemetry and drift, by model version.
	"GET /telemetry/events":                             query("model_version_id", "version", viewer),
	"GET /telemetry/summary/:model_version_id":          resource("model_version_id", "version", viewer),
	"GET /telemetry/summary/:model_version_id/by-class": resource("model_version_id", "version", viewer),
	"GET /telemetry/timeseries/:model_version_id":       resource("model_version_id", "version", viewer),
	"GET /drift/:model_version_id":                      resource("model_version_id", "version", viewer),
	"POST /drift/:model_version_id/check":               resource("model_version_id", "version", operator),
	"GET /drift/:model_version_id/history":              resource("model_version_id", "version", viewer),
	"GET /drift/:model_version_id/points":               resource("model_version_id", "version", viewer),
}

// resolveQueries find the project of a resource, by kind.
var resolveQueries = map[string]string{
	"project":        `SELECT id::text FROM projects WHERE id = $1::uuid`,
	"slug":           `SELECT id::text FROM projects WHERE slug = $1`,
	"model":          `SELECT project_id::text FROM models WHERE id = $1::uuid`,
	"version":        `SELECT m.project_id::text FROM model_versions v JOIN models m ON m.id = v.model_id WHERE v.id = $1::uuid`,
	"variant":        `SELECT m.project_id::text FROM model_variants mv JOIN model_versions v ON v.id = mv.version_id JOIN models m ON m.id = v.model_id WHERE mv.id = $1::uuid`,
	"deployment":     `SELECT project_id::text FROM deployments WHERE id = $1::uuid`,
	"experiment":     `SELECT project_id::text FROM experiments WHERE id = $1::uuid`,
	"device":         `SELECT project_id::text FROM devices WHERE id = $1::uuid`,
	"alert_sink":     `SELECT project_id::text FROM alert_sinks WHERE id = $1::uuid`,
	"enrollment_key": `SELECT project_id::text FROM enrollment_keys WHERE id = $1::uuid`,
}
