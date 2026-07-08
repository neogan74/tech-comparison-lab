package k8s

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type Client struct {
	cs *kubernetes.Clientset
}

func NewClient(kubeconfig, context string) (*Client, error) {
	if kubeconfig == "" {
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
	}
	loadRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadRules.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if context != "" {
		overrides.CurrentContext = context
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{cs: cs}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cs.Discovery().ServerVersion()
	return err
}

// EnsureNamespace creates the namespace if it does not exist.
func (c *Client) EnsureNamespace(ctx context.Context, ns string) error {
	_, err := c.cs.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return err
	}
	_, err = c.cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	return err
}

// WaitForCM polls until the named ConfigMap exists in ns, or until timeout.
func (c *Client) WaitForCM(ctx context.Context, ns, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := c.cs.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			return nil
		}
		if !k8serrors.IsNotFound(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timeout waiting for ConfigMap %s/%s after %s", ns, name, timeout)
}

// DeleteCM removes a ConfigMap; ignores not-found.
func (c *Client) DeleteCM(ctx context.Context, ns, name string) error {
	err := c.cs.CoreV1().ConfigMaps(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if k8serrors.IsNotFound(err) {
		return nil
	}
	return err
}

// WaitForCMGone polls until the ConfigMap is absent, or until timeout.
func (c *Client) WaitForCMGone(ctx context.Context, ns, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := c.cs.CoreV1().ConfigMaps(ns).Get(ctx, name, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timeout waiting for ConfigMap %s/%s to disappear after %s", ns, name, timeout)
}
