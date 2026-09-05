package logs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testEntries() []Entry {
	return []Entry{
		{TimestampNs: 1_700_000_002_000_000_000, Service: "svc-000", Level: "info", Message: "second"},
		{TimestampNs: 1_700_000_001_000_000_000, Service: "svc-000", Level: "info", Message: "first"},
		{TimestampNs: 1_700_000_003_000_000_000, Service: "svc-001", Level: "error", Message: "other"},
	}
}

func TestEncodeLokiGroupsAndSortsStreams(t *testing.T) {
	body, err := EncodeLoki(testEntries())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var push lokiPush
	if err := json.Unmarshal(body, &push); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Two distinct (service, level) pairs => two streams, ordered by label.
	if len(push.Streams) != 2 {
		t.Fatalf("got %d streams, want 2: %s", len(push.Streams), body)
	}
	if got := push.Streams[0].Stream["service"]; got != "svc-000" {
		t.Errorf("first stream service = %q, want svc-000", got)
	}
	if got := push.Streams[1].Stream["level"]; got != "error" {
		t.Errorf("second stream level = %q, want error", got)
	}

	// Values within a stream must ascend by timestamp — Loki penalises
	// out-of-order lines.
	vals := push.Streams[0].Values
	if len(vals) != 2 {
		t.Fatalf("got %d values, want 2", len(vals))
	}
	if vals[0][0] >= vals[1][0] {
		t.Errorf("values not sorted ascending: %v", vals)
	}
	if vals[0][1] != "first" || vals[1][1] != "second" {
		t.Errorf("lines paired with wrong timestamps: %v", vals)
	}
}

