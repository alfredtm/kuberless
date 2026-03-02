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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AppPhase describes the lifecycle phase of the app.
// +kubebuilder:validation:Enum=Pending;Deploying;Ready;Failed;Paused
type AppPhase string

const (
	AppPhasePending   AppPhase = "Pending"
	AppPhaseDeploying AppPhase = "Deploying"
	AppPhaseReady     AppPhase = "Ready"
	AppPhaseFailed    AppPhase = "Failed"
	AppPhasePaused    AppPhase = "Paused"
)

// EnvVar represents an environment variable for an app.
type EnvVar struct {
	// Name of the environment variable.
	Name string `json:"name"`
	// Literal value.
	// +optional
	Value string `json:"value,omitempty"`
	// Reference to a secret key.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

// SecretKeyRef references a key in a Kubernetes Secret.
type SecretKeyRef struct {
	// Name of the Secret.
	Name string `json:"name"`
	// Key within the Secret.
	Key string `json:"key"`
}

// ScalingSpec defines the autoscaling behavior for an app.
type ScalingSpec struct {
	// Minimum number of instances. 0 enables scale-to-zero.
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	MinInstances int32 `json:"minInstances,omitempty"`

	// Maximum number of instances.
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	MaxInstances int32 `json:"maxInstances,omitempty"`

	// Target concurrent requests per instance.
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=1
	TargetConcurrency int32 `json:"targetConcurrency,omitempty"`
}

// AppResources defines the resource requests/limits for an app container.
type AppResources struct {
	// CPU request (e.g. "100m").
	// +kubebuilder:default="100m"
	CPURequest string `json:"cpuRequest,omitempty"`
	// Memory limit (e.g. "512Mi").
	// +kubebuilder:default="512Mi"
	MemoryLimit string `json:"memoryLimit,omitempty"`
}

// AppSpec defines the desired state of App.
type AppSpec struct {
	// Reference to the parent Tenant (by name, must exist in the same cluster).
	// +kubebuilder:validation:MinLength=1
	TenantRef string `json:"tenantRef"`

	// Container image to deploy.
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// Container port to expose.
	// +kubebuilder:default=8080
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// Environment variables for the container.
	// +optional
	Env []EnvVar `json:"env,omitempty"`

	// Autoscaling configuration.
	// +optional
	Scaling ScalingSpec `json:"scaling,omitempty"`

	// Resource requests and limits.
	// +optional
	Resources AppResources `json:"resources,omitempty"`

	// Custom domain hostnames for this app.
	// +optional
	CustomDomains []CustomDomain `json:"customDomains,omitempty"`

	// Whether the app is paused (scaled to zero and not accepting traffic).
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// CustomDomain represents a custom domain mapping.
type CustomDomain struct {
	// Hostname for the custom domain.
	Hostname string `json:"hostname"`
}

// AppStatus defines the observed state of App.
type AppStatus struct {
	// Current lifecycle phase of the app.
	// +optional
	Phase AppPhase `json:"phase,omitempty"`

	// The auto-generated URL for this app.
	// +optional
	URL string `json:"url,omitempty"`

	// The name of the latest Knative revision.
	// +optional
	LatestRevision string `json:"latestRevision,omitempty"`

	// Number of ready instances.
	// +optional
	ReadyInstances int32 `json:"readyInstances,omitempty"`

	// Standard conditions for the app.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyInstances`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// App is the Schema for the apps API.
type App struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppSpec   `json:"spec,omitempty"`
	Status AppStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AppList contains a list of App.
type AppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []App `json:"items"`
}

func init() {
	SchemeBuilder.Register(&App{}, &AppList{})
}
