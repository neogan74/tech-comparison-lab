package bench

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/tech-comparison-lab/loadgen-k8s/internal/report"
)

const (
	// pause:3.10 is already cached on k8s nodes and runs as UID 65535 (non-root).
	benchImage     = "registry.k8s.io/pause:3.10"
	deployName     = "bench-workload"
	deployLabel    = "app"
	deployLabelVal = "bench-workload"
)

// RunDeploy measures time from Deployment creation to all pods Ready.
// It repeats rounds times and returns percentile stats over all rounds.
func RunDeploy(ctx context.Context, cs *kubernetes.Clientset, ns string, rounds int) ([]report.Result, error) {
	var durs []time.Duration

	for i := 0; i < rounds; i++ {
		fmt.Printf("  deploy: round %d/%d — cleaning...\n", i+1, rounds)
		_ = cs.AppsV1().Deployments(ns).Delete(ctx, deployName, metav1.DeleteOptions{})
		if err := waitPodsGone(ctx, cs, ns); err != nil {
			return nil, fmt.Errorf("deploy round %d cleanup: %w", i+1, err)
		}

		dep := makeDeployment(ns, 1)
		start := time.Now()
		if _, err := cs.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("deploy round %d create: %w", i+1, err)
		}
		elapsed, err := waitDeployReady(ctx, cs, ns, deployName, 1)
		if err != nil {
			return nil, fmt.Errorf("deploy round %d wait: %w", i+1, err)
		}
		_ = start
		durs = append(durs, elapsed)
		fmt.Printf("  deploy: round %d ready in %s\n", i+1, elapsed.Round(time.Millisecond))
	}

	_ = cs.AppsV1().Deployments(ns).Delete(ctx, deployName, metav1.DeleteOptions{})
	_ = waitPodsGone(ctx, cs, ns)

	return []report.Result{report.FromDurations("deploy:1-replica", durs, 0)}, nil
}

func makeDeployment(ns string, replicas int32) *appsv1.Deployment {
	var gracePeriod int64 = 0
	allowPrivEsc := false
	runAsNonRoot := true
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployName,
			Namespace: ns,
			Labels:    map[string]string{deployLabel: deployLabelVal},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{deployLabel: deployLabelVal},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{deployLabel: deployLabelVal},
				},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &gracePeriod,
					Containers: []corev1.Container{
						{
							Name:  "bench",
							Image: benchImage,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"),
									corev1.ResourceMemory: resource.MustParse("8Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivEsc,
								RunAsNonRoot:             &runAsNonRoot,
							},
						},
					},
				},
			},
		},
	}
}

func waitDeployReady(ctx context.Context, cs *kubernetes.Clientset, ns, name string, replicas int32) (time.Duration, error) {
	start := time.Now()
	deadline := start.Add(5 * time.Minute)
	for {
		dep, err := cs.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			return 0, err
		}
		if err == nil &&
			dep.Status.ReadyReplicas >= replicas &&
			dep.Status.UpdatedReplicas >= replicas &&
			dep.Status.UnavailableReplicas == 0 {
			return time.Since(start), nil
		}
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("timeout: %s/%s not ready after 5m", ns, name)
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func waitPodsGone(ctx context.Context, cs *kubernetes.Clientset, ns string) error {
	deadline := time.Now().Add(3 * time.Minute)
	selector := deployLabel + "=" + deployLabelVal
	for {
		pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: selector})
		if err != nil {
			return err
		}
		if len(pods.Items) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout: pods in %s still running after 3m", ns)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