func TestEncodeBulkNDJSONShape(t *testing.T) {
	body, err := EncodeBulk(testEntries())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	s := string(body)

	// _bulk requires a trailing newline, or the last action is ignored.
	if !strings.HasSuffix(s, "\n") {
		t.Errorf("bulk body must end with newline: %q", s)
	}

	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("got %d lines, want 6 (action+doc per entry): %q", len(lines), s)
	}
	for i := 0; i < len(lines); i += 2 {
		if lines[i] != `{"create":{}}` {
			t.Errorf("line %d = %q, want action line", i, lines[i])
		}
		var doc struct {
			Timestamp string `json:"@timestamp"`
			Service   string `json:"service"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal([]byte(lines[i+1]), &doc); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i+1, err)
		}
		if doc.Service == "" || doc.Message == "" || doc.Timestamp == "" {
			t.Errorf("line %d missing fields: %+v", i+1, doc)
		}
	}

	// Entry order is preserved (no grouping, unlike Loki).
	if !strings.Contains(lines[1], `"message":"second"`) {
		t.Errorf("first document should be the first entry: %q", lines[1])
	}
}

func TestParseQuotedInt(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{`"1234"`, 1234},
		{`1234`, 1234},
		{`"1234.75"`, 1234},
		{`"0"`, 0},
		{`"NaN"`, 0},
		{`null`, 0},
	}
	for _, c := range cases {
		if got := parseQuotedInt(json.RawMessage(c.in)); got != c.want {
			t.Errorf("parseQuotedInt(%s) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestBaseAddrTrimsTrailingSlash(t *testing.T) {
	c := New("loki", "http://localhost:3100/")
	if c.addr != "http://localhost:3100" {
		t.Errorf("addr = %q, want no trailing slash", c.addr)
	}
}

// recorder captures the method, path and raw query of every request so tests
// can assert which backend API each operation targets.
type recorder struct {
	srv  *httptest.Server
	last struct {
		method, path, query, body string
	}
	reply string
}

func newRecorder(reply string) *recorder {
	r := &recorder{reply: reply}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.last.method, r.last.path = req.Method, req.URL.Path
		r.last.query, r.last.body = req.URL.RawQuery, string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(r.reply))
	}))
	return r
}

func TestOperationsTargetBackendSpecificAPIs(t *testing.T) {
	const window = 5 * time.Minute
	ctx := context.Background()

	cases := []struct {
		db       string
		call     func(*Client) error
		wantPath string
	}{
		{"loki", func(c *Client) error { return c.Push(ctx, testEntries()) }, "/loki/api/v1/push"},
		{"elasticsearch", func(c *Client) error { return c.Push(ctx, testEntries()) }, "/" + Index + "/_bulk"},

		{"loki", func(c *Client) error { return c.LabelValues(ctx, window) }, "/loki/api/v1/label/service/values"},
		{"elasticsearch", func(c *Client) error { return c.LabelValues(ctx, window) }, "/" + Index + "/_search"},

		{"loki", func(c *Client) error { return c.QueryRange(ctx, "svc-000", 100, window) }, "/loki/api/v1/query_range"},
		{"elasticsearch", func(c *Client) error { return c.QueryRange(ctx, "svc-000", 100, window) }, "/" + Index + "/_search"},

		{"loki", func(c *Client) error { return c.FilterMatch(ctx, "svc-000", "timeout", 100, window) }, "/loki/api/v1/query_range"},
		{"elasticsearch", func(c *Client) error { return c.FilterMatch(ctx, "svc-000", "timeout", 100, window) }, "/" + Index + "/_search"},

		{"loki", func(c *Client) error { return c.CountOverTime(ctx, "svc-000", window) }, "/loki/api/v1/query"},
		{"elasticsearch", func(c *Client) error { return c.CountOverTime(ctx, "svc-000", window) }, "/" + Index + "/_count"},
	}

	for _, c := range cases {
		rec := newRecorder(`{"errors":false,"count":0}`)
		client := New(c.db, rec.srv.URL)
		if err := c.call(client); err != nil {
			rec.srv.Close()
			t.Fatalf("%s: call failed: %v", c.db, err)
		}
		if rec.last.path != c.wantPath {
			t.Errorf("%s: path = %q, want %q", c.db, rec.last.path, c.wantPath)
		}
		rec.srv.Close()
	}
}

func TestFilterMatchUsesLineFilterOnLokiAndMatchOnES(t *testing.T) {
	ctx := context.Background()

	rec := newRecorder(`{}`)
	defer rec.srv.Close()
	if err := New("loki", rec.srv.URL).FilterMatch(ctx, "svc-000", "timeout", 10, time.Minute); err != nil {
		t.Fatalf("loki: %v", err)
	}
	// The LogQL expression arrives URL-encoded; decode before asserting.
	q, err := url.ParseQuery(rec.last.query)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if want := `{service="svc-000"} |= "timeout"`; q.Get("query") != want {
		t.Errorf("logql = %q, want %q", q.Get("query"), want)
	}
	if q.Get("direction") != "backward" {
		t.Errorf("direction = %q, want backward", q.Get("direction"))
	}

	rec2 := newRecorder(`{}`)
	defer rec2.srv.Close()
	if err := New("elasticsearch", rec2.srv.URL).FilterMatch(ctx, "svc-000", "timeout", 10, time.Minute); err != nil {
		t.Fatalf("elasticsearch: %v", err)
	}
	for _, want := range []string{`"term"`, `"service": "svc-000"`, `"match"`, `"message": "timeout"`} {
		if !strings.Contains(rec2.last.body, want) {
			t.Errorf("ES query body missing %s: %s", want, rec2.last.body)
		}
	}
}

func TestPushSurfacesBulkItemErrors(t *testing.T) {
	// _bulk answers 200 even when documents fail, so the per-item status must
	// be inspected or dropped data would look like a successful ingest.
	rec := newRecorder(`{"errors":true,"items":[
	  {"create":{"status":201}},
	  {"create":{"status":400,"error":{"type":"mapper_parsing_exception"}}}
	]}`)
	defer rec.srv.Close()

	err := New("elasticsearch", rec.srv.URL).Push(context.Background(), testEntries())
	if err == nil {
		t.Fatal("expected an error when bulk items fail")
	}
	if !strings.Contains(err.Error(), "mapper_parsing_exception") {
		t.Errorf("error should name the failing item: %v", err)
	}
}

func TestEnsureIndexIsNoOpForLoki(t *testing.T) {
	rec := newRecorder(`{}`)
	defer rec.srv.Close()

	if err := New("loki", rec.srv.URL).EnsureIndex(context.Background()); err != nil {
		t.Fatalf("loki EnsureIndex: %v", err)
	}
	if rec.last.method != "" {
		t.Errorf("loki EnsureIndex issued %s %s, want no request",
			rec.last.method, rec.last.path)
	}

	if err := New("elasticsearch", rec.srv.URL).EnsureIndex(context.Background()); err != nil {
		t.Fatalf("es EnsureIndex: %v", err)
	}
	if rec.last.method != http.MethodPut || rec.last.path != "/"+Index {
		t.Errorf("es EnsureIndex = %s %s, want PUT /%s",
			rec.last.method, rec.last.path, Index)
	}
}

func TestFlushRefreshesOnlyElasticsearch(t *testing.T) {
	rec := newRecorder(`{}`)
	defer rec.srv.Close()

	if err := New("loki", rec.srv.URL).Flush(context.Background()); err != nil {
		t.Fatalf("loki Flush: %v", err)
	}
	if rec.last.method != "" {
		t.Errorf("loki Flush issued %s %s, want no request", rec.last.method, rec.last.path)
	}

	if err := New("elasticsearch", rec.srv.URL).Flush(context.Background()); err != nil {
		t.Fatalf("es Flush: %v", err)
	}
	if rec.last.path != "/"+Index+"/_refresh" {
		t.Errorf("es Flush path = %q, want /%s/_refresh", rec.last.path, Index)
	}
}

func TestCountIngestedParsesBackendShapes(t *testing.T) {
	ctx := context.Background()

	lokiRec := newRecorder(`{"data":{"result":[{"value":[1700000000,"4231"]}]}}`)
	defer lokiRec.srv.Close()
	got, err := New("loki", lokiRec.srv.URL).CountIngested(ctx, "svc-000", time.Minute)
	if err != nil {
		t.Fatalf("loki: %v", err)
	}
	if got != 4231 {
		t.Errorf("loki count = %d, want 4231", got)
	}

	// No matching streams yet must read as zero, not as an error.
	emptyRec := newRecorder(`{"data":{"result":[]}}`)
	defer emptyRec.srv.Close()
	got, err = New("loki", emptyRec.srv.URL).CountIngested(ctx, "svc-000", time.Minute)
	if err != nil || got != 0 {
		t.Errorf("empty loki result = (%d, %v), want (0, nil)", got, err)
	}

	esRec := newRecorder(`{"count":9876}`)
	defer esRec.srv.Close()
	got, err = New("elasticsearch", esRec.srv.URL).CountIngested(ctx, "svc-000", time.Minute)
	if err != nil {
		t.Fatalf("elasticsearch: %v", err)
	}
	if got != 9876 {
		t.Errorf("es count = %d, want 9876", got)
	}
}

func TestDoRejectsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("Ingestion rate limit exceeded"))
	}))
	defer srv.Close()

	err := New("loki", srv.URL).Push(context.Background(), testEntries())
	if err == nil {
		t.Fatal("expected an error on 429")
	}
	// Rate-limit rejections are the most likely real ingest failure, so the
	// status and body must both reach the caller.
	if !strings.Contains(err.Error(), "429") ||
		!strings.Contains(err.Error(), "Ingestion rate limit") {
		t.Errorf("error should carry status and body: %v", err)
	}
}

func TestPushEmptyBatchIsNoOp(t *testing.T) {
	rec := newRecorder(`{}`)
	defer rec.srv.Close()

	if err := New("elasticsearch", rec.srv.URL).Push(context.Background(), nil); err != nil {
		t.Fatalf("empty push: %v", err)
	}
	if rec.last.method != "" {
		t.Errorf("empty push issued %s %s, want no request", rec.last.method, rec.last.path)
	}
}

func TestLokiDuration(t *testing.T) {
	cases := []struct {
		secs int
		want string
	}{
		{300, "300s"},
		{1, "1s"},
		{0, "1s"}, // sub-second windows must still yield a valid LogQL range
	}
	for _, c := range cases {
		got := lokiDuration(time.Duration(c.secs) * time.Second)
		if got != c.want {
			t.Errorf("lokiDuration(%ds) = %q, want %q", c.secs, got, c.want)
		}
	}
}
