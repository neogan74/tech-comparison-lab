package pulsar

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pulsarclient "github.com/apache/pulsar-client-go/pulsar"
)

// payload is the same fixed payload used by the Kafka benchmark.
const payload = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// Bench drives Pulsar benchmark operations.
type Bench struct {
	client pulsarclient.Client
	topic  string
}

// ConsumeStats holds per-consumer and total consumption results.
type ConsumeStats struct {
	PerConsumer []int
	Total       int
}

// New creates a Pulsar benchmark client. Short topic names use the public/default namespace.
func New(url, topic string) (*Bench, error) {
	client, err := pulsarclient.NewClient(pulsarclient.ClientOptions{
		URL:               url,
		OperationTimeout:  30 * time.Second,
		ConnectionTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("NewClient: %w", err)
	}
	return &Bench{client: client, topic: normalizeTopic(topic)}, nil
}

// Close releases the Pulsar client and its connections.
func (b *Bench) Close() {
	b.client.Close()
}

func normalizeTopic(topic string) string {
	if strings.Contains(topic, "://") {
		return topic
	}
	return "persistent://public/default/" + topic
}

func makeMsg(id int64) []byte {
	return []byte(fmt.Sprintf(`{"id":%d,"ts":%d,"p":"%s"}`, id, time.Now().UnixNano(), payload))
}

// Produce sends count messages and waits for a broker acknowledgement after each batch.
func (b *Bench) Produce(ctx context.Context, count, batchSize int) ([]time.Duration, time.Duration, error) {
	producer, err := b.client.CreateProducer(pulsarclient.ProducerOptions{
		Topic:                   b.topic,
		DisableBatching:         false,
		BatchingMaxMessages:     uint(batchSize),
		BatchingMaxPublishDelay: 5 * time.Millisecond,
		SendTimeout:             30 * time.Second,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("CreateProducer: %w", err)
	}
	defer producer.Close()

	durations := make([]time.Duration, 0, count/batchSize+1)
	start := time.Now()
	var msgID int64

	for remaining := count; remaining > 0; {
		size := batchSize
		if remaining < size {
			size = remaining
		}

		batchStart := time.Now()
		var wg sync.WaitGroup
		var firstErr error
		var errOnce sync.Once
		wg.Add(size)

		for i := 0; i < size; i++ {
			id := msgID
			msgID++
			producer.SendAsync(ctx, &pulsarclient.ProducerMessage{Payload: makeMsg(id)}, func(_ pulsarclient.MessageID, _ *pulsarclient.ProducerMessage, sendErr error) {
				defer wg.Done()
				if sendErr != nil {
					errOnce.Do(func() { firstErr = sendErr })
				}
			})
		}

		wg.Wait()
		if firstErr != nil {
			return durations, time.Since(start), fmt.Errorf("SendAsync: %w", firstErr)
		}
		durations = append(durations, time.Since(batchStart))
		remaining -= size
	}

	return durations, time.Since(start), nil
}

// Consume reads count messages through a shared subscription.
func (b *Bench) Consume(ctx context.Context, count, consumers int) (ConsumeStats, time.Duration, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	subscription := fmt.Sprintf("bench-%d", time.Now().UnixNano())
	consumerClients := make([]pulsarclient.Consumer, 0, consumers)
	for i := 0; i < consumers; i++ {
		consumer, err := b.client.Subscribe(pulsarclient.ConsumerOptions{
			Topic:                       b.topic,
			SubscriptionName:            subscription,
			Type:                        pulsarclient.Shared,
			SubscriptionInitialPosition: pulsarclient.SubscriptionPositionEarliest,
			// Keep prefetch small so the first subscriber cannot buffer the entire
			// backlog before the remaining shared consumers connect.
			ReceiverQueueSize: 10,
		})
		if err != nil {
			for _, opened := range consumerClients {
				opened.Close()
			}
			return ConsumeStats{}, 0, fmt.Errorf("Subscribe consumer %d: %w", i, err)
		}
		consumerClients = append(consumerClients, consumer)
	}
	defer func() {
		for _, consumer := range consumerClients {
			consumer.Close()
		}
	}()

	var totalConsumed atomic.Int64
	perConsumer := make([]atomic.Int64, consumers)
	errCh := make(chan error, consumers)
	var wg sync.WaitGroup
	start := time.Now()

	for i, consumer := range consumerClients {
		wg.Add(1)
		go func(id int, c pulsarclient.Consumer) {
			defer wg.Done()
			for {
				msg, err := c.Receive(ctx)
				if err != nil {
					if ctx.Err() == nil {
						errCh <- fmt.Errorf("consumer %d receive: %w", id, err)
					}
					return
				}
				c.Ack(msg)
				perConsumer[id].Add(1)
				if totalConsumed.Add(1) >= int64(count) {
					cancel()
					return
				}
			}
		}(i, consumer)
	}

	wg.Wait()
	elapsed := time.Since(start)
	close(errCh)
	for err := range errCh {
		if err != nil {
			return ConsumeStats{}, elapsed, err
		}
	}

	stats := ConsumeStats{PerConsumer: make([]int, consumers)}
	for i := range perConsumer {
		stats.PerConsumer[i] = int(perConsumer[i].Load())
		stats.Total += stats.PerConsumer[i]
	}
	return stats, elapsed, nil
}
