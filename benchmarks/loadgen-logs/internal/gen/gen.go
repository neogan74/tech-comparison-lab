// Package gen produces synthetic application log lines in a backend-neutral
// model. Each entry carries a service and level (Loki stream labels /
// Elasticsearch keyword fields) plus a unique message body, so no two entries
// collide as duplicates in either store.
package gen

import (
	"fmt"
	mrand "math/rand"

	"github.com/tech-comparison-lab/loadgen-logs/internal/logs"
)

// FilterToken is the word the filter-match benchmark searches for. It appears
// in exactly every TokenEveryN-th generated line, so both backends scan the
// same needle-in-haystack ratio.
const FilterToken = "timeout"

// TokenEveryN controls FilterToken density: 1 in 20 lines, i.e. 5%.
const TokenEveryN = 20

var levels = []string{"info", "info", "warn", "error", "debug"}

var templates = []string{
	"handled request method=GET path=/api/v2/resource status=200 duration_ms=%d",
	"handled request method=POST path=/api/v2/resource status=201 duration_ms=%d",
	"db query executed table=orders rows=%d",
	"cache lookup key=session hit=true elapsed_ms=%d",
	"published event topic=orders.created partition=%d",
}

// Services returns n deterministic service names, e.g. "svc-000".
func Services(n int) []string {
	if n < 1 {
		n = 1
	}
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("svc-%03d", i)
	}
	return names
}

// Entry builds log entry i. services assigns the emitting service
// (round-robin), nowNs anchors entries to a recent wall-clock time and
// windowNs spreads them backwards over the query window so they fall inside
// both backends' range queries.
func Entry(i int, services []string, nowNs, windowNs int64, rnd *mrand.Rand) logs.Entry {
	msg := fmt.Sprintf("seq=%d "+templates[i%len(templates)], i, 1+rnd.Intn(500))
	if i%TokenEveryN == 0 {
		msg += " error=upstream " + FilterToken
	}
	return logs.Entry{
		// Offset by 1s so no entry ever lands in the future relative to the
		// backend's clock, which both stores reject.
		TimestampNs: nowNs - 1_000_000_000 - rnd.Int63n(windowNs),
		Service:     services[i%len(services)],
		Level:       levels[i%len(levels)],
		Message:     msg,
	}
}
