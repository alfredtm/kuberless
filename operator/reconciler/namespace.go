package reconciler

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/alfredtm/kuberless/api/v1alpha1"
	"github.com/alfredtm/kuberless/operator/shared"
)

// NamespaceReconciler manages the namespace for a tenant.
type NamespaceReconciler struct {
	Client client.Client
}

// Reconcile ensures the namespace for the tenant exists with correct labels.
func (r *NamespaceReconciler) Reconcile(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	logger := log.FromContext(ctx)
	nsName := tenant.GetNamespaceName()

	ns := &corev1.Namespace{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: nsName}, ns)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("getting namespace %s: %w", nsName, err)
		}

		logger.Info("Creating namespace", "namespace", nsName)
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					shared.LabelManagedBy:       shared.LabelManagedByValue,
					shared.LabelTenant:          tenant.Name,
					"capsule.clastix.io/tenant": tenant.Name,
				},
			},
		}
		if err := r.Client.Create(ctx, ns); err != nil {
			return fmt.Errorf("creating namespace %s: %w", nsName, err)
		}
		return nil
	}

	// Ensure labels are up-to-date.
	updated := false
	if ns.Labels == nil {
		ns.Labels = make(map[string]string)
	}
	for k, v := range map[string]string{
		shared.LabelManagedBy:       shared.LabelManagedByValue,
		shared.LabelTenant:          tenant.Name,
		"capsule.clastix.io/tenant": tenant.Name,
	} {
		if ns.Labels[k] != v {
			ns.Labels[k] = v
			updated = true
		}
	}
	if updated {
		logger.Info("Updating namespace labels", "namespace", nsName)
		if err := r.Client.Update(ctx, ns); err != nil {
			return fmt.Errorf("updating namespace %s: %w", nsName, err)
		}
	}

	return nil
}

// Delete removes the namespace for the tenant.
func (r *NamespaceReconciler) Delete(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	logger := log.FromContext(ctx)
	nsName := tenant.GetNamespaceName()

	ns := &corev1.Namespace{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: nsName}, ns)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting namespace %s for deletion: %w", nsName, err)
	}

	logger.Info("Deleting namespace", "namespace", nsName)
	if err := r.Client.Delete(ctx, ns); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting namespace %s: %w", nsName, err)
	}
	return nil
}
