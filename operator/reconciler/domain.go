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

// DomainReconciler manages Knative DomainMapping resources for custom domains.
type DomainReconciler struct {
	Client client.Client
}

// Reconcile ensures DomainMappings for the app's custom domains exist.
func (r *DomainReconciler) Reconcile(ctx context.Context, app *platformv1alpha1.App) error {
	logger := log.FromContext(ctx)

	// Get existing DomainMappings for this app.
	existingList := &unstructured.UnstructuredList{}
	existingList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "serving.knative.dev",
		Version: "v1beta1",
		Kind:    "DomainMappingList",
	})

	err := r.Client.List(ctx, existingList,
		client.InNamespace(app.Namespace),
		client.MatchingLabels{shared.LabelApp: app.Name},
	)
	if err != nil {
		return fmt.Errorf("listing DomainMappings: %w", err)
	}

	// Build a set of desired hostnames.
	desiredHosts := make(map[string]bool)
	for _, d := range app.Spec.CustomDomains {
		desiredHosts[d.Hostname] = true
	}

	// Delete DomainMappings that are no longer desired.
	existingHosts := make(map[string]bool)
	for _, item := range existingList.Items {
		hostname := item.GetName()
		existingHosts[hostname] = true
		if !desiredHosts[hostname] {
			logger.Info("Deleting DomainMapping", "hostname", hostname)
			if err := r.Client.Delete(ctx, &item); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("deleting DomainMapping %s: %w", hostname, err)
			}
		}
	}

	// Create DomainMappings that don't exist yet.
	for _, d := range app.Spec.CustomDomains {
		if existingHosts[d.Hostname] {
			continue
		}
		logger.Info("Creating DomainMapping", "hostname", d.Hostname)
		dm := r.buildDomainMapping(app, d.Hostname)
		if err := r.Client.Create(ctx, dm); err != nil {
			return fmt.Errorf("creating DomainMapping %s: %w", d.Hostname, err)
		}
	}

	return nil
}

// Delete removes all DomainMappings for the app.
func (r *DomainReconciler) Delete(ctx context.Context, app *platformv1alpha1.App) error {
	logger := log.FromContext(ctx)

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "serving.knative.dev",
		Version: "v1beta1",
		Kind:    "DomainMappingList",
	})

	err := r.Client.List(ctx, list,
		client.InNamespace(app.Namespace),
		client.MatchingLabels{shared.LabelApp: app.Name},
	)
	if err != nil {
		return fmt.Errorf("listing DomainMappings for deletion: %w", err)
	}

	for _, item := range list.Items {
		logger.Info("Deleting DomainMapping", "hostname", item.GetName())
		if err := r.Client.Delete(ctx, &item); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("deleting DomainMapping %s: %w", item.GetName(), err)
		}
	}

	return nil
}

func (r *DomainReconciler) buildDomainMapping(app *platformv1alpha1.App, hostname string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "serving.knative.dev/v1beta1",
			"kind":       "DomainMapping",
			"metadata": map[string]interface{}{
				"name":      hostname,
				"namespace": app.Namespace,
				"labels": map[string]interface{}{
					shared.LabelManagedBy: shared.LabelManagedByValue,
					shared.LabelApp:       app.Name,
					shared.LabelTenant:    app.Spec.TenantRef,
				},
			},
			"spec": map[string]interface{}{
				"ref": map[string]interface{}{
					"name":       app.Name,
					"kind":       "Service",
					"apiVersion": "serving.knative.dev/v1",
				},
			},
		},
	}
}
