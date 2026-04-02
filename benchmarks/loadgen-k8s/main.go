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

	runLatency := func() {
		fmt.Println("→ api-latency...")
		r, err := bench.RunAPILatency(ctx, cs, *count)
		if err != nil {
			fmt.Fprintf(os.Stderr, "api-latency: %v\n", err)
			return
		}
		results = append(results, r...)
	}

	runOverhead := func() {
		fmt.Println("→ overhead...")
		r, err := bench.RunOverhead(ctx, cs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "overhead: %v\n", err)
			return
		}
		results = append(results, r...)
	}

	withNamespace := func(fn func()) {
		if err := ensureNamespace(ctx, cs, *namespace); err != nil {
			fmt.Fprintf(os.Stderr, "namespace: %v\n", err)
			return
		}
		fn()
		if !*noCleanup {
			_ = cs.CoreV1().Namespaces().Delete(ctx, *namespace, metav1.DeleteOptions{})
		}
	}

	runDeployInNS := func() {
		fmt.Printf("→ deploy (%d rounds)...\n", *rounds)
		r, err := bench.RunDeploy(ctx, cs, *namespace, *rounds)
		if err != nil {
			fmt.Fprintf(os.Stderr, "deploy: %v\n", err)
			return
		}
		results = append(results, r...)
	}

	runScaleInNS := func() {
		fmt.Printf("→ scale (max=%d, %d rounds)...\n", *maxReplicas, *rounds)
		r, err := bench.RunScale(ctx, cs, *namespace, *rounds, int32(*maxReplicas))
		if err != nil {
			fmt.Fprintf(os.Stderr, "scale: %v\n", err)
			return
		}
		results = append(results, r...)
	}

	switch *op {
	case "all":
		// Share one namespace for deploy + scale to avoid termination race.
		runLatency()
		runOverhead()
		if err := ensureNamespace(ctx, cs, *namespace); err != nil {
			fmt.Fprintf(os.Stderr, "namespace: %v\n", err)
			os.Exit(1)
		}
		runDeployInNS()
		runScaleInNS()
		if !*noCleanup {
			_ = cs.CoreV1().Namespaces().Delete(ctx, *namespace, metav1.DeleteOptions{})
		}
	case "api-latency":
		runLatency()
	case "overhead":
		runOverhead()
	case "deploy":
		withNamespace(runDeployInNS)
	case "scale":
		withNamespace(runScaleInNS)
	default:
		fmt.Fprintf(os.Stderr, "unknown op: %s (valid: all|api-latency|deploy|scale|overhead)\n", *op)
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
