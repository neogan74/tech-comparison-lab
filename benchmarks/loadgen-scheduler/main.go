package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/client-go/tools/clientcmd"

	kubernetesrunner "github.com/tech-comparison-lab/loadgen-scheduler/internal/kubernetes"
	nomadrunner "github.com/tech-comparison-lab/loadgen-scheduler/internal/nomad"
	"github.com/tech-comparison-lab/loadgen-scheduler/internal/report"
	"github.com/tech-comparison-lab/loadgen-scheduler/internal/scheduler"
	swarmrunner "github.com/tech-comparison-lab/loadgen-scheduler/internal/swarm"
)

type config struct {
	platform     string
	kubeconfig   string
	kubeContext  string
	namespace    string
	nomadAddress string
	dockerHost   string
	rounds       int
	replicas     int
	timeout      time.Duration
	outFile      string
	noCleanup    bool
}

func main() {
	value := parseFlags()
	if err := validateConfig(value); err != nil {
		fail("config: %v", err)
	}
	runner, err := newRunner(value)
	if err != nil {
		fail("connect to %s: %v", value.platform, err)
	}

	results, err := benchmark(context.Background(), runner, value)
	if err != nil {
		fail("benchmark %s: %v", runner.Name(), err)
	}
	valueReport := report.Report{Platform: runner.Name(), Timestamp: time.Now().UTC(), Results: results}
	report.PrintTable(os.Stdout, valueReport)
	if value.outFile != "" {
		if err := report.WriteJSON(value.outFile, valueReport); err != nil {
			fail("write results: %v", err)
		}
		fmt.Printf("Results saved to %s\n", value.outFile)
	}
}

func parseFlags() config {
	value := config{}
	flag.StringVar(&value.platform, "platform", "kubernetes", "scheduler platform: kubernetes|nomad|swarm")
	flag.StringVar(&value.kubeconfig, "kubeconfig", clientcmd.RecommendedHomeFile, "path to kubeconfig")
	flag.StringVar(&value.kubeContext, "context", "", "kubeconfig context")
	flag.StringVar(&value.namespace, "namespace", "bench-scheduler", "Kubernetes benchmark namespace")
	flag.StringVar(&value.nomadAddress, "nomad-address", "http://127.0.0.1:4646", "Nomad HTTP API address")
	flag.StringVar(&value.dockerHost, "docker-host", "unix:///var/run/docker.sock", "Docker Engine API host (unix:// or http://)")
	flag.IntVar(&value.rounds, "rounds", 3, "benchmark rounds")
	flag.IntVar(&value.replicas, "replicas", 10, "scale-up target")
	flag.DurationVar(&value.timeout, "timeout", 3*time.Minute, "timeout per operation")
	flag.StringVar(&value.outFile, "out", "", "output JSON file")
	flag.BoolVar(&value.noCleanup, "no-cleanup", false, "leave benchmark workload running")
	flag.Parse()
	return value
}

func validateConfig(value config) error {
	if value.platform != "kubernetes" && value.platform != "nomad" && value.platform != "swarm" {
		return fmt.Errorf("--platform must be kubernetes, nomad, or swarm")
	}
	if value.rounds < 1 {
		return fmt.Errorf("--rounds must be >= 1")
	}
	if value.replicas < 2 {
		return fmt.Errorf("--replicas must be >= 2")
	}
	if value.timeout <= 0 {
		return fmt.Errorf("--timeout must be positive")
	}
	if value.namespace == "" {
		return fmt.Errorf("--namespace must not be empty")
	}
	return nil
}

func newRunner(value config) (scheduler.Runner, error) {
	if value.platform == "nomad" {
		return nomadrunner.New(value.nomadAddress)
	}
	if value.platform == "swarm" {
		return swarmrunner.New(value.dockerHost)
	}
	return kubernetesrunner.New(value.kubeconfig, value.kubeContext, value.namespace)
}

func benchmark(ctx context.Context, runner scheduler.Runner, value config) ([]report.Result, error) {
	ops := []string{
		"deploy:1-replica",
		fmt.Sprintf("scale:1-to-%d", value.replicas),
		"recover:1-instance",
		fmt.Sprintf("scale:%d-to-1", value.replicas),
	}
	durations := make(map[string][]time.Duration, len(ops))

	cleanup := func() error {
		operationCtx, cancel := context.WithTimeout(ctx, value.timeout)
		defer cancel()
		return runner.Cleanup(operationCtx)
	}
	if err := cleanup(); err != nil {
		return nil, fmt.Errorf("initial cleanup: %w", err)
	}
	if !value.noCleanup {
		defer func() { _ = cleanup() }()
	}

	for round := 1; round <= value.rounds; round++ {
		fmt.Printf("[%s] round %d/%d\n", runner.Name(), round, value.rounds)
		if round > 1 {
			if err := cleanup(); err != nil {
				return nil, fmt.Errorf("round %d cleanup: %w", round, err)
			}
		}

		duration, err := timed(ctx, value.timeout, func(operationCtx context.Context) (time.Duration, error) {
			return runner.Deploy(operationCtx, 1)
		})
		if err != nil {
			return nil, fmt.Errorf("round %d deploy: %w", round, err)
		}
		durations[ops[0]] = append(durations[ops[0]], duration)

		duration, err = timed(ctx, value.timeout, func(operationCtx context.Context) (time.Duration, error) {
			return runner.Scale(operationCtx, value.replicas)
		})
		if err != nil {
			return nil, fmt.Errorf("round %d scale up: %w", round, err)
		}
		durations[ops[1]] = append(durations[ops[1]], duration)

		duration, err = timed(ctx, value.timeout, runner.Recover)
		if err != nil {
			return nil, fmt.Errorf("round %d recover: %w", round, err)
		}
		durations[ops[2]] = append(durations[ops[2]], duration)

		duration, err = timed(ctx, value.timeout, func(operationCtx context.Context) (time.Duration, error) {
			return runner.Scale(operationCtx, 1)
		})
		if err != nil {
			return nil, fmt.Errorf("round %d scale down: %w", round, err)
		}
		durations[ops[3]] = append(durations[ops[3]], duration)
	}

	results := make([]report.Result, 0, len(ops))
	for _, op := range ops {
		results = append(results, report.FromDurations(op, durations[op], 0))
	}
	return results, nil
}

func timed(ctx context.Context, timeout time.Duration, operation func(context.Context) (time.Duration, error)) (time.Duration, error) {
	operationCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return operation(operationCtx)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
