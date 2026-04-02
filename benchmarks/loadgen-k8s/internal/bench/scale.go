package bench

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/tech-comparison-lab/loadgen-k8s/internal/report"
)

type scaleStep struct{ from, to int32 }

// RunScale measures time to scale a Deployment: 1→5→maxReplicas→1.
// It repeats rounds times and returns percentile stats per step.
func RunScale(ctx context.Context, cs *kubernetes.Clientset, ns string, rounds int, maxReplicas int32) ([]report.Result, error) {
	steps := buildSteps(maxReplicas)

	dursByOp := make(map[string][]time.Duration, len(steps))
	for _, s := range steps {
		dursByOp[s.name()] = nil
	}

	for i := 0; i < rounds; i++ {
		fmt.Printf("  scale: round %d/%d — setup...\n", i+1, rounds)

		_ = cs.AppsV1().Deployments(ns).Delete(ctx, deployName, metav1.DeleteOptions{})
		_ = waitPodsGone(ctx, cs, ns)

		dep := makeDeployment(ns, 1)
		if _, err := cs.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("scale round %d create: %w", i+1, err)
		}
		if _, err := waitDeployReady(ctx, cs, ns, deployName, 1); err != nil {
			return nil, fmt.Errorf("scale round %d initial ready: %w", i+1, err)
		}

		for _, s := range steps {
			elapsed, err := doScale(ctx, cs, ns, s.to)
			if err != nil {
				return nil, fmt.Errorf("scale round %d %s: %w", i+1, s.name(), err)
			}
			dursByOp[s.name()] = append(dursByOp[s.name()], elapsed)
			fmt.Printf("  scale: %s ready in %s\n", s.name(), elapsed.Round(time.Millisecond))
		}
	}

	_ = cs.AppsV1().Deployments(ns).Delete(ctx, deployName, metav1.DeleteOptions{})
	_ = waitPodsGone(ctx, cs, ns)

	var results []report.Result
	for _, s := range steps {
		results = append(results, report.FromDurations(s.name(), dursByOp[s.name()], 0))
	}
	return results, nil
}

func buildSteps(maxReplicas int32) []scaleStep {
	if maxReplicas <= 5 {
		return []scaleStep{{1, maxReplicas}, {maxReplicas, 1}}
	}
	return []scaleStep{{1, 5}, {5, maxReplicas}, {maxReplicas, 1}}
}

func (s scaleStep) name() string {
	return fmt.Sprintf("scale:%d-to-%d", s.from, s.to)
}

func doScale(ctx context.Context, cs *kubernetes.Clientset, ns string, replicas int32) (time.Duration, error) {
	dep, err := cs.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		return 0, err
	}
	dep.Spec.Replicas = &replicas
	if _, err := cs.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		return 0, err
	}
	return waitDeployReady(ctx, cs, ns, deployName, replicas)
}
