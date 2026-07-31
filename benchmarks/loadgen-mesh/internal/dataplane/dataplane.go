// Package dataplane measures HTTP request latency and throughput against a
// mesh-fronted service reached over a local address (the experiment runner
// port-forwards the in-cluster Service). The server-side sidecar proxy is on
// the request path, so the numbers capture inbound data-plane overhead.
package dataplane

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/tech-comparison-lab/loadgen-mesh/internal/report"
)

// Run sends count GET requests to addr using workers concurrent goroutines and
// returns a latency Result plus a throughput scalar, both tagged with label
// (e.g. "meshed" or "baseline").
func Run(ctx context.Context, addr, label string, count, workers int) ([]report.Result, error) {
	if err := probe(ctx, addr); err != nil {
		return nil, fmt.Errorf("probe %s: %w", addr, err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	jobs := make(chan struct{}, workers*2)

	var mu sync.Mutex
	durs := make([]time.Duration, 0, count)
	errs := 0
	var wg sync.WaitGroup

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				t0 := time.Now()
				err := do(ctx, client, addr)
				d := time.Since(t0)
				mu.Lock()
				if err != nil {
					errs++
				} else {
					durs = append(durs, d)
				}
				mu.Unlock()
			}
		}()
	}

	start := time.Now()
	for i := 0; i < count; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	wg.Wait()
	total := time.Since(start)

	latency := report.FromDurations(fmt.Sprintf("data-plane:%s:latency", label), durs, errs)
	var rps float64
	if total > 0 {
		rps = float64(len(durs)) / total.Seconds()
	}
	throughput := report.Scalar(fmt.Sprintf("data-plane:%s:throughput", label), rps, "req/s")

	return []report.Result{latency, throughput}, nil
}

func do(ctx context.Context, client *http.Client, addr string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// probe retries a single request for up to 30s so a freshly opened
// port-forward has time to connect before the measured load begins.
func probe(ctx context.Context, addr string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := do(ctx, client, addr); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return lastErr
}
