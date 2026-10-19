// Package clitest is an in-memory fake of the part of the FleetML REST API
// that the quickstart's CLI calls use: projects, models, versions, variant
// upload and finalize, enrollment keys, deployments and devices. It answers with the same paths,
// status codes, JSON envelope ({"data": ...} or {"error": {"code", "message"}})
// and checks as internal/api/rest, so the CLI can be tested end to end where
// the real server cannot run (the Windows CI job, and the Windows run workflow
// through cmd/clitest-server). The same CLI calls run against a real server in
// cmd/fleetml's smoke test when FLEETML_SMOKE_SERVER is set.
package clitest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Server is a running fake. Its URL is the REST base URL (FLEETML_SERVER).
type Server struct {
	*httptest.Server
	apiKey string

	mu          sync.Mutex
	projects    []map[string]any
	models      []map[string]any
	versions    []map[string]any
	variants    map[string]map[string]any
	uploads     map[string][]byte
	deployments map[string]map[string]any
	devices     []map[string]any
	keys        []map[string]any
}

// NewServer starts a fake that accepts the admin key apiKey and has one
// registered device. It is closed when the test ends.
func NewServer(t testing.TB, apiKey string) *Server {
	s := newFake(apiKey)
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// NewHandler returns the same fake as NewServer as a handler, for a process
// that serves it on a port of its own (cmd/clitest-server).
func NewHandler(apiKey string) http.Handler {
	return http.HandlerFunc(newFake(apiKey).serve)
}

func newFake(apiKey string) *Server {
	return &Server{
		apiKey:      apiKey,
		variants:    map[string]map[string]any{},
		uploads:     map[string][]byte{},
		deployments: map[string]map[string]any{},
		devices: []map[string]any{{
			"id": uuid.NewString(), "os": "ANDROID", "os_version": "14", "ram_mb": 6144,
			"chipset": "simulated", "labels": map[string]string{"app": "smoke"},
		}},
	}
}

// Upload returns the bytes uploaded for a variant.
func (s *Server) Upload(variantID string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.uploads[variantID]
}

// Projects returns every project created so far.
func (s *Server) Projects() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.projects...)
}

// Variants returns every variant created so far.
func (s *Server) Variants() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]map[string]any, 0, len(s.variants))
	for _, v := range s.variants {
		out = append(out, v)
	}
	return out
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	// The probes, like the real server's, need no API key.
	if r.Method == http.MethodGet && (r.URL.Path == "/healthz" || r.URL.Path == "/readyz") {
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	// The presigned upload URL: like the object store, it must not be sent the admin key.
	if id, ok := strings.CutPrefix(r.URL.Path, "/upload/"); ok && r.Method == http.MethodPut {
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "the admin API key must not be sent to the object store", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.uploads[id] = body
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		return
	}
	path, ok := strings.CutPrefix(r.URL.Path, "/api/v1/")
	if !ok {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "no route "+r.URL.Path)
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+s.apiKey {
		respondError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or invalid API key")
		return
	}
	var body map[string]any
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := strings.Split(path, "/")
	switch {
	case r.Method == http.MethodPost && path == "projects":
		s.createProject(w, body)
	case r.Method == http.MethodGet && path == "projects":
		respondData(w, http.StatusOK, s.projects)
	case len(p) == 3 && p[0] == "projects" && p[2] == "enrollment-keys":
		s.enrollmentKeys(w, r.Method, p[1], body)
	case len(p) == 3 && p[0] == "projects" && p[2] == "models":
		s.projectModels(w, r.Method, p[1], body)
	case len(p) == 3 && p[0] == "models" && p[2] == "versions":
		s.modelVersions(w, r.Method, p[1], body)
	case r.Method == http.MethodPost && len(p) == 3 && p[0] == "versions" && p[2] == "variants":
		s.createVariant(w, r, p[1], body)
	case r.Method == http.MethodPatch && len(p) == 3 && p[0] == "versions" && p[2] == "status":
		s.setVersionStatus(w, p[1], body)
	case r.Method == http.MethodPost && len(p) == 3 && p[0] == "variants" && p[2] == "finalize":
		s.finalize(w, p[1])
	case r.Method == http.MethodPost && path == "deployments":
		s.createDeployment(w, body)
	case r.Method == http.MethodPost && len(p) == 3 && p[0] == "deployments" && p[2] == "start":
		s.startDeployment(w, p[1])
	case r.Method == http.MethodGet && len(p) == 2 && p[0] == "deployments":
		if d, ok := s.deployments[p[1]]; ok {
			respondData(w, http.StatusOK, d)
		} else {
			respondError(w, http.StatusNotFound, "NOT_FOUND", "deployment not found")
		}
	case r.Method == http.MethodGet && path == "devices":
		s.listDevices(w, r.URL.Query().Get("project_id"))
	default:
		respondError(w, http.StatusNotFound, "NOT_FOUND", "no route "+r.Method+" "+r.URL.Path)
	}
}

