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

// CapsuleReconciler manages Capsule Tenant CRs for namespace management.
type CapsuleReconciler struct {
	Client client.Client
}

// Reconcile ensures the Capsule Tenant CR exists for the platform tenant.
func (r *CapsuleReconciler) Reconcile(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	logger := log.FromContext(ctx)
	capsuleName := tenant.Name

	desired := r.buildCapsuleTenant(tenant, capsuleName)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "capsule.clastix.io",
		Version: "v1beta2",
		Kind:    "Tenant",
	})

	err := r.Client.Get(ctx, client.ObjectKey{Name: capsuleName}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("getting Capsule Tenant: %w", err)
		}
		logger.Info("Creating Capsule Tenant", "name", capsuleName)
		return r.Client.Create(ctx, desired)
	}

	// Update the spec.
	desired.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("Updating Capsule Tenant", "name", capsuleName)
	return r.Client.Update(ctx, desired)
}

// Delete removes the Capsule Tenant CR.
func (r *CapsuleReconciler) Delete(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	logger := log.FromContext(ctx)
	capsuleName := tenant.Name

	ct := &unstructured.Unstructured{}
	ct.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "capsule.clastix.io",
		Version: "v1beta2",
		Kind:    "Tenant",
	})

	err := r.Client.Get(ctx, client.ObjectKey{Name: capsuleName}, ct)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting Capsule Tenant for deletion: %w", err)
	}

	logger.Info("Deleting Capsule Tenant", "name", capsuleName)
	return r.Client.Delete(ctx, ct)
}

func (r *CapsuleReconciler) buildCapsuleTenant(tenant *platformv1alpha1.Tenant, name string) *unstructured.Unstructured {
	ct := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "capsule.clastix.io/v1beta2",
			"kind":       "Tenant",
			"metadata": map[string]interface{}{
				"name": name,
				"labels": map[string]interface{}{
					shared.LabelManagedBy: shared.LabelManagedByValue,
					shared.LabelTenant:    tenant.Name,
				},
			},
			"spec": map[string]interface{}{
				"owners": []interface{}{
					map[string]interface{}{
						"kind": "User",
						"name": tenant.Spec.OwnerEmail,
					},
				},
				"namespaceOptions": map[string]interface{}{
					"quota": int64(tenant.Spec.MaxApps + 1), // +1 for the tenant namespace itself
				},
			},
		},
	}

	return ct
}
