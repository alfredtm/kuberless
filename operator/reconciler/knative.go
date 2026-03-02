package reconciler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/alfredtm/kuberless/api/v1alpha1"
	"github.com/alfredtm/kuberless/operator/shared"
)

// KnativeReconciler manages Knative Service resources for apps.
type KnativeReconciler struct {
	Client client.Client
}

// Reconcile ensures the Knative Service for the app exists and is up-to-date.
func (r *KnativeReconciler) Reconcile(ctx context.Context, app *platformv1alpha1.App) error {
	logger := log.FromContext(ctx)
	ksvcName := app.Name
	ns := app.Namespace

	desired := r.buildKnativeService(app, ksvcName, ns)

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "serving.knative.dev",
		Version: "v1",
		Kind:    "Service",
	})

	err := r.Client.Get(ctx, client.ObjectKey{Name: ksvcName, Namespace: ns}, existing)
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("getting Knative Service: %w", err)
		}
		logger.Info("Creating Knative Service", "namespace", ns, "name", ksvcName)
		return r.Client.Create(ctx, desired)
	}

	// Check if the spec has changed to avoid unnecessary updates that create new revisions.
	existingSpec, _ := json.Marshal(existing.Object["spec"])
	desiredSpec, _ := json.Marshal(desired.Object["spec"])
	if string(existingSpec) == string(desiredSpec) {
		logger.V(1).Info("Knative Service spec unchanged, skipping update", "name", ksvcName)
		return nil
	}

	// Preserve immutable annotations set by Knative (e.g. serving.knative.dev/creator).
	existingAnnotations := existing.GetAnnotations()
	desiredAnnotations := desired.GetAnnotations()
	if desiredAnnotations == nil {
		desiredAnnotations = make(map[string]string)
	}
	for _, key := range []string{"serving.knative.dev/creator", "serving.knative.dev/lastModifier"} {
		if v, ok := existingAnnotations[key]; ok {
			desiredAnnotations[key] = v
		}
	}
	desired.SetAnnotations(desiredAnnotations)

	desired.SetResourceVersion(existing.GetResourceVersion())
	logger.Info("Updating Knative Service", "namespace", ns, "name", ksvcName)
	return r.Client.Update(ctx, desired)
}

// Delete removes the Knative Service for the app.
func (r *KnativeReconciler) Delete(ctx context.Context, app *platformv1alpha1.App) error {
	logger := log.FromContext(ctx)

	ksvc := &unstructured.Unstructured{}
	ksvc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "serving.knative.dev",
		Version: "v1",
		Kind:    "Service",
	})

	err := r.Client.Get(ctx, client.ObjectKey{Name: app.Name, Namespace: app.Namespace}, ksvc)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("getting Knative Service for deletion: %w", err)
	}

	logger.Info("Deleting Knative Service", "namespace", app.Namespace, "name", app.Name)
	return r.Client.Delete(ctx, ksvc)
}

// GetStatus reads the status of the Knative Service and returns app status fields.
func (r *KnativeReconciler) GetStatus(ctx context.Context, app *platformv1alpha1.App) (url, latestRevision string, readyInstances int32, ready bool, err error) {
	ksvc := &unstructured.Unstructured{}
	ksvc.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "serving.knative.dev",
		Version: "v1",
		Kind:    "Service",
	})

	err = r.Client.Get(ctx, client.ObjectKey{Name: app.Name, Namespace: app.Namespace}, ksvc)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", "", 0, false, nil
		}
		return "", "", 0, false, err
	}

	status, ok := ksvc.Object["status"].(map[string]interface{})
	if !ok {
		return "", "", 0, false, nil
	}

	if u, ok := status["url"].(string); ok {
		url = u
	}

	if lr, ok := status["latestReadyRevisionName"].(string); ok {
		latestRevision = lr
	}

	// Check conditions for readiness.
	if conditions, ok := status["conditions"].([]interface{}); ok {
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if cond["type"] == "Ready" && cond["status"] == "True" {
				ready = true
			}
		}
	}

	return url, latestRevision, readyInstances, ready, nil
}

func (r *KnativeReconciler) buildKnativeService(app *platformv1alpha1.App, name, namespace string) *unstructured.Unstructured {
	annotations := map[string]interface{}{
		"autoscaling.knative.dev/min-scale": strconv.Itoa(int(app.Spec.Scaling.MinInstances)),
		"autoscaling.knative.dev/max-scale": strconv.Itoa(int(app.Spec.Scaling.MaxInstances)),
		"autoscaling.knative.dev/target":    strconv.Itoa(int(app.Spec.Scaling.TargetConcurrency)),
	}

	// Build environment variables.
	envVars := make([]interface{}, 0, len(app.Spec.Env))
	for _, e := range app.Spec.Env {
		ev := map[string]interface{}{
			"name": e.Name,
		}
		if e.SecretRef != nil {
			ev["valueFrom"] = map[string]interface{}{
				"secretKeyRef": map[string]interface{}{
					"name": e.SecretRef.Name,
					"key":  e.SecretRef.Key,
				},
			}
		} else {
			ev["value"] = e.Value
		}
		envVars = append(envVars, ev)
	}

	container := map[string]interface{}{
		"image": app.Spec.Image,
		"ports": []interface{}{
			map[string]interface{}{
				"containerPort": int64(app.Spec.Port),
			},
		},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    app.Spec.Resources.CPURequest,
				"memory": app.Spec.Resources.MemoryLimit,
			},
			"limits": map[string]interface{}{
				"cpu":    app.Spec.Resources.CPURequest,
				"memory": app.Spec.Resources.MemoryLimit,
			},
		},
	}

	if len(envVars) > 0 {
		container["env"] = envVars
	}

	ksvc := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "serving.knative.dev/v1",
			"kind":       "Service",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					shared.LabelManagedBy: shared.LabelManagedByValue,
					shared.LabelApp:       app.Name,
					shared.LabelTenant:    app.Spec.TenantRef,
				},
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"metadata": map[string]interface{}{
						"annotations": annotations,
					},
					"spec": map[string]interface{}{
						"containers": []interface{}{container},
					},
				},
			},
		},
	}

	return ksvc
}
