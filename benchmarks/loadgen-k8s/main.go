package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/tech-comparison-lab/loadgen-k8s/internal/bench"
	"github.com/tech-comparison-lab/loadgen-k8s/internal/client"
	"github.com/tech-comparison-lab/loadgen-k8s/internal/report"
)

func main() {
	var (
		kubeconfig  = flag.String("kubeconfig", client.DefaultKubeconfig(), "path to kubeconfig")
		kctx        = flag.String("context", "", "kubeconfig context (default: current)")
		namespace   = flag.String("namespace", "bench-k8s", "namespace for benchmark workloads")
		op          = flag.String("op", "all", "benchmarks: all|api-latency|deploy|scale|overhead")
		count       = flag.Int("count", 1000, "iterations for API latency test")
		rounds      = flag.Int("rounds", 3, "rounds for deploy/scale benchmarks")
		maxReplicas = flag.Int("replicas", 20, "max replicas for scale test")
		outFile     = flag.String("out", "", "output JSON file")
		noCleanup   = flag.Bool("no-cleanup", false, "skip namespace cleanup after run")
	)
	flag.Parse()

	if err := validateConfig(*namespace, *op, *count, *rounds, *maxReplicas); err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	cfg, err := client.BuildConfig(*kubeconfig, *kctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kubeconfig: %v\n", err)
		os.Exit(1)
	}
	cs, err := client.NewClientset(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clientset: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	clusterType := client.DetectClusterType(ctx, cs)
	fmt.Printf("cluster type : %s\n", clusterType)

	var results []report.Result
	runLatency := func() error {
		fmt.Println("→ api-latency...")
		r, err := bench.RunAPILatency(ctx, cs, *count)
		if err != nil {
			return fmt.Errorf("api-latency: %w", err)
		}
		results = append(results, r...)
		return nil
	}

	runOverhead := func() error {
		fmt.Println("→ overhead...")
		r, err := bench.RunOverhead(ctx, cs)
		if err != nil {
			return fmt.Errorf("overhead: %w", err)
		}
		results = append(results, r...)
		return nil
	}

	withNamespace := func(fn func() error) error {
		if err := ensureNamespace(ctx, cs, *namespace); err != nil {
			return fmt.Errorf("namespace: %w", err)
		}
		err := fn()
		if !*noCleanup {
			_ = cs.CoreV1().Namespaces().Delete(ctx, *namespace, metav1.DeleteOptions{})
		}
		return err
	}

	runDeployInNS := func() error {
		fmt.Printf("→ deploy (%d rounds)...\n", *rounds)
		r, err := bench.RunDeploy(ctx, cs, *namespace, *rounds)
		if err != nil {
			return fmt.Errorf("deploy: %w", err)
		}
		results = append(results, r...)
		return nil
	}

	runScaleInNS := func() error {
		fmt.Printf("→ scale (max=%d, %d rounds)...\n", *maxReplicas, *rounds)
		r, err := bench.RunScale(ctx, cs, *namespace, *rounds, int32(*maxReplicas))
		if err != nil {
			return fmt.Errorf("scale: %w", err)
		}
		results = append(results, r...)
		return nil
	}

	switch *op {
	case "all":
		// Share one namespace for deploy + scale to avoid termination race.
		if err := runLatency(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := runOverhead(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := ensureNamespace(ctx, cs, *namespace); err != nil {
			fmt.Fprintf(os.Stderr, "namespace: %v\n", err)
			os.Exit(1)
		}
		if err := runDeployInNS(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			if !*noCleanup {
				_ = cs.CoreV1().Namespaces().Delete(ctx, *namespace, metav1.DeleteOptions{})
			}
			os.Exit(1)
		}
		if err := runScaleInNS(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			if !*noCleanup {
				_ = cs.CoreV1().Namespaces().Delete(ctx, *namespace, metav1.DeleteOptions{})
			}
			os.Exit(1)
		}
		if !*noCleanup {
			_ = cs.CoreV1().Namespaces().Delete(ctx, *namespace, metav1.DeleteOptions{})
		}
	case "api-latency":
		if err := runLatency(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "overhead":
		if err := runOverhead(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "deploy":
		if err := withNamespace(runDeployInNS); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "scale":
		if err := withNamespace(runScaleInNS); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown op: %s (valid: all|api-latency|deploy|scale|overhead)\n", *op)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "no benchmark results produced")
		os.Exit(1)
	}

	rep := report.Report{
		ClusterType: clusterType,
		Timestamp:   time.Now().UTC(),
		Results:     results,
	}
	report.PrintTable(os.Stdout, rep)

	if *outFile != "" {
		if err := report.WriteJSON(*outFile, rep); err != nil {
			fmt.Fprintf(os.Stderr, "write json: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Results saved to %s\n", *outFile)
	}
}

func ensureNamespace(ctx context.Context, cs *kubernetes.Clientset, name string) error {
	_, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return err
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	_, err = cs.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return err
}

func validateConfig(namespace, op string, count, rounds, maxReplicas int) error {
	if namespace == "" {
		return fmt.Errorf("--namespace must not be empty")
	}
	validOps := map[string]bool{
		"all":         true,
		"api-latency": true,
		"deploy":      true,
		"scale":       true,
		"overhead":    true,
	}
	if !validOps[op] {
		return fmt.Errorf("unknown --op %q, want all|api-latency|deploy|scale|overhead", op)
	}
	if count < 1 {
		return fmt.Errorf("--count must be >= 1")
	}
	if rounds < 1 {
		return fmt.Errorf("--rounds must be >= 1")
	}
	if maxReplicas < 1 {
		return fmt.Errorf("--replicas must be >= 1")
	}
	return nil
}
