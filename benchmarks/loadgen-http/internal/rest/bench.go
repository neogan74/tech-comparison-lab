package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tech-comparison-lab/loadgen-http/internal/types"
)

// Bench drives REST benchmark operations.
type Bench struct {
	base   string // e.g. "http://localhost:8080"
	client *http.Client
}

// New creates a Bench with a tuned HTTP transport.
func New(base string, workers int) *Bench {
	transport := &http.Transport{
		MaxIdleConns:        workers * 2,
		MaxIdleConnsPerHost: workers * 2,
		IdleConnTimeout:     90 * time.Second,
	}
	return &Bench{
		base:   base,
		client: &http.Client{Transport: transport, Timeout: 10 * time.Second},
	}
}

// RunOp runs count requests for operation op across workers goroutines.
// Returns per-request durations, total elapsed time, and error count.
func (b *Bench) RunOp(ctx context.Context, op string, count, workers int) ([]time.Duration, time.Duration, int, error) {
	durs := make([]time.Duration, count)
	var errCount atomic.Int64

	work := make(chan int, count)
	for i := range count {
		work <- i
	}
	close(work)

	var wg sync.WaitGroup
	start := time.Now()

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				t := time.Now()
				if err := b.doOp(ctx, op); err != nil {
					errCount.Add(1)
				}
				durs[i] = time.Since(t)
			}
		}()
	}
	wg.Wait()
	return durs, time.Since(start), int(errCount.Load()), nil
}

func (b *Bench) doOp(ctx context.Context, op string) error {
	switch op {
	case "echo":
		return b.echo(ctx)
	case "get-user":
		return b.getUser(ctx)
	case "create-order":
		return b.createOrder(ctx)
	default:
		return fmt.Errorf("unknown op: %s", op)
	}
}

func (b *Bench) echo(ctx context.Context) error {
	resp, err := b.client.Get(b.base + "/echo?msg=hello")
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (b *Bench) getUser(ctx context.Context) error {
	id := mrand.Intn(10000) + 1
	resp, err := b.client.Get(fmt.Sprintf("%s/users/%d", b.base, id))
	if err != nil {
		return err
	}
	var u types.UserResp
	json.NewDecoder(resp.Body).Decode(&u)
	resp.Body.Close()
	return nil
}

func (b *Bench) createOrder(ctx context.Context) error {
	req := types.OrderReq{UserID: mrand.Intn(10000) + 1, ProductID: mrand.Intn(100) + 1, Qty: mrand.Intn(5) + 1}
	body, _ := json.Marshal(req)
	resp, err := b.client.Post(b.base+"/orders", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
