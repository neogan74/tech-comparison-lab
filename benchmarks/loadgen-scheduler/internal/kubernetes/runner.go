package kubernetes

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	workloadName  = "scheduler-bench"
	workloadKey   = "app"
	workloadVal   = "scheduler-bench"
	workloadImage = "registry.k8s.io/pause:3.10"
)

type Runner struct {
	client    kubernetes.Interface
	namespace string
	replicas  int32
}

func New(kubeconfig, kubeContext, namespace string) (*Runner, error) {
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: kubeContext}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	runner := &Runner{client: client, namespace: namespace}
	if err := runner.ensureNamespace(context.Background()); err != nil {
		return nil, err
	}
	return runner, nil
}

func (runner *Runner) Name() string { return "kubernetes" }

func (runner *Runner) ensureNamespace(ctx context.Context) error {
	_, err := runner.client.CoreV1().Namespaces().Get(ctx, runner.namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	_, err = runner.client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: runner.namespace},
	}, metav1.CreateOptions{})
	return err
}

func (runner *Runner) Cleanup(ctx context.Context) error {
	policy := metav1.DeletePropagationForeground
	err := runner.client.AppsV1().Deployments(runner.namespace).Delete(ctx, workloadName, metav1.DeleteOptions{PropagationPolicy: &policy})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	runner.replicas = 0
	return runner.waitPods(ctx, 0, "")
}

func (runner *Runner) Deploy(ctx context.Context, replicas int) (time.Duration, error) {
	start := time.Now()
	_, err := runner.client.AppsV1().Deployments(runner.namespace).Create(ctx, deployment(runner.namespace, int32(replicas)), metav1.CreateOptions{})
	if err != nil {
		return 0, err
	}
	runner.replicas = int32(replicas)
	if err := runner.waitReady(ctx, runner.replicas); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (runner *Runner) Scale(ctx context.Context, replicas int) (time.Duration, error) {
	value, err := runner.client.AppsV1().Deployments(runner.namespace).Get(ctx, workloadName, metav1.GetOptions{})
	if err != nil {
		return 0, err
	}
	value.Spec.Replicas = int32ptr(int32(replicas))
	start := time.Now()
	if _, err := runner.client.AppsV1().Deployments(runner.namespace).Update(ctx, value, metav1.UpdateOptions{}); err != nil {
		return 0, err
	}
	runner.replicas = int32(replicas)
	if err := runner.waitReady(ctx, runner.replicas); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (runner *Runner) Recover(ctx context.Context) (time.Duration, error) {
	pods, err := runner.client.CoreV1().Pods(runner.namespace).List(ctx, metav1.ListOptions{LabelSelector: workloadKey + "=" + workloadVal})
	if err != nil {
		return 0, err
	}
	if len(pods.Items) == 0 {
		return 0, fmt.Errorf("no workload pods available for recovery benchmark")
	}
	victim := pods.Items[0]
	zero := int64(0)
	start := time.Now()
	if err := runner.client.CoreV1().Pods(runner.namespace).Delete(ctx, victim.Name, metav1.DeleteOptions{GracePeriodSeconds: &zero}); err != nil {
		return 0, err
	}
	if err := runner.waitPods(ctx, runner.replicas, string(victim.UID)); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (runner *Runner) waitReady(ctx context.Context, replicas int32) error {
	for {
		value, err := runner.client.AppsV1().Deployments(runner.namespace).Get(ctx, workloadName, metav1.GetOptions{})
		if err == nil && value.Status.ObservedGeneration >= value.Generation && value.Status.ReadyReplicas == replicas && value.Status.UpdatedReplicas == replicas && value.Status.UnavailableReplicas == 0 {
			return nil
		}
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if err := wait(ctx); err != nil {
			return fmt.Errorf("wait for %d ready Kubernetes replicas: %w", replicas, err)
		}
	}
}

func (runner *Runner) waitPods(ctx context.Context, count int32, excludedUID string) error {
	for {
		pods, err := runner.client.CoreV1().Pods(runner.namespace).List(ctx, metav1.ListOptions{LabelSelector: workloadKey + "=" + workloadVal})
		if err != nil {
			return err
		}
		ready := int32(0)
		oldPresent := false
		for _, pod := range pods.Items {
			if string(pod.UID) == excludedUID {
				oldPresent = true
			}
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					ready++
				}
			}
		}
		if ready == count && !oldPresent {
			return nil
		}
		if err := wait(ctx); err != nil {
			return fmt.Errorf("wait for %d Kubernetes pods: %w", count, err)
		}
	}
}

func deployment(namespace string, replicas int32) *appsv1.Deployment {
	zero := int64(0)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: workloadName, Namespace: namespace},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32ptr(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{workloadKey: workloadVal}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{workloadKey: workloadVal}},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &zero,
					Containers: []corev1.Container{{
						Name: "bench", Image: workloadImage, ImagePullPolicy: corev1.PullIfNotPresent,
						Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("10Mi"),
						}},
					}},
				},
			},
		},
	}
}

func int32ptr(value int32) *int32 { return &value }

func wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(200 * time.Millisecond):
		return nil
	}
}
