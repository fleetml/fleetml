package rest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/fleetml/fleetml/internal/auth"
)

type fakeKeyStore struct {
	mu     sync.Mutex
	keys   map[uuid.UUID]*auth.APIKey
	hashes map[uuid.UUID]string
}

func newFakeKeyStore() *fakeKeyStore {
	return &fakeKeyStore{keys: map[uuid.UUID]*auth.APIKey{}, hashes: map[uuid.UUID]string{}}
}

func (f *fakeKeyStore) Create(_ context.Context, userID uuid.UUID, name, keyHash string) (*auth.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := &auth.APIKey{ID: uuid.New(), UserID: userID, Name: name, CreatedAt: time.Now()}
	f.keys[k.ID] = k
	f.hashes[k.ID] = keyHash
	return k, nil
}

func (f *fakeKeyStore) GetActiveByHash(_ context.Context, keyHash string) (*auth.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, h := range f.hashes {
		if h == keyHash && f.keys[id].RevokedAt == nil {
			return f.keys[id], nil
		}
	}
	return nil, fmt.Errorf("lookup: %w", auth.ErrNotFound)
}

func (f *fakeKeyStore) List(_ context.Context) ([]auth.APIKey, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []auth.APIKey{}
	for _, k := range f.keys {
		out = append(out, *k)
	}
	return out, nil
}

func (f *fakeKeyStore) Revoke(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, ok := f.keys[id]
	if !ok {
		return auth.ErrNotFound
	}
	active := 0
	for _, other := range f.keys {
		if other.RevokedAt == nil && other.ID != id {
			active++
		}
	}
	if active == 0 {
		return auth.ErrLastKey
	}
	now := time.Now()
	k.RevokedAt = &now
	return nil
}

func (f *fakeKeyStore) Touch(context.Context, uuid.UUID) error { return nil }

func (f *fakeKeyStore) Count(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.keys), nil
}

// newAuthTestRouter mirrors the production wiring: /healthz open, /api/v1 behind the key middleware.
func newAuthTestRouter(t *testing.T) (r *gin.Engine, svc *auth.APIKeyService, adminKey string) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc = auth.NewAPIKeyService(newFakeKeyStore())
	adminKey, _, err := svc.Create(context.Background(), auth.BootstrapUserID, "admin")
	require.NoError(t, err)

	r = gin.New()
	r.Use(RequestID())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	api := r.Group("/api/v1", RequireAuth(svc, nil, logger))
	api.GET("/projects", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"data": []string{}}) })
	api.DELETE("/projects/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	NewAPIKeyHandler(svc, logger).RegisterRoutes(api)
	return r, svc, adminKey
}

func doAuth(r *gin.Engine, method, path, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, http.NoBody)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequireAuth_APIKeyAllow(t *testing.T) {
	r, _, key := newAuthTestRouter(t)
	assert.Equal(t, http.StatusOK, doAuth(r, "GET", "/api/v1/projects", "Bearer "+key).Code)
	assert.Equal(t, http.StatusOK, doAuth(r, "GET", "/api/v1/projects", "bearer "+key).Code, "scheme is case-insensitive")
	assert.Equal(t, http.StatusNoContent, doAuth(r, "DELETE", "/api/v1/projects/x", "Bearer "+key).Code)
	assert.Equal(t, http.StatusOK, doAuth(r, "GET", "/healthz", "").Code, "/healthz needs no key")
}

func TestRequireAuth_APIKeyDeny(t *testing.T) {
	r, _, key := newAuthTestRouter(t)
	cases := map[string]string{
		"no header":     "",
		"wrong key":     "Bearer not-a-real-key",
		"empty bearer":  "Bearer ",
		"basic scheme":  "Basic " + key,
		"key no scheme": key,
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			w := doAuth(r, "DELETE", "/api/v1/projects/x", header)
			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.Contains(t, w.Header().Get("WWW-Authenticate"), "Bearer")
			assert.Contains(t, w.Body.String(), "UNAUTHORIZED")
		})
	}
}

func TestRequireAuth_APIKeyRevoked(t *testing.T) {
	r, svc, admin := newAuthTestRouter(t)
	ctx := context.Background()
	second, rec, err := svc.Create(ctx, auth.BootstrapUserID, "second")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, doAuth(r, "GET", "/api/v1/projects", "Bearer "+second).Code)

	w := doAuth(r, "POST", "/api/v1/api-keys/"+rec.ID.String()+"/revoke", "Bearer "+admin)
	require.Equal(t, http.StatusNoContent, w.Code)

	assert.Equal(t, http.StatusUnauthorized, doAuth(r, "GET", "/api/v1/projects", "Bearer "+second).Code)
	assert.Equal(t, http.StatusOK, doAuth(r, "GET", "/api/v1/projects", "Bearer "+admin).Code)
}

func TestAPIKeyHandler_CreateListAndLastKeyGuard(t *testing.T) {
	r, _, admin := newAuthTestRouter(t)

	assert.Equal(t, http.StatusUnauthorized, doAuth(r, "GET", "/api/v1/api-keys", "").Code, "key management needs a key")

	req := httptest.NewRequest("POST", "/api/v1/api-keys", strings.NewReader(`{"name":"ci"}`))
	req.Header.Set("Authorization", "Bearer "+admin)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	body := parseBody(t, w)
	data := body["data"].(map[string]interface{})
	newKey, _ := data["key"].(string)
	assert.NotEmpty(t, newKey, "the plaintext key is returned on create")
	assert.Equal(t, "ci", data["name"])

	w = doAuth(r, "GET", "/api/v1/api-keys", "Bearer "+newKey)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), newKey, "list never shows a key")
	assert.NotContains(t, w.Body.String(), "hash")

	// Revoke "ci" with its own key, then the admin key cannot be revoked (last one).
	list := parseBody(t, w)["data"].([]interface{})
	var adminID, ciID string
	for _, item := range list {
		m := item.(map[string]interface{})
		if m["name"] == "admin" {
			adminID = m["id"].(string)
		} else {
			ciID = m["id"].(string)
		}
	}
	require.Equal(t, http.StatusNoContent, doAuth(r, "POST", "/api/v1/api-keys/"+ciID+"/revoke", "Bearer "+newKey).Code)
	assert.Equal(t, http.StatusConflict, doAuth(r, "POST", "/api/v1/api-keys/"+adminID+"/revoke", "Bearer "+admin).Code)
}
