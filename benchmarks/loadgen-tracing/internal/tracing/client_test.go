package tracing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodeSpansZipkinShape(t *testing.T) {
	spans := []Span{{
		TraceID:       "0000000000000000000000000000000a",
		ID:            "00000000000003e8",
		Name:          "GET /x",
		Timestamp:     1700000000000000,
		Duration:      1234,
		Kind:          "SERVER",
		LocalEndpoint: Endpoint{ServiceName: "svc-000"},
		Tags:          map[string]string{"http.status_code": "200"},
	}}
	body, err := EncodeSpans(spans)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Must be a JSON array and round-trip back to the same span.
	if !strings.HasPrefix(strings.TrimSpace(string(body)), "[") {
		t.Fatalf("expected JSON array, got %s", body)
	}
	var back []Span
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(back) != 1 || back[0].LocalEndpoint.ServiceName != "svc-000" {
		t.Fatalf("round-trip mismatch: %+v", back)
	}

	// parentId must be omitted when empty (root span).
	if strings.Contains(string(body), "parentId") {
		t.Errorf("root span should not emit parentId: %s", body)
	}
}

func TestQueryURLsDifferByBackend(t *testing.T) {
	j := New("jaeger", "http://ingest:9411", "http://query:16686")
	z := New("zipkin", "http://zip:9411", "")

	cases := []struct {
		got, want string
	}{
		{j.servicesURL(), "http://query:16686/api/services"},
		{z.servicesURL(), "http://zip:9411/api/v2/services"},
		{j.tracesURL("svc-000", 20), "http://query:16686/api/traces?service=svc-000&limit=20"},
		{z.tracesURL("svc-000", 20), "http://zip:9411/api/v2/traces?serviceName=svc-000&limit=20"},
		{j.traceURL("abc"), "http://query:16686/api/traces/abc"},
		{z.traceURL("abc"), "http://zip:9411/api/v2/trace/abc"},
		{j.operationsURL("svc-000"), "http://query:16686/api/services/svc-000/operations"},
		{z.operationsURL("svc-000"), "http://zip:9411/api/v2/spans?serviceName=svc-000"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("url = %q, want %q", c.got, c.want)
		}
	}

	// Zipkin ingest and query share one base URL when queryAddr is empty.
	if z.queryAddr != z.ingestAddr {
		t.Errorf("zipkin query addr = %q, want %q", z.queryAddr, z.ingestAddr)
	}
}

func TestFirstTraceID(t *testing.T) {
	jaeger := json.RawMessage(`{"data":[{"traceID":"deadbeef"}]}`)
	if got := firstTraceID("jaeger", jaeger); got != "deadbeef" {
		t.Errorf("jaeger firstTraceID = %q, want deadbeef", got)
	}
	zipkin := json.RawMessage(`[[{"traceId":"cafef00d","id":"1"}]]`)
	if got := firstTraceID("zipkin", zipkin); got != "cafef00d" {
		t.Errorf("zipkin firstTraceID = %q, want cafef00d", got)
	}
	if got := firstTraceID("zipkin", json.RawMessage(`[]`)); got != "" {
		t.Errorf("empty response should yield \"\", got %q", got)
	}
}
