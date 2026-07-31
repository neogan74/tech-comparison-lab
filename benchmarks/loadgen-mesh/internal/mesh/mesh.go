// Package mesh drives the control-plane / injection benchmarks against a
// service mesh (Istio or Linkerd) installed in the target cluster, using
// client-go only (no metrics-server, no in-cluster load tooling).
package mesh

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"github.com/tech-comparison-lab/loadgen-mesh/internal/report"
)

const (
	// echoImage serves HTTP/1.1 200s on :80 and is small + widely cached.
	echoImage   = "nginx:1.27-alpine"
	echoName    = "echo"
	echoLabel   = "app"
	echoLabelV  = "echo"
	echoSvcPort = 80
)

// Spec describes how a mesh is installed and how a namespace opts into sidecar
// injection.
type Spec struct {
	Name string // "istio" | "linkerd"
	// ControlNamespace holds the control-plane pods counted by the footprint op.
	ControlNamespace string
	// InjectKey/InjectVal is the metadata applied to a namespace so the mesh's
	// admission webhook injects sidecars into its pods. IsAnnotation selects
	// whether it goes on annotations (Linkerd) or labels (Istio).
	InjectKey    string
	InjectVal    string
	IsAnnotation bool
	// AppContainer is the name of the workload container, so footprint can tell
	// injected sidecar containers apart from the app.
	AppContainer string
}

// Specs maps a mesh name to its Spec.
var Specs = map[string]Spec{
	"istio": {
		Name:             "istio",
		ControlNamespace: "istio-system",
		InjectKey:        "istio-injection",
		InjectVal:        "enabled",
		IsAnnotation:     false,
		AppContainer:     echoName,
	},
	"linkerd": {
		Name:             "linkerd",
		ControlNamespace: "linkerd",
		InjectKey:        "linkerd.io/inject",
		InjectVal:        "enabled",
		IsAnnotation:     true,
		AppContainer:     echoName,
	},
}

// EnsureNamespace creates ns (idempotently) with the mesh's injection metadata
// applied, so pods created in it receive a sidecar.
func EnsureNamespace(ctx context.Context, cs *kubernetes.Clientset, s Spec, ns string, injected bool) error {
	meta := metav1.ObjectMeta{Name: ns}
	if injected {
		if s.IsAnnotation {
			meta.Annotations = map[string]string{s.InjectKey: s.InjectVal}
		} else {
			meta.Labels = map[string]string{s.InjectKey: s.InjectVal}
		}
	}

	existing, err := cs.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		_, err = cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: meta}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	// Namespace exists — make sure the injection metadata is present.
	existing.Labels = mergeInto(existing.Labels, meta.Labels)
	existing.Annotations = mergeInto(existing.Annotations, meta.Annotations)
	_, err = cs.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func mergeInto(dst, src map[string]string) map[string]string {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = map[string]string{}
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// RunInject deploys the echo workload into an injected namespace and measures
// the time from Deployment creation to all replicas Ready — capturing the
// admission-webhook injection plus sidecar-proxy startup cost. It repeats
// rounds times and returns percentile stats.
func RunInject(ctx context.Context, cs *kubernetes.Clientset, s Spec, ns string, replicas, rounds int) ([]report.Result, error) {
	if err := EnsureNamespace(ctx, cs, s, ns, true); err != nil {
		return nil, fmt.Errorf("ensure namespace: %w", err)
	}

	var durs []time.Duration
	for i := 0; i < rounds; i++ {
		fmt.Printf("  inject: round %d/%d — cleaning...\n", i+1, rounds)
		_ = cs.AppsV1().Deployments(ns).Delete(ctx, echoName, metav1.DeleteOptions{})
		if err := waitPodsGone(ctx, cs, ns); err != nil {
			return nil, fmt.Errorf("inject round %d cleanup: %w", i+1, err)
		}

		dep := makeEchoDeployment(ns, int32(replicas))
		if _, err := cs.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
			return nil, fmt.Errorf("inject round %d create: %w", i+1, err)
		}
		elapsed, err := waitDeployReady(ctx, cs, ns, echoName, int32(replicas))
		if err != nil {
			return nil, fmt.Errorf("inject round %d wait: %w", i+1, err)
		}
		durs = append(durs, elapsed)
		fmt.Printf("  inject: round %d ready in %s\n", i+1, elapsed.Round(time.Millisecond))
	}

	return []report.Result{report.FromDurations(fmt.Sprintf("inject:ready-%dr", replicas), durs, 0)}, nil
}

