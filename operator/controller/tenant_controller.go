/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/alfredtm/kuberless/api/v1alpha1"
	"github.com/alfredtm/kuberless/operator/reconciler"
	"github.com/alfredtm/kuberless/operator/shared"
)

// TenantReconciler reconciles a Tenant object
type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	NamespaceReconciler     *reconciler.NamespaceReconciler
	CapsuleReconciler       *reconciler.CapsuleReconciler
	CiliumReconciler        *reconciler.CiliumReconciler
	ResourceQuotaReconciler *reconciler.ResourceQuotaReconciler
}

// +kubebuilder:rbac:groups=kuberless.io,resources=tenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kuberless.io,resources=tenants/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kuberless.io,resources=tenants/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=resourcequotas,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=capsule.clastix.io,resources=tenants,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=cilium.io,resources=ciliumnetworkpolicies,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=kuberless.io,resources=apps,verbs=get;list;watch;delete

func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	tenant := &platformv1alpha1.Tenant{}
	if err := r.Get(ctx, req.NamespacedName, tenant); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion.
	if !tenant.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, tenant)
	}

	// Ensure finalizer is set.
	if !controllerutil.ContainsFinalizer(tenant, shared.FinalizerTenant) {
		controllerutil.AddFinalizer(tenant, shared.FinalizerTenant)
		if err := r.Update(ctx, tenant); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set phase to Pending if not set.
	if tenant.Status.Phase == "" {
		tenant.Status.Phase = platformv1alpha1.TenantPhasePending
		if err := r.Status().Update(ctx, tenant); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile sub-resources.
	logger.Info("Reconciling tenant", "name", tenant.Name)

	// 1. Namespace
	if err := r.NamespaceReconciler.Reconcile(ctx, tenant); err != nil {
		return r.setFailed(ctx, tenant, "NamespaceFailed", err)
	}

	// 2. Capsule Tenant
	if err := r.CapsuleReconciler.Reconcile(ctx, tenant); err != nil {
		return r.setFailed(ctx, tenant, "CapsuleFailed", err)
	}

	// 3. CiliumNetworkPolicy
	if err := r.CiliumReconciler.Reconcile(ctx, tenant); err != nil {
		return r.setFailed(ctx, tenant, "CiliumPolicyFailed", err)
	}

	// 4. ResourceQuota
	if err := r.ResourceQuotaReconciler.Reconcile(ctx, tenant); err != nil {
		return r.setFailed(ctx, tenant, "ResourceQuotaFailed", err)
	}

	// Count active apps.
	appList := &platformv1alpha1.AppList{}
	if err := r.List(ctx, appList, client.InNamespace(tenant.GetNamespaceName())); err != nil {
		logger.Error(err, "Failed to list apps")
	} else {
		tenant.Status.ActiveApps = int32(len(appList.Items))
	}

	// Update status to Active.
	tenant.Status.Phase = platformv1alpha1.TenantPhaseActive
	tenant.Status.Namespace = tenant.GetNamespaceName()
	meta.SetStatusCondition(&tenant.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            "All tenant resources reconciled successfully",
		LastTransitionTime: metav1.Now(),
	})

	if err := r.Status().Update(ctx, tenant); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Tenant reconciled successfully", "name", tenant.Name, "namespace", tenant.GetNamespaceName())
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *TenantReconciler) reconcileDelete(ctx context.Context, tenant *platformv1alpha1.Tenant) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling tenant deletion", "name", tenant.Name)

	tenant.Status.Phase = platformv1alpha1.TenantPhaseDeleting
	if err := r.Status().Update(ctx, tenant); err != nil {
		return ctrl.Result{}, err
	}

	// Delete apps first.
	appList := &platformv1alpha1.AppList{}
	if err := r.List(ctx, appList, client.InNamespace(tenant.GetNamespaceName())); err == nil {
		for i := range appList.Items {
			if err := r.Delete(ctx, &appList.Items[i]); err != nil && !errors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("deleting app %s: %w", appList.Items[i].Name, err)
			}
		}
		if len(appList.Items) > 0 {
			logger.Info("Waiting for apps to be deleted", "count", len(appList.Items))
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
	}

	// Delete CiliumNetworkPolicy.
	if err := r.CiliumReconciler.Delete(ctx, tenant); err != nil {
		logger.Error(err, "Failed to delete CiliumNetworkPolicy")
	}

	// Delete Capsule Tenant.
	if err := r.CapsuleReconciler.Delete(ctx, tenant); err != nil {
		logger.Error(err, "Failed to delete Capsule Tenant")
	}

	// Delete namespace last.
	if err := r.NamespaceReconciler.Delete(ctx, tenant); err != nil {
		logger.Error(err, "Failed to delete namespace")
	}

	// Remove finalizer.
	controllerutil.RemoveFinalizer(tenant, shared.FinalizerTenant)
	if err := r.Update(ctx, tenant); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Tenant deleted successfully", "name", tenant.Name)
	return ctrl.Result{}, nil
}

func (r *TenantReconciler) setFailed(ctx context.Context, tenant *platformv1alpha1.Tenant, reason string, err error) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Error(err, "Tenant reconciliation failed", "reason", reason)

	tenant.Status.Phase = platformv1alpha1.TenantPhaseFailed
	meta.SetStatusCondition(&tenant.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})

	if updateErr := r.Status().Update(ctx, tenant); updateErr != nil {
		logger.Error(updateErr, "Failed to update tenant status")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.Tenant{}).
		Named("tenant").
		Complete(r)
}
