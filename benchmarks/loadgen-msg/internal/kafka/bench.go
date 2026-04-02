package kafka

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
)

// payload is fixed ~160-byte string to make each message ~200 bytes total.
const payload = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

// Bench drives Kafka benchmark operations.
type Bench struct {
	brokers []string
	topic   string
}

// New returns a Bench targeting the given brokers and topic.
func New(brokers []string, topic string) *Bench {
	return &Bench{brokers: brokers, topic: topic}
}

// CreateTopic creates the topic with the given number of partitions.
// Idempotent: ignores "topic already exists" errors.
func (b *Bench) CreateTopic(ctx context.Context, partitions int) error {
	conn, err := kafka.DialContext(ctx, "tcp", b.brokers[0])
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	// In single-node KRaft mode the broker is also the controller.
	if err := conn.CreateTopics(kafka.TopicConfig{
		Topic:             b.topic,
		NumPartitions:     partitions,
		ReplicationFactor: 1,
	}); err != nil {
		// Ignore "topic already exists" — treat as idempotent.
		if err.Error() != "topic already exists" {
			return fmt.Errorf("CreateTopics: %w", err)
		}
	}
	return nil
}

// DeleteTopic removes the topic. Ignores "unknown topic" errors.
func (b *Bench) DeleteTopic(ctx context.Context) error {
	conn, err := kafka.DialContext(ctx, "tcp", b.brokers[0])
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	return conn.DeleteTopics(b.topic)
}

func makeMsg(id int64) []byte {
	return []byte(fmt.Sprintf(`{"id":%d,"ts":%d,"p":"%s"}`, id, time.Now().UnixNano(), payload))
}

// Produce sends count messages in batches of batchSize.
// Returns per-batch durations and total elapsed time.
func (b *Bench) Produce(ctx context.Context, count, batchSize int) ([]time.Duration, time.Duration, error) {
	w := &kafka.Writer{
		Addr:         kafka.TCP(b.brokers...),
		Topic:        b.topic,
		Balancer:     &kafka.RoundRobin{},
		BatchSize:    batchSize,
		BatchTimeout: 5 * time.Millisecond,
		RequiredAcks: kafka.RequireOne,
	}
	defer w.Close()

	durs := make([]time.Duration, 0, count/batchSize+1)
	start := time.Now()
	var msgID int64
	remaining := count

	for remaining > 0 {
		size := batchSize
		if remaining < size {
			size = remaining
		}
		msgs := make([]kafka.Message, size)
		for i := range msgs {
			msgs[i] = kafka.Message{Value: makeMsg(msgID)}
			msgID++
		}
		t := time.Now()
		if err := w.WriteMessages(ctx, msgs...); err != nil {
			return durs, time.Since(start), fmt.Errorf("WriteMessages: %w", err)
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
// Uses a unique consumer group each call to always start from offset 0.
func (b *Bench) Consume(ctx context.Context, count, consumers int) (ConsumeStats, time.Duration, error) {
	var totalConsumed atomic.Int64
	perConsumer := make([]atomic.Int64, consumers)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	groupID := fmt.Sprintf("bench-%d", time.Now().UnixNano())
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < consumers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			r := kafka.NewReader(kafka.ReaderConfig{
				Brokers:     b.brokers,
				Topic:       b.topic,
				GroupID:     groupID,
				MinBytes:    1,
				MaxBytes:    10e6,
				StartOffset: kafka.FirstOffset,
			})
			defer r.Close()
			for {
				if _, err := r.ReadMessage(ctx); err != nil {
					return
				}
				perConsumer[id].Add(1)
				if totalConsumed.Add(1) >= int64(count) {
					cancel()
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
