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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("server unavailable: %w", err)
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
	resp, err := c.httpClient.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("server unavailable: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return body, nil
}
