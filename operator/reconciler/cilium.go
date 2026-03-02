package reconciler

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/alfredtm/kuberless/api/v1alpha1"
	"github.com/alfredtm/kuberless/operator/shared"
)

// CiliumReconciler manages CiliumNetworkPolicy resources for tenant isolation.
type CiliumReconciler struct {
	Client client.Client
}

// Reconcile ensures the CiliumNetworkPolicy for the tenant namespace exists.
func (r *CiliumReconciler) Reconcile(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	logger := log.FromContext(ctx)
	nsName := tenant.GetNamespaceName()

	if !tenant.IsNetworkIsolationEnabled() {
		logger.Info("Network isolation disabled, skipping CiliumNetworkPolicy", "tenant", tenant.Name)
		return nil
	}

	policyName := tenant.Name + "-isolation"

	desired := r.buildPolicy(tenant, policyName, nsName)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: "v2",
		Kind:    "CiliumNetworkPolicy",
	})

	err := r.Client.Get(ctx, client.ObjectKey{Name: policyName, Namespace: nsName}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("getting CiliumNetworkPolicy: %w", err)
		}
		logger.Info("Creating CiliumNetworkPolicy", "namespace", nsName, "name", policyName)
		return r.Client.Create(ctx, desired)
	}

	// Update the spec.
	desired.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("Updating CiliumNetworkPolicy", "namespace", nsName, "name", policyName)
	return r.Client.Update(ctx, desired)
}

// Delete removes the CiliumNetworkPolicy for the tenant namespace.
func (r *CiliumReconciler) Delete(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	logger := log.FromContext(ctx)
	nsName := tenant.GetNamespaceName()
	policyName := tenant.Name + "-isolation"

	policy := &unstructured.Unstructured{}
	policy.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cilium.io",
		Version: "v2",
		Kind:    "CiliumNetworkPolicy",
	})

	err := r.Client.Get(ctx, client.ObjectKey{Name: policyName, Namespace: nsName}, policy)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting CiliumNetworkPolicy for deletion: %w", err)
	}

	logger.Info("Deleting CiliumNetworkPolicy", "namespace", nsName, "name", policyName)
	return r.Client.Delete(ctx, policy)
}

func (r *CiliumReconciler) buildPolicy(tenant *platformv1alpha1.Tenant, name, namespace string) *unstructured.Unstructured {
	policy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cilium.io/v2",
			"kind":       "CiliumNetworkPolicy",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					shared.LabelManagedBy: shared.LabelManagedByValue,
					shared.LabelTenant:    tenant.Name,
				},
			},
			"spec": map[string]interface{}{
				"endpointSelector": map[string]interface{}{},
				"ingress": []interface{}{
					// Allow ingress from same namespace.
					map[string]interface{}{
						"fromEndpoints": []interface{}{
							map[string]interface{}{},
						},
					},
					// Allow ingress from knative-serving namespace.
					map[string]interface{}{
						"fromEndpoints": []interface{}{
							map[string]interface{}{
								"matchLabels": map[string]interface{}{
									"k8s:io.kubernetes.pod.namespace": shared.KnativeServingNamespace,
								},
							},
						},
					},
					// Allow ingress from kuberless-system namespace.
					map[string]interface{}{
						"fromEndpoints": []interface{}{
							map[string]interface{}{
								"matchLabels": map[string]interface{}{
									"k8s:io.kubernetes.pod.namespace": shared.SystemNamespace,
								},
							},
						},
					},
				},
				"egress": []interface{}{
					// Allow egress to same namespace.
					map[string]interface{}{
						"toEndpoints": []interface{}{
							map[string]interface{}{},
						},
					},
					// Allow DNS resolution.
					map[string]interface{}{
						"toEndpoints": []interface{}{
							map[string]interface{}{
								"matchLabels": map[string]interface{}{
									"k8s:io.kubernetes.pod.namespace": "kube-system",
									"k8s-app":                         "kube-dns",
								},
							},
						},
						"toPorts": []interface{}{
							map[string]interface{}{
								"ports": []interface{}{
									map[string]interface{}{
										"port":     "53",
										"protocol": "UDP",
									},
									map[string]interface{}{
										"port":     "53",
										"protocol": "TCP",
									},
								},
							},
						},
					},
					// Allow egress to internet (exclude RFC1918).
					map[string]interface{}{
						"toCIDRSet": []interface{}{
							map[string]interface{}{
								"cidr": "0.0.0.0/0",
								"except": []interface{}{
									"10.0.0.0/8",
									"172.16.0.0/12",
									"192.168.0.0/16",
								},
							},
						},
					},
				},
			},
		},
	}

	return policy
}
