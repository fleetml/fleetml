package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
