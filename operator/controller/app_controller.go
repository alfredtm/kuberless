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

// AppReconciler reconciles a App object
type AppReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	KnativeReconciler *reconciler.KnativeReconciler
	DomainReconciler  *reconciler.DomainReconciler
}

// +kubebuilder:rbac:groups=kuberless.io,resources=apps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kuberless.io,resources=apps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kuberless.io,resources=apps/finalizers,verbs=update
// +kubebuilder:rbac:groups=serving.knative.dev,resources=services,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=serving.knative.dev,resources=domainmappings,verbs=get;list;watch;create;update;delete

func (r *AppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	app := &platformv1alpha1.App{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion.
	if !app.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, app)
	}

	// Ensure finalizer is set.
	if !controllerutil.ContainsFinalizer(app, shared.FinalizerApp) {
		controllerutil.AddFinalizer(app, shared.FinalizerApp)
		if err := r.Update(ctx, app); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Set initial status.
	if app.Status.Phase == "" {
		app.Status.Phase = platformv1alpha1.AppPhasePending
		if err := r.Status().Update(ctx, app); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle paused state.
	if app.Spec.Paused {
		app.Status.Phase = platformv1alpha1.AppPhasePaused
		if err := r.Status().Update(ctx, app); err != nil {
			return ctrl.Result{}, err
		}
		// Delete Knative Service to scale to zero.
		if err := r.KnativeReconciler.Delete(ctx, app); err != nil {
			logger.Error(err, "Failed to delete Knative Service for paused app")
		}
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling app", "name", app.Name, "namespace", app.Namespace)

	// Only mark as deploying on first transition from Pending.
	if app.Status.Phase == platformv1alpha1.AppPhasePending {
		app.Status.Phase = platformv1alpha1.AppPhaseDeploying
		if err := r.Status().Update(ctx, app); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate tenant exists.
	tenant := &platformv1alpha1.Tenant{}
	if err := r.Get(ctx, client.ObjectKey{Name: app.Spec.TenantRef}, tenant); err != nil {
		if errors.IsNotFound(err) {
			return r.setFailed(ctx, app, "TenantNotFound", fmt.Errorf("tenant %s not found", app.Spec.TenantRef))
		}
		return ctrl.Result{}, err
	}

	// 1. Knative Service.
	if err := r.KnativeReconciler.Reconcile(ctx, app); err != nil {
		return r.setFailed(ctx, app, "KnativeServiceFailed", err)
	}

	// 2. Domain Mappings.
	if err := r.DomainReconciler.Reconcile(ctx, app); err != nil {
		return r.setFailed(ctx, app, "DomainMappingFailed", err)
	}

	// 3. Read status from Knative Service.
	url, latestRevision, readyInstances, ready, err := r.KnativeReconciler.GetStatus(ctx, app)
	if err != nil {
		logger.Error(err, "Failed to get Knative Service status")
	}

	app.Status.URL = url
	app.Status.LatestRevision = latestRevision
	app.Status.ReadyInstances = readyInstances

	if ready {
		app.Status.Phase = platformv1alpha1.AppPhaseReady
		meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "KnativeServiceReady",
			Message:            "App is ready and serving traffic",
			LastTransitionTime: metav1.Now(),
		})
	} else {
		app.Status.Phase = platformv1alpha1.AppPhaseDeploying
		meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "KnativeServiceNotReady",
			Message:            "Waiting for Knative Service to become ready",
			LastTransitionTime: metav1.Now(),
		})
	}

	if err := r.Status().Update(ctx, app); err != nil {
		return ctrl.Result{}, err
	}

	if !ready {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger.Info("App reconciled successfully", "name", app.Name, "url", url)
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *AppReconciler) reconcileDelete(ctx context.Context, app *platformv1alpha1.App) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling app deletion", "name", app.Name)

	// Delete DomainMappings.
	if err := r.DomainReconciler.Delete(ctx, app); err != nil {
		logger.Error(err, "Failed to delete DomainMappings")
	}

	// Delete Knative Service.
	if err := r.KnativeReconciler.Delete(ctx, app); err != nil {
		logger.Error(err, "Failed to delete Knative Service")
	}

	// Remove finalizer.
	controllerutil.RemoveFinalizer(app, shared.FinalizerApp)
	if err := r.Update(ctx, app); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("App deleted successfully", "name", app.Name)
	return ctrl.Result{}, nil
}

func (r *AppReconciler) setFailed(ctx context.Context, app *platformv1alpha1.App, reason string, err error) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Error(err, "App reconciliation failed", "reason", reason)

	app.Status.Phase = platformv1alpha1.AppPhaseFailed
	meta.SetStatusCondition(&app.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            err.Error(),
		LastTransitionTime: metav1.Now(),
	})

	if updateErr := r.Status().Update(ctx, app); updateErr != nil {
		logger.Error(updateErr, "Failed to update app status")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.App{}).
		Named("app").
		Complete(r)
}
