package reconciler

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/alfredtm/kuberless/api/v1alpha1"
	"github.com/alfredtm/kuberless/operator/shared"
)

// ResourceQuotaReconciler manages ResourceQuotas for tenant namespaces.
type ResourceQuotaReconciler struct {
	Client client.Client
}

// Reconcile ensures the ResourceQuota for the tenant namespace exists and is up-to-date.
func (r *ResourceQuotaReconciler) Reconcile(ctx context.Context, tenant *platformv1alpha1.Tenant) error {
	logger := log.FromContext(ctx)
	nsName := tenant.GetNamespaceName()
	quotaName := tenant.Name + "-quota"

	maxCPU, maxMemory := r.getResourceLimits(tenant)

	desired := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaName,
			Namespace: nsName,
			Labels: map[string]string{
				shared.LabelManagedBy: shared.LabelManagedByValue,
				shared.LabelTenant:    tenant.Name,
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{
				corev1.ResourceRequestsCPU:    resource.MustParse(maxCPU),
				corev1.ResourceLimitsCPU:      resource.MustParse(maxCPU),
				corev1.ResourceRequestsMemory: resource.MustParse(maxMemory),
				corev1.ResourceLimitsMemory:   resource.MustParse(maxMemory),
			},
		},
	}

	existing := &corev1.ResourceQuota{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: quotaName, Namespace: nsName}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("getting resource quota: %w", err)
		}
		logger.Info("Creating ResourceQuota", "namespace", nsName, "name", quotaName)
		return r.Client.Create(ctx, desired)
	}

	existing.Spec.Hard = desired.Spec.Hard
	existing.Labels = desired.Labels
	logger.Info("Updating ResourceQuota", "namespace", nsName, "name", quotaName)
	return r.Client.Update(ctx, existing)
}

func (r *ResourceQuotaReconciler) getResourceLimits(tenant *platformv1alpha1.Tenant) (cpu, memory string) {
	plan := string(tenant.Spec.Plan)
	if plan == "" {
		plan = "free"
	}

	defaults := shared.PlanDefaults[plan]
	cpu = defaults.MaxCPU
	memory = defaults.MaxMemory

	if tenant.Spec.ResourceLimits != nil {
		if tenant.Spec.ResourceLimits.MaxCPU != "" {
			cpu = tenant.Spec.ResourceLimits.MaxCPU
		}
		if tenant.Spec.ResourceLimits.MaxMemory != "" {
			memory = tenant.Spec.ResourceLimits.MaxMemory
		}
	}

	return cpu, memory
}
