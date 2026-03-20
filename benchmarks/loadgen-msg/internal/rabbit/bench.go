package rabbit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const payload = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// Bench drives RabbitMQ benchmark operations.
type Bench struct {
	conn  *amqp.Connection
	queue string
	dsn   string
}

// New connects to RabbitMQ and returns a Bench.
func New(dsn, queue string) (*Bench, error) {
	conn, err := amqp.Dial(dsn)
	if err != nil {
		return nil, fmt.Errorf("amqp.Dial: %w", err)
	}
	return &Bench{conn: conn, queue: queue, dsn: dsn}, nil
}

// Close releases the connection.
func (b *Bench) Close() { b.conn.Close() }

// Setup declares a durable queue.
func (b *Bench) Setup() error {
	ch, err := b.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	_, err = ch.QueueDeclare(b.queue, true, false, false, false, nil)
	return err
}

// Purge removes all messages from the queue.
func (b *Bench) Purge() error {
	ch, err := b.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	_, err = ch.QueuePurge(b.queue, false)
	return err
}

// QueueDepth returns the current number of ready messages in the queue.
func (b *Bench) QueueDepth() (int, error) {
	ch, err := b.conn.Channel()
	if err != nil {
		return 0, err
	}
	defer ch.Close()
	q, err := ch.QueueInspect(b.queue)
	if err != nil {
		return 0, err
	}
	return q.Messages, nil
}

func makeMsg(id int64) []byte {
	return []byte(fmt.Sprintf(`{"id":%d,"ts":%d,"p":"%s"}`, id, time.Now().UnixNano(), payload))
}

// Produce publishes count messages in logical batches of batchSize.
// Uses publisher confirms for reliability parity with Kafka RequireOne.
func (b *Bench) Produce(ctx context.Context, count, batchSize int) ([]time.Duration, time.Duration, error) {
	ch, err := b.conn.Channel()
	if err != nil {
		return nil, 0, err
	}
	defer ch.Close()

	if err := ch.Confirm(false); err != nil {
		return nil, 0, fmt.Errorf("confirm mode: %w", err)
	}
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, batchSize))

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
			if err := ch.PublishWithContext(ctx, "", b.queue, false, false,
				amqp.Publishing{
					ContentType:  "application/octet-stream",
					Body:         makeMsg(msgID),
					DeliveryMode: amqp.Transient,
				}); err != nil {
				return durs, time.Since(start), fmt.Errorf("publish: %w", err)
			}
			msgID++
		}
		// Wait for broker to confirm all messages in this batch.
		for i := 0; i < size; i++ {
			select {
			case conf, ok := <-confirms:
				if !ok {
					return durs, time.Since(start), fmt.Errorf("confirms channel closed")
				}
				if !conf.Ack {
					return durs, time.Since(start), fmt.Errorf("message nack'd by broker")
				}
			case <-ctx.Done():
				return durs, time.Since(start), ctx.Err()
			}
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
// Each consumer gets its own channel with auto-ack for maximum throughput.
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
			ch, err := b.conn.Channel()
			if err != nil {
				return
			}
			defer ch.Close()
			// Prefetch 500 messages at a time per consumer.
			ch.Qos(500, 0, false)

			tag := fmt.Sprintf("consumer-%d", id)
			deliveries, err := ch.Consume(b.queue, tag, true, false, false, false, nil)
			if err != nil {
				return
			}
			for {
				select {
				case _, ok := <-deliveries:
					if !ok {
						return
					}
					perConsumer[id].Add(1)
					if totalConsumed.Add(1) >= int64(count) {
						cancel()
						return
					}
				case <-ctx.Done():
					return
				}
			}
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
