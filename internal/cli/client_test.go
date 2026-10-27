package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Get_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{"name": "test"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	var result map[string]string
	err := c.Get("/test", &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])
}

func TestClient_Get_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"code": "NOT_FOUND", "message": "project not found"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	var result map[string]string
	err := c.Get("/test", &result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project not found")
}

func TestClient_Delete_NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.Delete("/test")
	assert.NoError(t, err)
}

func TestClient_Post_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]string{"id": "123"},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	var result map[string]string
	err := c.Post("/test", map[string]string{"name": "new"}, &result)
	require.NoError(t, err)
	assert.Equal(t, "123", result["id"])
}

func TestClient_SendsAPIKey(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": map[string]string{}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.SetAPIKey("k123")
	require.NoError(t, c.Get("/a", nil))
	require.NoError(t, c.Post("/b", map[string]string{}, nil))
	_, err := c.GetRaw("/c")
	require.NoError(t, err)
	assert.Equal(t, []string{"Bearer k123", "Bearer k123", "Bearer k123"}, got)
}

func TestClient_PutRawNeverSendsAPIKey(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
	}))
	defer srv.Close()

	c := NewClient("http://unused")
	c.SetAPIKey("k123")
	require.NoError(t, c.PutRaw(srv.URL+"/presigned", strings.NewReader("blob"), 4))
	assert.Empty(t, got, "the admin key must not leak to the storage endpoint")
}

func TestClient_UnauthorizedHint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{"code": "UNAUTHORIZED", "message": "missing API key"},
		})
	}))
	defer srv.Close()

	err := NewClient(srv.URL).Get("/x", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fleetml config set-key")
	assert.Contains(t, err.Error(), "unauthorized by "+srv.URL, "names the server that refused")
}
