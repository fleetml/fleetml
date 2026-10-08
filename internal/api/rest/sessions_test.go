package rest

import (
	"context"
	"encoding/json"
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

type fakeSessionStore struct {
	mu       sync.Mutex
	keys     *fakeKeyStore
	sessions map[string]*auth.Session // by token hash
}

func (f *fakeSessionStore) Create(_ context.Context, apiKeyID uuid.UUID, tokenHash, ua, ip string, expiresAt time.Time) (*auth.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := &auth.Session{ID: uuid.New(), APIKeyID: apiKeyID, UserAgent: ua, IP: ip, CreatedAt: time.Now(), LastSeenAt: time.Now(), ExpiresAt: expiresAt}
	f.sessions[tokenHash] = s
	return s, nil
}

func (f *fakeSessionStore) GetActiveByHash(_ context.Context, tokenHash string) (*auth.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[tokenHash]
	if !ok || s.RevokedAt != nil || time.Now().After(s.ExpiresAt) || f.keys.keys[s.APIKeyID].RevokedAt != nil {
		return nil, fmt.Errorf("lookup: %w", auth.ErrNotFound)
	}
	cp := *s
	return &cp, nil
}

func (f *fakeSessionStore) ListActive(_ context.Context) ([]auth.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []auth.Session{}
	for _, s := range f.sessions {
		if s.RevokedAt == nil {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (f *fakeSessionStore) Revoke(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sessions {
		if s.ID == id {
			now := time.Now()
			s.RevokedAt = &now
			return nil
		}
	}
	return auth.ErrNotFound
}

func (f *fakeSessionStore) Touch(context.Context, uuid.UUID) error { return nil }

type sessionRig struct {
	r     *gin.Engine
	keys  *auth.APIKeyService
	store *fakeSessionStore
	admin string
}

func newSessionRig(t *testing.T, ttl time.Duration) *sessionRig {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ks := newFakeKeyStore()
	keys := auth.NewAPIKeyService(ks)
	admin, _, err := keys.Create(context.Background(), "admin")
	require.NoError(t, err)
	store := &fakeSessionStore{keys: ks, sessions: map[string]*auth.Session{}}
	sessions := auth.NewSessionService(store, keys, "test-secret-test-secret-test-secret", ttl)
	r := gin.New()
	api := r.Group("/api/v1", RequireAuth(keys, sessions, logger))
	NewSessionHandler(sessions, logger).RegisterRoutes(r.Group("/api/v1"), api)
	NewAPIKeyHandler(keys, logger).RegisterRoutes(api)
	api.GET("/projects", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"data": []string{}}) })
	api.POST("/projects", func(c *gin.Context) { c.JSON(http.StatusCreated, gin.H{"data": gin.H{}}) })
	return &sessionRig{r: r, keys: keys, store: store, admin: admin}
}

type browser struct {
	cookie *http.Cookie
	csrf   string
}

func (s *sessionRig) do(method, path string, b *browser, csrf, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if b != nil && b.cookie != nil {
		req.AddCookie(b.cookie)
	}
	if csrf != "" {
		req.Header.Set(CSRFHeader, csrf)
	}
	w := httptest.NewRecorder()
	s.r.ServeHTTP(w, req)
	return w
}

func (s *sessionRig) login(t *testing.T, key string) *browser {
	t.Helper()
	w := s.do("POST", "/api/v1/auth/login", nil, "", `{"api_key":"`+key+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	return &browser{cookie: cookies[0], csrf: resp.Data.CSRFToken}
}

func TestSession_LoginCookieAndCSRF(t *testing.T) {
	s := newSessionRig(t, time.Hour)
	assert.Equal(t, http.StatusUnauthorized, s.do("POST", "/api/v1/auth/login", nil, "", `{"api_key":"wrong"}`).Code)

	b := s.login(t, s.admin)
	c := b.cookie
	assert.Equal(t, SessionCookie, c.Name)
	assert.True(t, c.HttpOnly, "httpOnly")
	assert.Equal(t, http.SameSiteStrictMode, c.SameSite)
	assert.Equal(t, "/", c.Path)
	assert.NotContains(t, c.Value, s.admin, "the cookie is not the API key")
	assert.InDelta(t, time.Hour.Seconds(), float64(c.MaxAge), 5)
	assert.NotEmpty(t, b.csrf)

	assert.Equal(t, http.StatusOK, s.do("GET", "/api/v1/projects", b, "", "").Code, "reads need no CSRF token")
	w := s.do("POST", "/api/v1/projects", b, "", "{}")
	assert.Equal(t, http.StatusForbidden, w.Code, "writes need the CSRF token")
	assert.Contains(t, w.Body.String(), "CSRF_INVALID")
	assert.Equal(t, http.StatusForbidden, s.do("POST", "/api/v1/projects", b, "forged", "{}").Code)
	assert.Equal(t, http.StatusCreated, s.do("POST", "/api/v1/projects", b, b.csrf, "{}").Code)

	// A second login is a new session with its own token: the old CSRF token is stale for it.
	b2 := s.login(t, s.admin)
	assert.NotEqual(t, b.csrf, b2.csrf)
	assert.Equal(t, http.StatusForbidden, s.do("POST", "/api/v1/projects", b2, b.csrf, "{}").Code, "stale CSRF token")
	assert.Equal(t, http.StatusCreated, s.do("POST", "/api/v1/projects", b2, b2.csrf, "{}").Code)

	// After a reload the dashboard gets its CSRF token back from the cookie.
	var cur struct {
		Data struct {
			CSRFToken string `json:"csrf_token"`
		} `json:"data"`
	}
	w = s.do("GET", "/api/v1/auth/session", b, "", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cur))
	assert.Equal(t, b.csrf, cur.Data.CSRFToken)

	// The API key still works without a cookie or CSRF token.
	req := httptest.NewRequest("POST", "/api/v1/projects", strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+s.admin)
	rec := httptest.NewRecorder()
	s.r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, http.StatusNotFound, func() int {
		req := httptest.NewRequest("GET", "/api/v1/auth/session", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+s.admin)
		rec := httptest.NewRecorder()
		s.r.ServeHTTP(rec, req)
		return rec.Code
	}(), "an API key has no session")
}

