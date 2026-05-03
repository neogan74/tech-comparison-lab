package graphqlbench

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
)

// Bench drives GraphQL benchmark operations.
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

type gqlRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

func (b *Bench) post(ctx context.Context, req gqlRequest) error {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.base+"/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
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
	return b.post(ctx, gqlRequest{
		Query: `query { echo(msg: "hello") { msg } }`,
	})
}

func (b *Bench) getUser(ctx context.Context) error {
	id := mrand.Intn(10000) + 1
	return b.post(ctx, gqlRequest{
		Query:     `query GetUser($id: Int) { getUser(id: $id) { id name email country score } }`,
		Variables: map[string]interface{}{"id": id},
	})
}

func (b *Bench) createOrder(ctx context.Context) error {
	return b.post(ctx, gqlRequest{
		Query: `mutation CreateOrder($userId: Int, $productId: Int, $qty: Int) {
			createOrder(userId: $userId, productId: $productId, qty: $qty) { id status total }
		}`,
		Variables: map[string]interface{}{
			"userId":    mrand.Intn(10000) + 1,
			"productId": mrand.Intn(100) + 1,
			"qty":       mrand.Intn(5) + 1,
		},
	})
}
