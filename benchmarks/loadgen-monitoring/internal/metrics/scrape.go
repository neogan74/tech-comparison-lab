// Package metrics scrapes a target's own Prometheus text-exposition
// /metrics endpoint, used to read self-reported gauges (e.g. on-disk
// storage size) that aren't queryable via PromQL on a single instance.
package metrics

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// SumMetric fetches baseURL+"/metrics" and sums the values of every sample
// line whose metric name matches name exactly (ignoring label sets), e.g.
// summing a gauge reported per-label-set into a single total.
func SumMetric(ctx context.Context, baseURL, name string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/metrics", nil)
	if err != nil {
		return 0, fmt.Errorf("new request: %w", err)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, fmt.Errorf("scrape failed: status=%d", resp.StatusCode)
	}

	var total float64
	found := false
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		metric := line
		if idx := strings.IndexAny(line, " {"); idx >= 0 {
			metric = line[:idx]
		}
		if metric != name {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		total += v
		found = true
	}
	if err := sc.Err(); err != nil {
		return 0, fmt.Errorf("scan response: %w", err)
	}
	if !found {
		return 0, fmt.Errorf("metric %q not found", name)
	}
	return total, nil
}