func TestSession_RevokeLogoutAndKeyRevocation(t *testing.T) {
	s := newSessionRig(t, time.Hour)
	a, b := s.login(t, s.admin), s.login(t, s.admin)

	var list struct {
		Data []struct {
			ID      string `json:"id"`
			Current bool   `json:"current"`
		} `json:"data"`
	}
	w := s.do("GET", "/api/v1/auth/sessions", b, "", "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Data, 2)
	var aID string
	for _, x := range list.Data {
		if !x.Current {
			aID = x.ID
		}
	}
	require.NotEmpty(t, aID)

	// b revokes a: a's next request is 401 SESSION_EXPIRED and its cookie is cleared.
	assert.Equal(t, http.StatusNoContent, s.do("POST", "/api/v1/auth/sessions/"+aID+"/revoke", b, b.csrf, "").Code)
	w = s.do("GET", "/api/v1/projects", a, "", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "SESSION_EXPIRED")
	require.Len(t, w.Result().Cookies(), 1)
	assert.Equal(t, -1, w.Result().Cookies()[0].MaxAge, "the cookie is cleared")
	assert.Equal(t, http.StatusOK, s.do("GET", "/api/v1/projects", b, "", "").Code)

	// Logout ends b.
	assert.Equal(t, http.StatusForbidden, s.do("POST", "/api/v1/auth/logout", b, "", "").Code, "logout needs the CSRF token too")
	assert.Equal(t, http.StatusNoContent, s.do("POST", "/api/v1/auth/logout", b, b.csrf, "").Code)
	assert.Equal(t, http.StatusUnauthorized, s.do("GET", "/api/v1/projects", b, "", "").Code)

	// Revoking the API key ends its sessions.
	second, rec, err := s.keys.Create(context.Background(), "second")
	require.NoError(t, err)
	c := s.login(t, second)
	assert.Equal(t, http.StatusOK, s.do("GET", "/api/v1/projects", c, "", "").Code)
	require.NoError(t, s.keys.Revoke(context.Background(), rec.ID))
	assert.Equal(t, http.StatusUnauthorized, s.do("GET", "/api/v1/projects", c, "", "").Code)
}

func TestSession_Expires(t *testing.T) {
	s := newSessionRig(t, 50*time.Millisecond)
	b := s.login(t, s.admin)
	assert.Equal(t, http.StatusOK, s.do("GET", "/api/v1/projects", b, "", "").Code)
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, http.StatusUnauthorized, s.do("GET", "/api/v1/projects", b, "", "").Code)
}