func (s *Server) createProject(w http.ResponseWriter, body map[string]any) {
	name, _ := body["name"].(string)
	if name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	p := map[string]any{"id": uuid.NewString(), "name": name, "slug": strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		"description": body["description"], "created_at": now()}
	s.projects = append(s.projects, p)
	respondData(w, http.StatusCreated, p)
}

func (s *Server) projectModels(w http.ResponseWriter, method, projectID string, body map[string]any) {
	if !s.hasProject(projectID) {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "project not found")
		return
	}
	if method == http.MethodGet {
		respondData(w, http.StatusOK, filter(s.models, "project_id", projectID))
		return
	}
	name, _ := body["name"].(string)
	if name == "" || body["framework"] == "" || body["task_type"] == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name, framework, and task_type are required")
		return
	}
	for _, m := range filter(s.models, "project_id", projectID) {
		if m["name"] == name {
			respondError(w, http.StatusConflict, "CONFLICT", "model "+name+" already exists")
			return
		}
	}
	m := map[string]any{"id": uuid.NewString(), "project_id": projectID, "name": name,
		"framework": body["framework"], "task_type": body["task_type"], "created_at": now()}
	s.models = append(s.models, m)
	respondData(w, http.StatusCreated, m)
}

func (s *Server) enrollmentKeys(w http.ResponseWriter, method, projectID string, body map[string]any) {
	if !s.hasProject(projectID) {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "project not found")
		return
	}
	if method == http.MethodGet {
		respondData(w, http.StatusOK, filter(s.keys, "project_id", projectID))
		return
	}
	name, _ := body["name"].(string)
	if name == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "name is required")
		return
	}
	// Like the server, only the create response carries the key; the list does not.
	k := map[string]any{"id": uuid.NewString(), "project_id": projectID, "name": name,
		"created_at": now(), "last_used_at": nil, "revoked_at": nil}
	s.keys = append(s.keys, k)
	created := map[string]any{"key": strings.ReplaceAll(uuid.NewString()+uuid.NewString(), "-", "")}
	for f, v := range k {
		created[f] = v
	}
	respondData(w, http.StatusCreated, created)
}

func (s *Server) modelVersions(w http.ResponseWriter, method, modelID string, body map[string]any) {
	if len(filter(s.models, "id", modelID)) == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "model not found")
		return
	}
	if method == http.MethodGet {
		respondData(w, http.StatusOK, filter(s.versions, "model_id", modelID))
		return
	}
	ver, _ := body["version"].(string)
	if ver == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "version is required")
		return
	}
	for _, v := range filter(s.versions, "model_id", modelID) {
		if v["version"] == ver {
			respondError(w, http.StatusConflict, "CONFLICT", "version "+ver+" already exists")
			return
		}
	}
	v := map[string]any{"id": uuid.NewString(), "model_id": modelID, "version": ver, "status": "DRAFT", "created_at": now()}
	s.versions = append(s.versions, v)
	respondData(w, http.StatusCreated, v)
}

func (s *Server) createVariant(w http.ResponseWriter, r *http.Request, versionID string, body map[string]any) {
	if len(filter(s.versions, "id", versionID)) == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "version not found")
		return
	}
	size, _ := body["blob_size_bytes"].(float64)
	sha, _ := body["blob_sha256"].(string)
	if body["quantization"] == nil || body["target_runtime"] == nil || size <= 0 || len(sha) != 64 {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "quantization, target_runtime, blob_size_bytes, and blob_sha256 are required")
		return
	}
	// Like the server: a variant that was never verified is replaced; a READY one is kept.
	for id, old := range s.variants {
		if old["model_version_id"] == versionID && old["quantization"] == body["quantization"] && old["target_runtime"] == body["target_runtime"] {
			if old["status"] == "READY" {
				respondError(w, http.StatusConflict, "CONFLICT", "already exists: variant of this version is already uploaded and verified; push a new version")
				return
			}
			delete(s.variants, id)
			delete(s.uploads, id)
		}
	}
	v := map[string]any{"id": uuid.NewString(), "model_version_id": versionID, "quantization": body["quantization"],
		"target_runtime": body["target_runtime"], "blob_size_bytes": int64(size), "blob_sha256": sha, "status": "UPLOADING"}
	s.variants[v["id"].(string)] = v
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"variant": v, "upload_url": "http://" + r.Host + "/upload/" + v["id"].(string) + "?X-Amz-Signature=fake",
	}})
}

