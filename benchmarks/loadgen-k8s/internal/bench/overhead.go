package bench

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/tech-comparison-lab/loadgen-k8s/internal/report"
)

// RunOverhead counts platform-level resources to estimate control plane overhead.
// Results are directly comparable between vanilla k8s (few system namespaces)
// and OpenShift (50+ openshift-* namespaces, 100+ system pods).
func RunOverhead(ctx context.Context, cs *kubernetes.Clientset) ([]report.Result, error) {
	// ── namespaces ─────────────────────────────────────────────────────────────
	nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	totalNS := len(nsList.Items)
	var systemNSNames []string
	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			systemNSNames = append(systemNSNames, ns.Name)
		}
	}

	// ── system pods ────────────────────────────────────────────────────────────
	systemPodCount := 0
	for _, nsName := range systemNSNames {
		pods, err := cs.CoreV1().Pods(nsName).List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
				systemPodCount++
			}
		}
	}

	// ── nodes ──────────────────────────────────────────────────────────────────
	nodeList, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	nodeCount := len(nodeList.Items)
	var totalCPUMillis int64
	var totalMemBytes int64
	for _, node := range nodeList.Items {
		if cpu := node.Status.Capacity.Cpu(); cpu != nil {
			totalCPUMillis += cpu.MilliValue()
		}
		if mem := node.Status.Capacity.Memory(); mem != nil {
			totalMemBytes += mem.Value()
		}
	}

	return []report.Result{
		report.Scalar("overhead:total-namespaces", float64(totalNS), "count"),
		report.Scalar("overhead:system-namespaces", float64(len(systemNSNames)), "count"),
		report.Scalar("overhead:system-pods", float64(systemPodCount), "count"),
		report.Scalar("overhead:nodes", float64(nodeCount), "count"),
		report.Scalar("overhead:total-cpu-cores", float64(totalCPUMillis)/1000, "cores"),
		report.Scalar("overhead:total-memory-gb", float64(totalMemBytes)/1e9, "GB"),
	}, nil
}

func isSystemNamespace(name string) bool {
	exact := map[string]bool{
		"default": true, "kube-system": true, "kube-public": true, "kube-node-lease": true,
	}
	if exact[name] {
		return true
	}
	for _, prefix := range []string{"kube-", "openshift-", "cattle-", "calico-", "cert-manager", "flux-system"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
