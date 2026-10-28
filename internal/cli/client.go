package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps HTTP calls to the FleetML REST API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new API client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type apiResponse struct {
	Data  json.RawMessage `json:"data"`
	Error *apiError       `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SetAPIKey sets the admin API key sent as a Bearer token on API requests.
func (c *Client) SetAPIKey(key string) {
	c.apiKey = key
}

// authorize adds the API key header. It is never added to presigned storage URLs.
func (c *Client) authorize(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// errUnauthorized explains how to fix a 401 and names the server that
// refused, since a key that is right for another server reads the same.
func errUnauthorized(server, serverMsg string, haveKey bool) error {
	if !haveKey {
		serverMsg += "; no API key is set: run `fleetml config set-key` with your key, or set FLEETML_API_KEY in your environment"
	}
	return fmt.Errorf("unauthorized by %s (%s): set an admin API key for that server with `fleetml config set-key` or FLEETML_API_KEY, "+
		"or point the CLI at the right server (FLEETML_SERVER, `fleetml config set server <URL>` or --server)", server, serverMsg)
}

// Get performs a GET request and unmarshals the data field.
func (c *Client) Get(path string, result interface{}) error {
	return c.do("GET", path, nil, result)
}

// Post performs a POST request.
func (c *Client) Post(path string, body, result interface{}) error {
	return c.do("POST", path, body, result)
}

// Patch performs a PATCH request.
func (c *Client) Patch(path string, body, result interface{}) error {
	return c.do("PATCH", path, body, result)
}

// Delete performs a DELETE request.
func (c *Client) Delete(path string) error {
	return c.do("DELETE", path, nil, nil)
}

// PutRaw performs a raw PUT request (for presigned URL uploads).
func (c *Client) PutRaw(url string, body io.Reader, contentLength int64) error {
	req, err := http.NewRequest("PUT", url, body)
	if err != nil {
		return err
	}
	req.ContentLength = contentLength
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("upload failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) do(method, path string, body, result interface{}) error {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshalling request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("server unavailable: %w (is it running? FLEETML_SERVER, `fleetml config set server <URL>` or --server pick the server)", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
		}
		return fmt.Errorf("invalid response: %s", string(respBody))
	}

	if apiResp.Error != nil {
		if resp.StatusCode == http.StatusUnauthorized {
			return errUnauthorized(c.baseURL, apiResp.Error.Message, c.apiKey != "")
		}
		return fmt.Errorf("%s", apiResp.Error.Message)
	}

	if result != nil && apiResp.Data != nil {
		if err := json.Unmarshal(apiResp.Data, result); err != nil {
			return fmt.Errorf("parsing response data: %w", err)
		}
	}

	return nil
}

// GetRaw performs a GET and returns the full response body.
func (c *Client) GetRaw(path string) (json.RawMessage, error) {
	req, err := http.NewRequest("GET", c.baseURL+path, http.NoBody)
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("server unavailable: %w (is it running? FLEETML_SERVER, `fleetml config set server <URL>` or --server pick the server)", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return body, nil
}
