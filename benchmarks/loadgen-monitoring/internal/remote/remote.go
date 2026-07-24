// Package remote implements a minimal Prometheus remote_write v1 client
// (protobuf + snappy over HTTP), compatible with both Prometheus
// (--web.enable-remote-write-receiver) and VictoriaMetrics single-node
// (/api/v1/write).
package remote

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang/snappy"
)

// Label is a Prometheus label name/value pair.
type Label struct {
	Name  string
	Value string
}

// Sample is a single float64 value at a millisecond timestamp.
type Sample struct {
	Value       float64
	TimestampMs int64
}

// TimeSeries is a label set plus its samples, as sent on the wire.
type TimeSeries struct {
	Labels  []Label
	Samples []Sample
}

// Client pushes batches of series to a remote_write endpoint.
type Client struct {
	writeURL string
	http     *http.Client
}

// New creates a Client targeting baseURL (e.g. http://localhost:9090 or
// http://localhost:8428), appending the standard /api/v1/write path.
func New(baseURL string) *Client {
	return &Client{
		writeURL: baseURL + "/api/v1/write",
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Push encodes series as a WriteRequest, snappy-compresses it, and POSTs it.
func (c *Client) Push(ctx context.Context, series []TimeSeries) error {
	raw := EncodeWriteRequest(series)
	compressed := snappy.Encode(nil, raw)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.writeURL, bytes.NewReader(compressed))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("remote write failed: status=%d body=%s", resp.StatusCode, body)
	}
	return nil
}
