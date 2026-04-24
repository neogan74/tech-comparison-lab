package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	kafkabench "github.com/tech-comparison-lab/loadgen-msg/internal/kafka"
	natsbench "github.com/tech-comparison-lab/loadgen-msg/internal/nats"
	rabbitbench "github.com/tech-comparison-lab/loadgen-msg/internal/rabbit"
	"github.com/tech-comparison-lab/loadgen-msg/internal/report"
)

func main() {
	db := flag.String("db", "", "broker: kafka | rabbitmq | nats (required)")
	op := flag.String("op", "all", "operation: produce|consume|all")
	count := flag.Int("count", 1000000, "total messages")
	batchSize := flag.Int("batch", 1000, "messages per batch write")
	consumers := flag.Int("consumers", 3, "concurrent consumers")
	addr := flag.String("addr", "", "broker address (or KAFKA_ADDR / RABBIT_ADDR / NATS_ADDR env)")
	topic := flag.String("topic", "bench", "Kafka topic / RabbitMQ queue name / NATS subject")
	partitions := flag.Int("partitions", 3, "Kafka topic partitions")
	out := flag.String("out", "", "write JSON results to file (optional)")
	clean := flag.Bool("clean", false, "delete topic/queue before running")
	dryRun := flag.Bool("dry-run", false, "test connectivity only")
	flag.Parse()

	if *db == "" {
		fmt.Fprintln(os.Stderr, "error: --db is required (kafka | rabbitmq | nats)")
		flag.Usage()
		os.Exit(1)
	}
	if *db != "kafka" && *db != "rabbitmq" && *db != "nats" {
		fmt.Fprintf(os.Stderr, "error: unknown --db %q\n", *db)
		os.Exit(1)
	}
	if err := validateConfig(*db, *op, *count, *batchSize, *consumers, *partitions); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if *addr == "" {
		envKey := map[string]string{"kafka": "KAFKA_ADDR", "rabbitmq": "RABBIT_ADDR", "nats": "NATS_ADDR"}[*db]
		*addr = os.Getenv(envKey)
	}
	if *addr == "" {
		defaults := map[string]string{
			"kafka":    "localhost:9093",
			"rabbitmq": "amqp://bench:benchpass@localhost:5672/",
			"nats":     "nats://localhost:4222",
		}
		*addr = defaults[*db]
	}

	ctx := context.Background()
	var results []report.Result

	switch *db {
	case "kafka":
		results = runKafka(ctx, *addr, *op, *count, *batchSize, *consumers, *partitions, *topic, *clean, *dryRun)
	case "rabbitmq":
		results = runRabbit(ctx, *addr, *op, *count, *batchSize, *consumers, *topic, *clean, *dryRun)
	case "nats":
		results = runNats(ctx, *addr, *op, *count, *batchSize, *consumers, *topic, *clean, *dryRun)
	}

	if len(results) > 0 {
		report.PrintTable(results)
		if *out != "" {
			summary := report.Summary{
				RunID:     fmt.Sprintf("%s-%d", *db, time.Now().Unix()),
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Results:   results,
			}
			if err := report.WriteJSON(summary, *out); err != nil {
				log.Printf("warning: could not write JSON: %v", err)
			} else {
				fmt.Printf("\nResults saved to %s\n", *out)
			}
		}
	}
}

func validateConfig(db, op string, count, batchSize, consumers, partitions int) error {
	validOps := map[string]bool{
		"produce": true,
		"consume": true,
		"all":     true,
	}
	if !validOps[op] {
		return fmt.Errorf("unknown --op %q, want produce|consume|all", op)
	}
	if db != "kafka" && db != "rabbitmq" && db != "nats" {
		return fmt.Errorf("unknown --db %q", db)
	}
	if count < 1 {
		return fmt.Errorf("--count must be >= 1")
	}
	if batchSize < 1 {
		return fmt.Errorf("--batch must be >= 1")
	}
	if consumers < 1 {
		return fmt.Errorf("--consumers must be >= 1")
	}
	if partitions < 1 {
		return fmt.Errorf("--partitions must be >= 1")
	}
	return nil
}

func runKafka(ctx context.Context, addr, op string, count, batchSize, consumers, partitions int, topic string, clean, dryRun bool) []report.Result {
	brokers := strings.Split(addr, ",")
	bench := kafkabench.New(brokers, topic)

	// Connectivity check via topic list
	fmt.Printf("kafka: connecting to %s...\n", addr)

	if dryRun {
		fmt.Println("kafka: dry-run OK")
		return nil
	}

	if clean {
		_ = bench.DeleteTopic(ctx) // ignore error if not exists
		fmt.Println("kafka: deleted topic")
	}
	if err := bench.CreateTopic(ctx, partitions); err != nil {
		log.Fatalf("kafka CreateTopic: %v", err)
	}
	fmt.Printf("kafka: topic %q ready (%d partitions)\n", topic, partitions)

	var results []report.Result

	if op == "produce" || op == "all" {
		fmt.Printf("kafka: producing %d messages (batch=%d)...\n", count, batchSize)
		durs, total, err := bench.Produce(ctx, count, batchSize)
		if err != nil {
			log.Fatalf("kafka produce: %v", err)
		}
		results = append(results, report.Compute("kafka", "produce", count, batchSize, durs, total))
		fmt.Printf("kafka: produce done in %s\n", total.Round(time.Millisecond))
	}

	if op == "consume" || op == "all" {
		fmt.Printf("kafka: consuming %d messages (%d consumers)...\n", count, consumers)
		stats, total, err := bench.Consume(ctx, count, consumers)
		if err != nil {
			log.Fatalf("kafka consume: %v", err)
		}
		r := report.Compute("kafka", "consume", stats.Total, 0, nil, total)
		r.ThroughputOps = float64(stats.Total) / total.Seconds()
		r.TotalMs = total.Milliseconds()
		for i, n := range stats.PerConsumer {
			qps := 0.0
			if total > 0 {
				qps = float64(n) / total.Seconds()
			}
			r.ConsumerStats = append(r.ConsumerStats, report.ConsumerStat{ID: i, Msgs: n, QPS: qps})
		}
		results = append(results, r)
		fmt.Printf("kafka: consume done in %s (%d msgs total)\n", total.Round(time.Millisecond), stats.Total)
	}

	return results
}

