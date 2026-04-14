package nats

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const payload = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// Bench drives NATS JetStream benchmark operations.
type Bench struct {
	nc     *nats.Conn
	js     jetstream.JetStream
	stream string
	subj   string
}

// New connects to NATS and returns a Bench with JetStream context.
func New(url, stream, subject string) (*Bench, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats.Connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream.New: %w", err)
	}

	return &Bench{nc: nc, js: js, stream: stream, subj: subject}, nil
}

// Close releases connection.
func (b *Bench) Close() { b.nc.Close() }

func makeMsg(id int64) []byte {
	return []byte(fmt.Sprintf(`{"id":%d,"ts":%d,"p":"%s"}`, id, time.Now().UnixNano(), payload))
}

// CreateStream creates a JetStream stream with the subject.
// Idempotent: ignores "stream already exists" errors.
func (b *Bench) CreateStream(ctx context.Context) error {
	_, err := b.js.CreateStream(ctx, jetstream.StreamConfig{
		Name:      b.stream,
		Subjects:  []string{b.subj},
		Storage:   jetstream.MemoryStorage,
		Replicas:  1,
		Retention: jetstream.LimitsPolicy,
		MaxAge:    24 * time.Hour,
	})
	if err != nil && err != jetstream.ErrStreamNameAlreadyInUse {
		return fmt.Errorf("CreateStream: %w", err)
	}
	return nil
}

// DeleteStream removes the stream. Ignores "unknown stream" errors.
func (b *Bench) DeleteStream(ctx context.Context) error {
	err := b.js.DeleteStream(ctx, b.stream)
	if err != nil && err != jetstream.ErrStreamNotFound {
		return fmt.Errorf("DeleteStream: %w", err)
	}
	return nil
}

// Purge removes all messages from the stream.
func (b *Bench) Purge(ctx context.Context) error {
	stream, err := b.js.Stream(ctx, b.stream)
	if err != nil {
		return fmt.Errorf("Stream: %w", err)
	}
	if err := stream.Purge(ctx); err != nil {
		return fmt.Errorf("Purge: %w", err)
	}
	return nil
}

// Produce publishes count messages in batches of batchSize.
// Returns per-batch durations and total elapsed time.
func (b *Bench) Produce(ctx context.Context, count, batchSize int) ([]time.Duration, time.Duration, error) {
	durs := make([]time.Duration, 0, count/batchSize+1)
	start := time.Now()
	var msgID int64
	remaining := count

	for remaining > 0 {
		size := batchSize
		if remaining < size {
			size = remaining
		}
		t := time.Now()
		for i := 0; i < size; i++ {
			_, err := b.js.Publish(ctx, b.subj, makeMsg(msgID))
			if err != nil {
				return durs, time.Since(start), fmt.Errorf("publish: %w", err)
			}
			msgID++
		}
		durs = append(durs, time.Since(t))
		remaining -= size
	}
	return durs, time.Since(start), nil
}

// ConsumeStats holds per-consumer and total consumption results.
type ConsumeStats struct {
	PerConsumer []int
	Total       int
}

// Consume reads count messages total across consumers goroutines.
// Each consumer gets its own durable subscription for maximum throughput.
func (b *Bench) Consume(ctx context.Context, count, consumers int) (ConsumeStats, time.Duration, error) {
	var totalConsumed atomic.Int64
	perConsumer := make([]atomic.Int64, consumers)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < consumers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			consName := fmt.Sprintf("consumer-%d-%d", id, time.Now().UnixNano())
			_, err := b.js.CreateConsumer(ctx, b.stream, jetstream.ConsumerConfig{
				Name:          consName,
				AckPolicy:     jetstream.AckExplicitPolicy,
				FilterSubject: b.subj,
			})
			if err != nil {
				return
			}
			defer b.js.DeleteConsumer(ctx, b.stream, consName)

			sub, err := b.nc.Subscribe(b.subj, func(msg *nats.Msg) {
				msg.Ack()
				perConsumer[id].Add(1)
				if totalConsumed.Add(1) >= int64(count) {
					cancel()
				}
			})
			if err != nil {
				return
			}
			defer sub.Unsubscribe()

			<-ctx.Done()
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	stats := ConsumeStats{PerConsumer: make([]int, consumers)}
	for i := range perConsumer {
		stats.PerConsumer[i] = int(perConsumer[i].Load())
		stats.Total += stats.PerConsumer[i]
	}
	return stats, elapsed, nil
}
