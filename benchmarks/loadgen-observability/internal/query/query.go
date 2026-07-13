// Package query implements a minimal PromQL HTTP API client, compatible with
// both Prometheus and VictoriaMetrics single-node (/api/v1/query).
package query

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Client queries a Prometheus-API-compatible endpoint.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates a Client targeting baseURL (e.g. http://localhost:9090 or
// http://localhost:8428).
func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 30 * time.Second}}
}

type apiResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string            `json:"resultType"`
		Result     []json.RawMessage `json:"result"`
	} `json:"data"`
}

// Instant runs an instant PromQL query and returns the number of series
// in the result vector (used as a sanity check, not part of timing).
func (c *Client) Instant(ctx context.Context, expr string) (int, error) {
	u := c.baseURL + "/api/v1/query?" + url.Values{"query": {expr}}.Encode()
	return c.do(ctx, u)
}

func (c *Client) do(ctx context.Context, u string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("new request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	var out apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}
	if out.Status != "success" {
		return 0, fmt.Errorf("query failed: %s", out.Error)
	}
	return len(out.Data.Result), nil
}