func (s *Server) finalize(w http.ResponseWriter, variantID string) {
	v, ok := s.variants[variantID]
	if !ok {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "variant not found")
		return
	}
	blob, uploaded := s.uploads[variantID]
	sum := sha256.Sum256(blob)
	switch {
	case !uploaded:
		respondError(w, http.StatusUnprocessableEntity, "INTEGRITY_ERROR", "blob not uploaded")
		return
	case int64(len(blob)) != v["blob_size_bytes"].(int64):
		respondError(w, http.StatusUnprocessableEntity, "INTEGRITY_ERROR", "blob size does not match the declared size")
		return
	case hex.EncodeToString(sum[:]) != v["blob_sha256"]:
		respondError(w, http.StatusUnprocessableEntity, "INTEGRITY_ERROR", "blob sha256 does not match the declared sha256")
		return
	}
	v["status"] = "READY"
	respondData(w, http.StatusOK, v)
}

func (s *Server) setVersionStatus(w http.ResponseWriter, versionID string, body map[string]any) {
	vs := filter(s.versions, "id", versionID)
	if len(vs) == 0 {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "version not found")
		return
	}
	vs[0]["status"] = body["status"]
	respondData(w, http.StatusOK, vs[0])
}

func (s *Server) createDeployment(w http.ResponseWriter, body map[string]any) {
	projectID, _ := body["project_id"].(string)
	versionID, _ := body["model_version_id"].(string)
	if projectID == "" || versionID == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "project_id and model_version_id are required")
		return
	}
	ready := false
	for _, v := range s.variants {
		ready = ready || (v["model_version_id"] == versionID && v["status"] == "READY")
	}
	if !ready {
		respondError(w, http.StatusUnprocessableEntity, "NOT_DEPLOYABLE",
			"model version not deployable: model version "+versionID+" has no READY variant (upload a blob and finalize it first)")
		return
	}
	strategy, _ := body["strategy"].(string)
	if strategy == "" {
		strategy = "ALL_AT_ONCE"
	}
	d := map[string]any{"id": uuid.NewString(), "project_id": projectID, "model_version_id": versionID, "strategy": strategy,
		"status": "PENDING", "target_selector": body["target_selector"], "urgent": body["urgent"], "created_at": now()}
	s.deployments[d["id"].(string)] = d
	respondData(w, http.StatusCreated, d)
}

func (s *Server) startDeployment(w http.ResponseWriter, id string) {
	d, ok := s.deployments[id]
	if !ok {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "deployment not found")
		return
	}
	if d["status"] != "PENDING" {
		respondError(w, http.StatusConflict, "INVALID_TRANSITION", "deployment is not PENDING")
		return
	}
	// An all-at-once deployment is assigned to the whole fleet when it starts.
	d["status"] = "ROLLING_OUT"
	if d["strategy"] == "ALL_AT_ONCE" {
		d["status"] = "COMPLETE"
	}
	respondData(w, http.StatusOK, d)
}

// listDevices filters on project_id like the server: a value that is not a
// UUID is refused. The fake's device is enrolled into the first project.
func (s *Server) listDevices(w http.ResponseWriter, projectID string) {
	if projectID == "" {
		respondData(w, http.StatusOK, s.devices)
		return
	}
	if _, err := uuid.Parse(projectID); err != nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "project_id must be a UUID")
		return
	}
	out := []map[string]any{}
	if len(s.projects) > 0 && s.projects[0]["id"] == projectID {
		out = s.devices
	}
	respondData(w, http.StatusOK, out)
}

func (s *Server) hasProject(id string) bool { return len(filter(s.projects, "id", id)) > 0 }

func filter(items []map[string]any, key, value string) []map[string]any {
	out := []map[string]any{}
	for _, it := range items {
		if it[key] == value {
			out = append(out, it)
		}
	}
	return out
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func respondData(w http.ResponseWriter, status int, data any) {
	respondJSON(w, status, map[string]any{"data": data})
}

func respondError(w http.ResponseWriter, status int, code, message string) {
	respondJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func respondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