// RunFootprint reports the mesh control-plane pod count and summed resource
// requests, plus the per-pod sidecar resource overhead read from an injected
// echo pod. Requires an injected echo Deployment to already exist in ns.
func RunFootprint(ctx context.Context, cs *kubernetes.Clientset, s Spec, ns string) ([]report.Result, error) {
	var out []report.Result

	cpPods, err := cs.CoreV1().Pods(s.ControlNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list control-plane pods: %w", err)
	}
	var cpCPU, cpMem int64
	for _, p := range cpPods.Items {
		c, m := sumRequests(p.Spec.Containers)
		cpCPU += c
		cpMem += m
	}
	out = append(out,
		report.Scalar("footprint:control-plane-pods", float64(len(cpPods.Items)), "pods"),
		report.Scalar("footprint:control-plane-cpu-req", float64(cpCPU), "millicores"),
		report.Scalar("footprint:control-plane-mem-req", float64(cpMem)/(1024*1024), "MiB"),
	)

	// Sidecar overhead: inspect one injected echo pod and sum every container
	// that isn't the app container (the sidecar, and any init proxy sidecars
	// reported as regular containers).
	pods, err := cs.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: echoLabel + "=" + echoLabelV})
	if err != nil {
		return nil, fmt.Errorf("list echo pods: %w", err)
	}
	if len(pods.Items) > 0 {
		var sidecars []corev1.Container
		for _, c := range pods.Items[0].Spec.Containers {
			if c.Name != s.AppContainer {
				sidecars = append(sidecars, c)
			}
		}
		scCPU, scMem := sumRequests(sidecars)
		out = append(out,
			report.Scalar("footprint:sidecar-containers", float64(len(sidecars)), "containers"),
			report.Scalar("footprint:sidecar-cpu-req", float64(scCPU), "millicores"),
			report.Scalar("footprint:sidecar-mem-req", float64(scMem)/(1024*1024), "MiB"),
		)
	}

	return out, nil
}

// sumRequests returns the total CPU (millicores) and memory (bytes) requests
// across the given containers.
func sumRequests(cs []corev1.Container) (cpuMillis, memBytes int64) {
	for _, c := range cs {
		if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			cpuMillis += q.MilliValue()
		}
		if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			memBytes += q.Value()
		}
	}
	return cpuMillis, memBytes
}

// EnsureEcho deploys the echo Deployment + Service into ns (idempotently) and
// waits until it is Ready. Used to stand up the data-plane target.
func EnsureEcho(ctx context.Context, cs *kubernetes.Clientset, ns string, replicas int) error {
	dep := makeEchoDeployment(ns, int32(replicas))
	if _, err := cs.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create echo deployment: %w", err)
	}
	svc := makeEchoService(ns)
	if _, err := cs.CoreV1().Services(ns).Create(ctx, svc, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create echo service: %w", err)
	}
	if _, err := waitDeployReady(ctx, cs, ns, echoName, int32(replicas)); err != nil {
		return fmt.Errorf("wait echo ready: %w", err)
	}
	return nil
}

func makeEchoDeployment(ns string, replicas int32) *appsv1.Deployment {
	var grace int64 = 0
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      echoName,
			Namespace: ns,
			Labels:    map[string]string{echoLabel: echoLabelV},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{echoLabel: echoLabelV}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{echoLabel: echoLabelV}},
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: &grace,
					Containers: []corev1.Container{{
						Name:  echoName,
						Image: echoImage,
						Ports: []corev1.ContainerPort{{ContainerPort: echoSvcPort}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("10m"),
								corev1.ResourceMemory: resource.MustParse("16Mi"),
							},
						},
					}},
				},
			},
		},
	}
}

func makeEchoService(ns string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: echoName, Namespace: ns},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{echoLabel: echoLabelV},
			Ports:    []corev1.ServicePort{{Port: echoSvcPort, TargetPort: intstr.FromInt(echoSvcPort)}},
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
	selector := echoLabel + "=" + echoLabelV
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
