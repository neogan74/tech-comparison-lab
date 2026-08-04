package client

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Clientset aliases kubernetes.Clientset so bench packages don't need to import client-go directly.
type Clientset = kubernetes.Clientset

// DefaultKubeconfig returns the default kubeconfig path.
func DefaultKubeconfig() string {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return kc
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".kube", "config")
	}
	return ""
}

// BuildConfig builds a *rest.Config from a kubeconfig file and optional context name.
func BuildConfig(kubeconfig, kctx string) (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		rules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if kctx != "" {
		overrides.CurrentContext = kctx
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

// NewClientset creates a new Kubernetes clientset from the given config.
// Rate limiting is disabled (QPS=0) so the API latency benchmark measures
// actual server latency, not client-side throttle delays.
func NewClientset(cfg *rest.Config) (*kubernetes.Clientset, error) {
	cfg.QPS = 0   // unlimited
	cfg.Burst = 0 // unlimited
	return kubernetes.NewForConfig(cfg)
}

// DetectClusterType returns "openshift" if openshift-* namespaces exist, otherwise "kubernetes".
func DetectClusterType(ctx context.Context, cs *kubernetes.Clientset) string {
	nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "kubernetes"
	}
	for _, ns := range nsList.Items {
		if strings.HasPrefix(ns.Name, "openshift-") {
			return "openshift"
		}
	}
	return "kubernetes"
}