func runRabbit(ctx context.Context, dsn, op string, count, batchSize, consumers int, queue string, clean, dryRun bool) []report.Result {
	bench, err := rabbitbench.New(dsn, queue)
	if err != nil {
		log.Fatalf("rabbitmq connect: %v", err)
	}
	defer bench.Close()
	fmt.Printf("rabbitmq: connected to %s\n", dsn)

	if dryRun {
		fmt.Println("rabbitmq: dry-run OK")
		return nil
	}

	if err := bench.Setup(); err != nil {
		log.Fatalf("rabbitmq setup: %v", err)
	}
	if clean {
		if err := bench.Purge(); err != nil {
			log.Fatalf("rabbitmq purge: %v", err)
		}
		fmt.Println("rabbitmq: queue purged")
	}

	var results []report.Result

	if op == "produce" || op == "all" {
		fmt.Printf("rabbitmq: producing %d messages (batch=%d)...\n", count, batchSize)
		durs, total, err := bench.Produce(ctx, count, batchSize)
		if err != nil {
			log.Fatalf("rabbitmq produce: %v", err)
		}
		results = append(results, report.Compute("rabbitmq", "produce", count, batchSize, durs, total))
		fmt.Printf("rabbitmq: produce done in %s\n", total.Round(time.Millisecond))
	}

	if op == "consume" || op == "all" {
		depth, _ := bench.QueueDepth()
		actual := depth
		if actual == 0 {
			actual = count
		}
		fmt.Printf("rabbitmq: consuming %d messages (%d consumers, queue depth=%d)...\n",
			actual, consumers, depth)
		stats, total, err := bench.Consume(ctx, actual, consumers)
		if err != nil {
			log.Fatalf("rabbitmq consume: %v", err)
		}
		r := report.Compute("rabbitmq", "consume", stats.Total, 0, nil, total)
		r.ThroughputOps = float64(stats.Total) / total.Seconds()
		r.TotalMs = total.Milliseconds()
		for i, n := range stats.PerConsumer {
			qps := 0.0
			if total > 0 {
				qps = float64(n) / total.Seconds()
			}
			r.ConsumerStats = append(r.ConsumerStats, report.ConsumerStat{ID: i, Msgs: n, QPS: qps})
		}
		results = append(results, r)
		fmt.Printf("rabbitmq: consume done in %s (%d msgs total)\n", total.Round(time.Millisecond), stats.Total)
	}

	return results
}

func runNats(ctx context.Context, addr, op string, count, batchSize, consumers int, subject string, clean, dryRun bool) []report.Result {
	stream := "bench-stream"
	bench, err := natsbench.New(addr, stream, subject)
	if err != nil {
		log.Fatalf("nats connect: %v", err)
	}
	defer bench.Close()
	fmt.Printf("nats: connected to %s\n", addr)

	if dryRun {
		fmt.Println("nats: dry-run OK")
		return nil
	}

	if err := bench.CreateStream(ctx); err != nil {
		log.Fatalf("nats CreateStream: %v", err)
	}
	if clean {
		if err := bench.Purge(ctx); err != nil {
			log.Fatalf("nats purge: %v", err)
		}
		fmt.Println("nats: stream purged")
	}

	var results []report.Result

	if op == "produce" || op == "all" {
		fmt.Printf("nats: producing %d messages (batch=%d)...\n", count, batchSize)
		durs, total, err := bench.Produce(ctx, count, batchSize)
		if err != nil {
			log.Fatalf("nats produce: %v", err)
		}
		results = append(results, report.Compute("nats", "produce", count, batchSize, durs, total))
		fmt.Printf("nats: produce done in %s\n", total.Round(time.Millisecond))
	}

	if op == "consume" || op == "all" {
		fmt.Printf("nats: consuming %d messages (%d consumers)...\n", count, consumers)
		stats, total, err := bench.Consume(ctx, count, consumers)
		if err != nil {
			log.Fatalf("nats consume: %v", err)
		}
		r := report.Compute("nats", "consume", stats.Total, 0, nil, total)
		r.ThroughputOps = float64(stats.Total) / total.Seconds()
		r.TotalMs = total.Milliseconds()
		for i, n := range stats.PerConsumer {
			qps := 0.0
			if total > 0 {
				qps = float64(n) / total.Seconds()
			}
			r.ConsumerStats = append(r.ConsumerStats, report.ConsumerStat{ID: i, Msgs: n, QPS: qps})
		}
		results = append(results, r)
		fmt.Printf("nats: consume done in %s (%d msgs total)\n", total.Round(time.Millisecond), stats.Total)
	}

	return results
}
