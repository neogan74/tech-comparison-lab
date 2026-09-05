// Package logs implements a backend-neutral log entry model plus HTTP ingest
// and query clients for the two benchmark targets. Unlike the tracing loadgen,
// Loki and Elasticsearch share no wire format: Loki takes label-keyed streams
// of (nanosecond, line) pairs on POST /loki/api/v1/push, Elasticsearch takes
// NDJSON action/document pairs on POST /_bulk. Both encoders therefore live
// here and are selected by backend, while the query surface is normalised to
// the four operations the benchmark compares.
package logs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Index is the Elasticsearch index the benchmark writes to and queries.
const Index = "logs-bench"

// Entry is one synthetic log line in the backend-neutral model. TimestampNs is
// wall-clock nanoseconds; Service and Level become Loki stream labels and
// Elasticsearch keyword fields, Message the log body.
type Entry struct {
	TimestampNs int64
	Service     string
	Level       string
	Message     string
}

// Client speaks to one backend ("loki" | "elasticsearch"). Both expose ingest
// and query on a single base URL, so one address is enough.
type Client struct {
	db   string
	addr string
	http *http.Client
}

// New builds a Client for db against base URL addr.
func New(db, addr string) *Client {
	return &Client{
		db:   db,
		addr: strings.TrimRight(addr, "/"),
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

// --- Ingest encoding ---

// lokiStream is one label-set keyed stream in a Loki push request. Values are
// [nanosecond-timestamp-as-string, line] pairs.
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}

// EncodeLoki groups entries into one stream per distinct (service, level) label
// pair, with each stream's values sorted ascending by timestamp — Loki rejects
// or penalises out-of-order lines within a stream depending on configuration.
func EncodeLoki(entries []Entry) ([]byte, error) {
	type key struct{ service, level string }
	byStream := make(map[key][][2]string)
	for _, e := range entries {
		k := key{e.Service, e.Level}
		byStream[k] = append(byStream[k], [2]string{
			strconv.FormatInt(e.TimestampNs, 10),
			e.Message,
		})
	}

	push := lokiPush{Streams: make([]lokiStream, 0, len(byStream))}
	for k, values := range byStream {
		sort.Slice(values, func(i, j int) bool { return values[i][0] < values[j][0] })
		push.Streams = append(push.Streams, lokiStream{
			Stream: map[string]string{"service": k.service, "level": k.level},
			Values: values,
		})
	}
	// Deterministic stream order keeps request bodies reproducible across runs.
	sort.Slice(push.Streams, func(i, j int) bool {
		a, b := push.Streams[i].Stream, push.Streams[j].Stream
		if a["service"] != b["service"] {
			return a["service"] < b["service"]
		}
		return a["level"] < b["level"]
	})
	return json.Marshal(push)
}

// EncodeBulk renders entries as an Elasticsearch _bulk NDJSON body: one
// action line per document, and a mandatory trailing newline.
func EncodeBulk(entries []Entry) ([]byte, error) {
	var buf bytes.Buffer
	action := []byte(`{"create":{}}` + "\n")
	for _, e := range entries {
		buf.Write(action)
		doc, err := json.Marshal(struct {
			Timestamp string `json:"@timestamp"`
			Service   string `json:"service"`
			Level     string `json:"level"`
			Message   string `json:"message"`
		}{
			Timestamp: time.Unix(0, e.TimestampNs).UTC().Format(time.RFC3339Nano),
			Service:   e.Service,
			Level:     e.Level,
			Message:   e.Message,
		})
		if err != nil {
			return nil, err
		}
		buf.Write(doc)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// Push ingests a batch of entries using the backend's native bulk format.
func (c *Client) Push(ctx context.Context, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	if c.db == "loki" {
		body, err := EncodeLoki(entries)
		if err != nil {
			return err
		}
		return c.post(ctx, c.addr+"/loki/api/v1/push", "application/json", body, nil)
	}

	body, err := EncodeBulk(entries)
	if err != nil {
		return err
	}
	var resp struct {
		Errors bool `json:"errors"`
		Items  []struct {
			Create struct {
				Status int             `json:"status"`
				Error  json.RawMessage `json:"error"`
			} `json:"create"`
		} `json:"items"`
	}
	url := c.addr + "/" + Index + "/_bulk"
	if err := c.post(ctx, url, "application/x-ndjson", body, &resp); err != nil {
		return err
	}
	// _bulk returns 200 even when individual documents fail; surface the first.
	if resp.Errors {
		for _, it := range resp.Items {
			if it.Create.Status >= 300 {
				return fmt.Errorf("bulk item failed: status %d: %s",
					it.Create.Status, it.Create.Error)
			}
		}
		return fmt.Errorf("bulk reported errors")
	}
	return nil
}

// --- Setup ---

// Ping verifies the backend is reachable and serving queries.
func (c *Client) Ping(ctx context.Context) error {
	if c.db == "loki" {
		return c.get(ctx, c.addr+"/ready", nil)
	}
	return c.get(ctx, c.addr+"/_cluster/health?wait_for_status=yellow&timeout=30s", nil)
}

// EnsureIndex creates the Elasticsearch index with an explicit mapping so
// service/level are aggregatable keywords and message is analysed full text.
// It is a no-op for Loki, which needs no schema.
func (c *Client) EnsureIndex(ctx context.Context) error {
	if c.db != "elasticsearch" {
		return nil
	}
	body := []byte(`{
	  "settings": {"number_of_shards": 1, "number_of_replicas": 0, "refresh_interval": "-1"},
	  "mappings": {"properties": {
	    "@timestamp": {"type": "date"},
	    "service":    {"type": "keyword"},
	    "level":      {"type": "keyword"},
	    "message":    {"type": "text"}
	  }}
	}`)
	err := c.put(ctx, c.addr+"/"+Index, "application/json", body)
	// A leftover index from an earlier run is fine; the mapping is identical.
	if err != nil && strings.Contains(err.Error(), "resource_already_exists_exception") {
		return nil
	}
	return err
}

// Flush makes freshly ingested data visible to queries. Elasticsearch ingests
// with refresh_interval disabled for write throughput, so the index must be
// refreshed explicitly; Loki serves from the in-memory head and needs nothing.
func (c *Client) Flush(ctx context.Context) error {
	if c.db != "elasticsearch" {
		return nil
	}
	return c.post(ctx, c.addr+"/"+Index+"/_refresh", "application/json", nil, nil)
}

// --- Queries ---

// LabelValues enumerates the distinct service values present in the store.
// Loki answers from its label index; Elasticsearch runs a terms aggregation.
func (c *Client) LabelValues(ctx context.Context, window time.Duration) error {
	if c.db == "loki" {
		start, end := timeRange(window)
		u := c.addr + "/loki/api/v1/label/service/values" +
			"?start=" + strconv.FormatInt(start, 10) +
			"&end=" + strconv.FormatInt(end, 10)
		return c.get(ctx, u, nil)
	}
	body := []byte(`{"size":0,"aggs":{"services":{"terms":{"field":"service","size":1000}}}}`)
	return c.post(ctx, c.addr+"/"+Index+"/_search", "application/json", body, nil)
}

// QueryRange fetches the most recent limit lines for one service — the "tail a
// service" access pattern.
func (c *Client) QueryRange(ctx context.Context, service string, limit int, window time.Duration) error {
	if c.db == "loki" {
		return c.get(ctx, c.rangeURL(fmt.Sprintf("{service=%q}", service), limit, window), nil)
	}
	body := []byte(fmt.Sprintf(`{
	  "size": %d,
	  "sort": [{"@timestamp": "desc"}],
	  "query": {"term": {"service": %q}}
	}`, limit, service))
	return c.post(ctx, c.addr+"/"+Index+"/_search", "application/json", body, nil)
}

// FilterMatch searches one service's lines for a token — Loki's line filter
// (brute-force scan over matching streams) against Elasticsearch's inverted
// index. This is the operation where the two designs differ most.
func (c *Client) FilterMatch(ctx context.Context, service, token string, limit int, window time.Duration) error {
	if c.db == "loki" {
		q := fmt.Sprintf("{service=%q} |= %q", service, token)
		return c.get(ctx, c.rangeURL(q, limit, window), nil)
	}
	body := []byte(fmt.Sprintf(`{
	  "size": %d,
	  "query": {"bool": {"filter": [
	    {"term":  {"service": %q}},
	    {"match": {"message": %q}}
	  ]}}
	}`, limit, service, token))
	return c.post(ctx, c.addr+"/"+Index+"/_search", "application/json", body, nil)
}

// CountOverTime counts one service's lines over the window: a Loki
// count_over_time instant query against an Elasticsearch _count.
func (c *Client) CountOverTime(ctx context.Context, service string, window time.Duration) error {
	if c.db == "loki" {
		q := fmt.Sprintf("count_over_time({service=%q}[%s])", service, lokiDuration(window))
		u := c.addr + "/loki/api/v1/query?query=" + url.QueryEscape(q)
		return c.get(ctx, u, nil)
	}
	body := []byte(fmt.Sprintf(`{"query":{"term":{"service":%q}}}`, service))
	return c.post(ctx, c.addr+"/"+Index+"/_count", "application/json", body, nil)
}

// CountIngested reports how many entries the backend has stored for service,
// used to verify ingest actually landed before the query phase runs.
func (c *Client) CountIngested(ctx context.Context, service string, window time.Duration) (int64, error) {
	if c.db == "loki" {
		q := fmt.Sprintf("count_over_time({service=%q}[%s])", service, lokiDuration(window))
		var resp struct {
			Data struct {
				Result []struct {
					Value [2]json.RawMessage `json:"value"`
				} `json:"result"`
			} `json:"data"`
		}
		u := c.addr + "/loki/api/v1/query?query=" + url.QueryEscape(q)
		if err := c.get(ctx, u, &resp); err != nil {
			return 0, err
		}
		if len(resp.Data.Result) == 0 {
			return 0, nil
		}
		// Instant-query sample values arrive as a quoted numeric string.
		return parseQuotedInt(resp.Data.Result[0].Value[1]), nil
	}

	var resp struct {
		Count int64 `json:"count"`
	}
	body := []byte(fmt.Sprintf(`{"query":{"term":{"service":%q}}}`, service))
	if err := c.post(ctx, c.addr+"/"+Index+"/_count", "application/json", body, &resp); err != nil {
		return 0, err
	}
	return resp.Count, nil
}

// rangeURL builds a Loki query_range request for a LogQL expression.
func (c *Client) rangeURL(query string, limit int, window time.Duration) string {
	start, end := timeRange(window)
	return c.addr + "/loki/api/v1/query_range" +
		"?query=" + url.QueryEscape(query) +
		"&limit=" + strconv.Itoa(limit) +
		"&direction=backward" +
		"&start=" + strconv.FormatInt(start, 10) +
		"&end=" + strconv.FormatInt(end, 10)
}

// timeRange returns the [now-window, now] bounds in nanoseconds, the unit
// Loki's range endpoints accept.
func timeRange(window time.Duration) (start, end int64) {
	now := time.Now()
	return now.Add(-window).UnixNano(), now.UnixNano()
}

// lokiDuration renders d as a LogQL range duration in whole seconds.
func lokiDuration(d time.Duration) string {
	secs := int64(d.Seconds())
	if secs < 1 {
		secs = 1
	}
	return strconv.FormatInt(secs, 10) + "s"
}

// parseQuotedInt reads a Prometheus-style sample value ("1234" or 1234),
// truncating any fractional part. Unparseable input yields 0.
func parseQuotedInt(raw json.RawMessage) int64 {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// --- HTTP plumbing ---

func (c *Client) get(ctx context.Context, u string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) post(ctx context.Context, u, contentType string, body []byte, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	return c.do(req, out)
}

func (c *Client) put(ctx context.Context, u, contentType string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
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
