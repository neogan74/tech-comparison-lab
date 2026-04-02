package bench

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/tech-comparison-lab/loadgen-k8s/internal/report"
)

// RunAPILatency benchmarks list-namespaces, list-pods, and list-nodes N times each.
func RunAPILatency(ctx context.Context, cs *kubernetes.Clientset, count int) ([]report.Result, error) {
	ops := []struct {
		name string
		fn   func() error
	}{
		{
			"api:list-namespaces",
			func() error {
				_, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
				return err
			},
		},
		{
			"api:list-pods-all",
			func() error {
				_, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{Limit: 100})
				return err
			},
		},
		{
			"api:list-nodes",
			func() error {
				_, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
				return err
			},
		},
	}

	var results []report.Result
	for _, op := range ops {
		fmt.Printf("  %-35s × %d...\n", op.name, count)
		durs := make([]time.Duration, 0, count)
		errors := 0
		for i := 0; i < count; i++ {
			start := time.Now()
			if err := op.fn(); err != nil {
				errors++
			} else {
				durs = append(durs, time.Since(start))
			}
		}
		fmt.Printf("  %-35s done (errors=%d)\n", op.name, errors)
		results = append(results, report.FromDurations(op.name, durs, errors))
	}
	return results, nil
}
