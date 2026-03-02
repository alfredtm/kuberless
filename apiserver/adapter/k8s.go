package adapter

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	platformv1alpha1 "github.com/alfredtm/kuberless/api/v1alpha1"
)

// loadRESTConfig returns a *rest.Config using the standard kubeconfig
// resolution order: KUBECONFIG env var, ~/.kube/config, in-cluster.
func loadRESTConfig() (*rest.Config, error) {
	// Try in-cluster first, fall back to kubeconfig file.
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

// schemeWithCRDs returns a runtime.Scheme that includes both the core
// Kubernetes types and the platform CRD types.
func schemeWithCRDs() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(platformv1alpha1.AddToScheme(s))
	return s
}

// NewK8sClientset creates a kubernetes.Interface clientset using the standard
// kubeconfig resolution order. This is needed for pod log streaming, which
// the controller-runtime client does not support.
func NewK8sClientset() (kubernetes.Interface, error) {
	cfg, err := loadRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("adapter: load kubeconfig for clientset: %w", err)
	}
	return kubernetes.NewForConfig(cfg)
}
