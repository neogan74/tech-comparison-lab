package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"

	"github.com/tech-comparison-lab/loadgen-http/internal/types"
)

func init() {
	// Same JSON codec as the server — replaces the default proto codec so that
	// both sides speak JSON over gRPC's HTTP/2 transport.
	encoding.RegisterCodec(jsonCodec{})
}

type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v any) error  { return json.Unmarshal(data, v) }
func (jsonCodec) Name() string                        { return "proto" }

// Bench drives gRPC benchmark operations over a single shared connection.
type Bench struct {
	conn *grpc.ClientConn
}

// New dials the gRPC server and returns a Bench.
func New(addr string) (*Bench, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpc.NewClient: %w", err)
	}
	return &Bench{conn: conn}, nil
}

// Close releases the connection.
func (b *Bench) Close() { b.conn.Close() }

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

func (b *Bench) doOp(ctx context.Context, op string) error {
	switch op {
	case "echo":
		return b.invoke(ctx, "Echo", &types.EchoReq{Msg: "hello"}, &types.EchoResp{})
	case "get-user":
		id := mrand.Intn(10000) + 1
		return b.invoke(ctx, "GetUser", &types.UserReq{ID: id}, &types.UserResp{})
	case "create-order":
		req := &types.OrderReq{UserID: mrand.Intn(10000) + 1, ProductID: mrand.Intn(100) + 1, Qty: mrand.Intn(5) + 1}
		return b.invoke(ctx, "CreateOrder", req, &types.OrderResp{})
	default:
		return fmt.Errorf("unknown op: %s", op)
	}
}

func (b *Bench) invoke(ctx context.Context, method string, req, resp any) error {
	return b.conn.Invoke(ctx, "/bench.BenchService/"+method, req, resp)
}
