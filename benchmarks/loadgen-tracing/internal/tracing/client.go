// Package tracing implements the Zipkin v2 span model plus HTTP ingest and
// query clients for the two benchmark targets. Both Zipkin and Jaeger accept
// the same Zipkin v2 JSON on POST /api/v2/spans (Jaeger via its
// COLLECTOR_ZIPKIN_HOST_PORT collector), so ingest is shared; only the query
// API paths differ between the backends.
package tracing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Endpoint is a Zipkin localEndpoint carrying the emitting service name.
type Endpoint struct {
	ServiceName string `json:"serviceName"`
}

// Span is a single Zipkin v2 span. Timestamp and Duration are microseconds.
type Span struct {
	TraceID       string            `json:"traceId"`
	ID            string            `json:"id"`
	ParentID      string            `json:"parentId,omitempty"`
	Name          string            `json:"name"`
	Timestamp     int64             `json:"timestamp"`
	Duration      int64             `json:"duration"`
	Kind          string            `json:"kind,omitempty"`
	LocalEndpoint Endpoint          `json:"localEndpoint"`
	Tags          map[string]string `json:"tags,omitempty"`
}

// Client speaks to one backend ("jaeger" | "zipkin"). Jaeger separates its
// ingest port (Zipkin collector) from its query port, so the two addresses are
// tracked independently; for Zipkin they are the same.
type Client struct {
	db         string
	ingestAddr string
	queryAddr  string
	http       *http.Client
}

// New builds a Client. ingestAddr is the base URL of the Zipkin-format
// collector; queryAddr is the base URL of the query API. If queryAddr is
// empty it defaults to ingestAddr.
func New(db, ingestAddr, queryAddr string) *Client {
	if queryAddr == "" {
		queryAddr = ingestAddr
	}
	return &Client{
		db:         db,
		ingestAddr: ingestAddr,
		queryAddr:  queryAddr,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

// EncodeSpans marshals a batch of spans into the Zipkin v2 JSON array body.
func EncodeSpans(spans []Span) ([]byte, error) {
	return json.Marshal(spans)
}

// Push ingests a batch of spans via POST /api/v2/spans.
func (c *Client) Push(ctx context.Context, spans []Span) error {
	body, err := EncodeSpans(spans)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.ingestAddr+"/api/v2/spans", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, nil)
}

// Services lists known service names.
func (c *Client) Services(ctx context.Context) error {
	return c.get(ctx, c.servicesURL())
}

// Traces finds recent traces for a service.
func (c *Client) Traces(ctx context.Context, service string, limit int) error {
	return c.get(ctx, c.tracesURL(service, limit))
}

// Operations lists the operation (span) names seen for a service.
func (c *Client) Operations(ctx context.Context, service string) error {
	return c.get(ctx, c.operationsURL(service))
}

// Trace fetches a single trace by id.
func (c *Client) Trace(ctx context.Context, traceID string) error {
	return c.get(ctx, c.traceURL(traceID))
}

// SampleTraceID returns a real trace id for service by reading the first trace
// out of a find-traces response, so downstream point-lookups use an id in the
// exact form the backend stores. Returns "" if no traces are indexed yet.
func (c *Client) SampleTraceID(ctx context.Context, service string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.tracesURL(service, 1), nil)
	if err != nil {
		return "", err
	}
	var raw json.RawMessage
	if err := c.do(req, &raw); err != nil {
		return "", err
	}
	return firstTraceID(c.db, raw), nil
}

func (c *Client) servicesURL() string {
	if c.db == "jaeger" {
		return c.queryAddr + "/api/services"
	}
	return c.queryAddr + "/api/v2/services"
}

func (c *Client) tracesURL(service string, limit int) string {
	esc := url.QueryEscape(service)
	lim := strconv.Itoa(limit)
	if c.db == "jaeger" {
		return c.queryAddr + "/api/traces?service=" + esc + "&limit=" + lim
	}
	return c.queryAddr + "/api/v2/traces?serviceName=" + esc + "&limit=" + lim
}

func (c *Client) operationsURL(service string) string {
	if c.db == "jaeger" {
		return c.queryAddr + "/api/services/" + url.PathEscape(service) + "/operations"
	}
	return c.queryAddr + "/api/v2/spans?serviceName=" + url.QueryEscape(service)
}

func (c *Client) traceURL(traceID string) string {
	if c.db == "jaeger" {
		return c.queryAddr + "/api/traces/" + url.PathEscape(traceID)
	}
	return c.queryAddr + "/api/v2/trace/" + url.PathEscape(traceID)
}

func (c *Client) get(ctx context.Context, u string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return c.do(req, nil)
}

// do executes req, requires a 2xx status, and optionally decodes the body.
func (c *Client) do(req *http.Request, out interface{}) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: status %d: %s",
			req.Method, req.URL.Path, resp.StatusCode, bytes.TrimSpace(snippet))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// firstTraceID extracts the first trace id from a find-traces response. Zipkin
// returns [[{"traceId":...}]]; Jaeger returns {"data":[{"traceID":...}]}.
func firstTraceID(db string, raw json.RawMessage) string {
	if db == "jaeger" {
		var r struct {
			Data []struct {
				TraceID string `json:"traceID"`
			} `json:"data"`
		}
		if json.Unmarshal(raw, &r) == nil && len(r.Data) > 0 {
			return r.Data[0].TraceID
		}
		return ""
	}
	var traces [][]struct {
		TraceID string `json:"traceId"`
	}
	if json.Unmarshal(raw, &traces) == nil && len(traces) > 0 && len(traces[0]) > 0 {
		return traces[0][0].TraceID
	}
	return ""
}
